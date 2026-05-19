//go:build darwin || linux

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return headlessLaunchInfo{}, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return headlessLaunchInfo{}, err
	}

	quotedCommand := shellQuote(append([]string{exePath}, args...))
	return headlessLaunchInfo{
		PID: pid,
		Instructions: []string{
			fmt.Sprintf("Application continues in background and the shell prompt is free."),
			fmt.Sprintf("Stop: kill %d", pid),
			fmt.Sprintf("Restart: kill %d && %s", pid, quotedCommand),
		},
	}, nil
}

func shellQuote(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			quoted = append(quoted, "''")
			continue
		}
		safe := true
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._:-", r)) {
				safe = false
				break
			}
		}
		if safe {
			quoted = append(quoted, part)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(part, "'", "'\"'\"'")+"'")
	}
	return strings.Join(quoted, " ")
}
