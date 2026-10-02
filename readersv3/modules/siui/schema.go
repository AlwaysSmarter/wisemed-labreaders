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
 [void]$set.Add('http://www.cnas.ro/siui/2.0',[string]$p.schema)
 $set.Compile()
 $settings=New-Object System.Xml.XmlReaderSettings
 $settings.DtdProcessing=[System.Xml.DtdProcessing]::Prohibit
 $settings.XmlResolver=$null
 $settings.ValidationType=[System.Xml.ValidationType]::Schema
 $settings.Schemas=$set
 $settings.MaxCharactersInDocument=1048576
 $text=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($p.data))
 $reader=[System.Xml.XmlReader]::Create([System.IO.StringReader]::new($text),$settings)
 try { while($reader.Read()){} } finally {$reader.Dispose()}
 exit 0
} catch { [Console]::Error.WriteLine('XML does not conform to the bundled PIAS schema'); exit 1 }
`

func validateSchema(ctx context.Context, schema string, data []byte) error {
	if schema != "ParaclinicServicesValidateRequest.xsd" && schema != "ParaclinicServicesValidateResponse.xsd" {
		return errors.New("unsupported schema")
	}
	if _, e := parseXML(data); e != nil {
		return e
	}
	dir, e := os.MkdirTemp("", "wisemed-siui-xsd-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{schema, "CommonReportingTypes.xsd"} {
		b, e := assets.ReadFile("assets/schemas/validation/" + name)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			return e
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
		payload, _ := json.Marshal(map[string]string{"schema": filepath.Join(dir, schema), "data": base64.StdEncoding.EncodeToString(data)})
		cmd.Stdin = bytes.NewReader(payload)
	} else {
		cmd = exec.CommandContext(ctx, "xmllint", "--nonet", "--noout", "--schema", filepath.Join(dir, schema), "-")
		cmd.Stdin = bytes.NewReader(data)
	}
	// Do not capture libxml diagnostics: they can contain patient data.
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("XML does not conform to the bundled PIAS schema, or the XSD validator is unavailable")
	}
	return nil
}
