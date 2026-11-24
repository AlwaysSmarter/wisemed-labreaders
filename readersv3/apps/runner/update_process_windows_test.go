package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPowerShellUpdaterLaunchWritesLogs(t *testing.T) {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}
	dir := filepath.Join(t.TempDir(), "stage with spaces")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "ready")
	script := filepath.Join(dir, "apply-update.ps1")
	body := "Set-Content -LiteralPath " + powerShellQuote(marker) + " -Value 'ready'\r\nWrite-Output 'updater started'\r\n"
	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	configureUpdateProcess(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "updater started") {
		t.Fatalf("PowerShell launch: %v, %s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("updater never entered script: %v", err)
	}
}
