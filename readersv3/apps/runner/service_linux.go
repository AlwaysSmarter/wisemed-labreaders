//go:build linux

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"wisemed-labreaders/readersv3/core/config"
)

func installService(cfg *config.Config) error {
	info, err := buildServiceInstallInfo(cfg)
	if err != nil {
		return err
	}
	unitPath := "/etc/systemd/system/" + info.ServiceName + ".service"
	body := fmt.Sprintf(`[Unit]
Description=%s
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s -config %s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, info.Description, shellEscape(info.WorkDir), shellEscape(info.Executable), shellEscape(info.ConfigPath))
	if err := os.WriteFile(unitPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w. Run as root", unitPath, err)
	}
	commands := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", info.ServiceName + ".service"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	fmt.Printf("Serviciu instalat cu succes: %s\n", info.ServiceName)
	fmt.Printf("Pentru a-l porni: systemctl start %s.service\n", info.ServiceName)
	fmt.Printf("Pentru a-l opri: systemctl stop %s.service\n", info.ServiceName)
	fmt.Printf("Pentru restart: systemctl restart %s.service\n", info.ServiceName)
	fmt.Printf("Status: systemctl status %s.service\n", info.ServiceName)
	return nil
}

func shellEscape(value string) string {
	if value == "" {
		return "\"\""
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
