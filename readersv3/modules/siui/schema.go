package siui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// The application is Go. Windows supplies the XSD engine through PowerShell's
// System.Xml; no downloaded executable or C# project is needed. User XML travels
// on stdin, never as executable text. Only bundled local schemas are resolved.
const schemaScript = `$ErrorActionPreference='Stop'
try {
 $p=[Console]::In.ReadToEnd()|ConvertFrom-Json
 $set=New-Object System.Xml.Schema.XmlSchemaSet
 $set.XmlResolver=New-Object System.Xml.XmlUrlResolver
 [void]$set.Add([string]$p.namespace,[string]$p.schema)
 $set.Compile()
 $settings=New-Object System.Xml.XmlReaderSettings
 $settings.DtdProcessing=[System.Xml.DtdProcessing]::Prohibit
 $settings.XmlResolver=$null
 $settings.ValidationType=[System.Xml.ValidationType]::Schema
 $settings.Schemas=$set
 $settings.MaxCharactersInDocument=[long]$p.limit
 $text=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($p.data))
 $reader=[System.Xml.XmlReader]::Create([System.IO.StringReader]::new($text),$settings)
 try { while($reader.Read()){} } finally {$reader.Dispose()}
 exit 0
} catch { [Console]::Error.WriteLine('XML does not conform to the bundled PIAS schema'); exit 1 }
`

func validateSchema(ctx context.Context, schema string, data []byte) error {
	base := "assets/schemas/validation/"
	limit := maxData
	names := []string{schema, "CommonReportingTypes.xsd", "CommonReportingTypesPE.xsd"}
	if strings.HasPrefix(schema, "docmed:") {
		schema = strings.TrimPrefix(schema, "docmed:")
		base = "assets/schemas/docmed/"
		limit = docMedXMLLimit
		names = nil
		entries, err := assets.ReadDir(strings.TrimSuffix(base, "/"))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
	}
	if _, err := assets.ReadFile(base + schema); err != nil || filepath.Base(schema) != schema {
		return errors.New("unsupported schema")
	}
	if _, err := parseXMLLimit(data, limit); err != nil {
		return err
	}
	dir, e := os.MkdirTemp("", "wisemed-siui-xsd-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	for _, name := range names {
		b, err := assets.ReadFile(base + name)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			return err
		}
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		chars := utf16.Encode([]rune(schemaScript))
		b := make([]byte, len(chars)*2)
		for i, v := range chars {
			binary.LittleEndian.PutUint16(b[i*2:], v)
		}
		exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		cmd = exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
		schemaData, _ := assets.ReadFile(base + schema)
		ns := piasNS
		if base == "assets/schemas/docmed/" {
			ns = docMedNS
		}
		if bytes.Contains(schemaData, []byte(`targetNamespace="http://www.cnas.ro/pel/1.0"`)) {
			ns = "http://www.cnas.ro/pel/1.0"
		}
		payload, _ := json.Marshal(map[string]any{"limit": limit, "namespace": ns, "schema": filepath.Join(dir, schema), "data": base64.StdEncoding.EncodeToString(data)})
		cmd.Stdin = bytes.NewReader(payload)
	} else {
		validator, err := exec.LookPath("xmllint")
		if err != nil {
			return errors.New("validatorul XSD xmllint nu este disponibil pe calculatorul bridge-ului")
		}
		cmd = exec.CommandContext(ctx, validator, "--nonet", "--noout", "--schema", filepath.Join(dir, schema), "-")
		cmd.Stdin = bytes.NewReader(data)
	}
	// Do not capture libxml diagnostics: they can contain patient data.
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var invalid *exec.ExitError
		if !errors.As(e, &invalid) {
			return errors.New("validatorul XSD nu a putut fi pornit pe calculatorul bridge-ului")
		}
		return errors.New("XML does not conform to the bundled PIAS schema")
	}
	return nil
}
