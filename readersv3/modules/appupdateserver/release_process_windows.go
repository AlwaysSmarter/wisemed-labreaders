package appupdateserver

import (
	"os/exec"
	"strconv"
	"time"
)

func configureReleaseProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() }
	cmd.WaitDelay = 5 * time.Second
}
