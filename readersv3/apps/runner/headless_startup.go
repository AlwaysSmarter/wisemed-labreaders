package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const headlessStartupEnv = "WISEMED_HEADLESS_STARTUP_FILE"

// The detached process reports startup through a private file so it does not
// keep the calling terminal's stdout open after the launcher returns.
func startHeadlessAndWait(cmd *exec.Cmd) error {
	dir, err := os.MkdirTemp("", "wisemed-startup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "result")
	cmd.Env = append(os.Environ(), headlessStartupEnv+"="+path)
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(time.Minute)
	defer timeout.Stop()
	for {
		select {
		case <-ticker.C:
			result, err := os.ReadFile(path)
			if err == nil {
				if len(result) == 0 {
					return nil
				}
				<-exited
				return errors.New(string(result))
			}
		case err := <-exited:
			if result, readErr := os.ReadFile(path); readErr == nil && len(result) != 0 {
				return errors.New(string(result))
			}
			if err != nil {
				return fmt.Errorf("background process stopped during startup: %w", err)
			}
			return errors.New("background process stopped during startup")
		case <-timeout.C:
			_ = cmd.Process.Kill()
			<-exited
			return errors.New("background startup did not complete within one minute")
		}
	}
}

func writeHeadlessStartup(path string, err error) {
	var result []byte
	if err != nil {
		result = []byte(err.Error())
	}
	temp := path + ".tmp"
	if writeErr := os.WriteFile(temp, result, 0o600); writeErr == nil {
		_ = os.Rename(temp, path)
	}
}
