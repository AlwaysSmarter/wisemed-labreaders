//go:build darwin

package anafdocsmart

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func importXFAToPDF(acrobatApp, workPDF, workXML, outputPDF string) error {
	menuName := "DocsmartAutoImportMenu"
	menuScriptPath := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Adobe", "Acrobat", "DC", "JavaScripts", "docsmart_auto_import.js")
	if err := os.MkdirAll(filepath.Dir(menuScriptPath), 0o755); err != nil {
		return fmt.Errorf("prepare Acrobat JavaScripts directory: %w", err)
	}
	if err := os.WriteFile(menuScriptPath, []byte(buildFolderLevelScript(menuName, workXML, outputPDF)), 0o644); err != nil {
		return fmt.Errorf("write Acrobat folder script: %w", err)
	}
	defer os.Remove(menuScriptPath)

	if err := restartAcrobat(acrobatApp); err != nil {
		return err
	}
	if err := openPDFInAcrobat(acrobatApp, workPDF); err != nil {
		return err
	}
	if err := executeAcrobatMenu(acrobatApp, menuName); err != nil {
		return err
	}
	if err := waitForOutput(outputPDF, 30*time.Second); err != nil {
		return err
	}
	if err := closeActiveDoc(acrobatApp); err != nil {
		return err
	}
	return nil
}

func buildFolderLevelScript(menuName, xmlPath, outputPath string) string {
	xmlJS := jsString(xmlPath)
	outputJS := jsString(outputPath)
	menuJS := jsString(menuName)
	return fmt.Sprintf(`app.addMenuItem({
  cName: %s,
  cUser: %s,
  cParent: "File",
  cEnable: "event.rc = (app.activeDocs.length > 0);",
  cExec: "docsmartAutoImport();"
});

var docsmartAutoImport = app.trustedFunction(function () {
  app.beginPriv();
  try {
    var doc = (typeof event != "undefined" && event.target) ? event.target : app.activeDocs[0];
    var tool = csDataTool.GetInstance();
    tool.__ValidationMessages = "";
    tool.__DoLoadXmlFromFile = function () {
      this.Xml = "";
      this.Form.importDataObject("temp.xml", %s);
      var stream = this.Form.getDataObjectContents("temp.xml");
      var rawXml = util.stringFromStream(stream, "utf-8");
      if (rawXml == "") {
        rawXml = util.stringFromStream(stream, "utf-16");
      }
      this.Xml = this.__CleanupXml(rawXml);
      var xmlTag = "<f" + this.Declaratie.Numar + " ";
      if (this.Xml.indexOf(xmlTag) == -1) {
        this.__ValidationMessages += "Fisierul XML importat nu este valid!\\r\\n";
      }
    };
    tool.ExecuteImport();
    if (tool.__ValidationMessages != "") {
      throw new Error(tool.__ValidationMessages);
    }
    doc.saveAs(%s);
  } finally {
    app.endPriv();
  }
});`, menuJS, menuJS, xmlJS, outputJS)
}

func restartAcrobat(acrobatApp string) error {
	quitScript := fmt.Sprintf(`tell application %s
try
 quit saving no
end try
end tell`, appleScriptString(acrobatApp))
	_ = runOSA(quitScript)
	time.Sleep(2 * time.Second)
	cmd := exec.Command("open", "-a", acrobatApp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launch Acrobat: %s", strings.TrimSpace(string(out)))
	}
	time.Sleep(5 * time.Second)
	return nil
}

func openPDFInAcrobat(acrobatApp, pdfPath string) error {
	script := fmt.Sprintf(`tell application %s
activate
open POSIX file %s
end tell`, appleScriptString(acrobatApp), appleScriptString(pdfPath))
	if err := runOSA(script); err != nil {
		return fmt.Errorf("open PDF in Acrobat: %w", err)
	}
	time.Sleep(3 * time.Second)
	return nil
}

func executeAcrobatMenu(acrobatApp, menuName string) error {
	script := fmt.Sprintf(`tell application %s
activate
execute menu item %s of menu "File"
end tell`, appleScriptString(acrobatApp), appleScriptString(menuName))
	if err := runOSA(script); err != nil {
		return fmt.Errorf("execute Acrobat menu %s: %w", menuName, err)
	}
	return nil
}

func closeActiveDoc(acrobatApp string) error {
	script := fmt.Sprintf(`tell application %s
try
 if (count of documents) > 0 then
  close active doc saving no
 end if
end try
end tell`, appleScriptString(acrobatApp))
	if err := runOSA(script); err != nil {
		return fmt.Errorf("close Acrobat document: %w", err)
	}
	return nil
}

func waitForOutput(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for output PDF %s", path)
}

func runOSA(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

func appleScriptString(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func jsString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
