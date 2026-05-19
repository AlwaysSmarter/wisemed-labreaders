//go:build windows

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

type headlessLaunchInfo struct {
	PID          int
	Instructions []string
}

func launchHeadlessProcess(configPath string) (headlessLaunchInfo, error) {
	exePath, err := os.Executable()
	if err != nil {
		return headlessLaunchInfo{}, err
	}
	workdir, err := os.Getwd()
	if err != nil {
		return headlessLaunchInfo{}, err
	}
	devNull, err := os.OpenFile("NUL", os.O_RDWR, 0)
	if err != nil {
		return headlessLaunchInfo{}, err
	}
	defer devNull.Close()

	args := []string{"-config", configPath, "-headless-child"}
	cmd := exec.Command(exePath, args...)
	cmd.Dir = workdir
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return headlessLaunchInfo{}, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return headlessLaunchInfo{}, err
	}

	return headlessLaunchInfo{
		PID: pid,
		Instructions: []string{
			"Application continues in background without keeping the console attached.",
			fmt.Sprintf("Stop: taskkill /PID %d /T /F", pid),
			fmt.Sprintf("Restart: taskkill /PID %d /T /F && %s", pid, cmdQuote(append([]string{exePath}, args...))),
		},
	}, nil
}

func cmdQuote(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			quoted = append(quoted, "\"\"")
			continue
		}
		if strings.IndexFunc(part, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '"'
		}) == -1 {
			quoted = append(quoted, part)
			continue
		}
		quoted = append(quoted, "\""+strings.ReplaceAll(part, "\"", "\\\"")+"\"")
	}
	return strings.Join(quoted, " ")
}
