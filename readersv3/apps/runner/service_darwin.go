//go:build darwin

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
)

func installService(cfg *config.Config) error {
	info, err := buildServiceInstallInfo(cfg)
	if err != nil {
		return err
	}
	label := "eu.wisemed.readersv3." + strings.TrimPrefix(info.ServiceName, "wisemed-")
	plistPath := filepath.Join("/Library/LaunchDaemons", label+".plist")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>-config</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>StandardOutPath</key>
  <string>/var/log/%s.log</string>
  <key>StandardErrorPath</key>
  <string>/var/log/%s.log</string>
</dict>
</plist>
`, label, xmlEscape(info.Executable), xmlEscape(info.ConfigPath), xmlEscape(info.WorkDir), info.ServiceName, info.ServiceName)
	if err := os.WriteFile(plistPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w. Run with sudo", plistPath, err)
	}
	_ = exec.Command("launchctl", "bootout", "system", plistPath).Run()
	for _, args := range [][]string{
		{"launchctl", "bootstrap", "system", plistPath},
		{"launchctl", "enable", "system/" + label},
		{"launchctl", "kickstart", "-k", "system/" + label},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	fmt.Printf("Serviciu instalat cu succes: %s\n", label)
	fmt.Printf("Pentru a-l porni: sudo launchctl kickstart -k system/%s\n", label)
	fmt.Printf("Pentru a-l opri: sudo launchctl bootout system/%s\n", label)
	fmt.Printf("Pentru restart: sudo launchctl bootout system/%s && sudo launchctl bootstrap system %s && sudo launchctl kickstart -k system/%s\n", label, plistPath, label)
	fmt.Printf("Status: sudo launchctl print system/%s\n", label)
	return nil
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}
