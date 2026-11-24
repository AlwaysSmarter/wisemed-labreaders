package runner

import (
	"os/exec"
	"syscall"
)

func configureUpdateProcess(cmd *exec.Cmd) {
	// PowerShell is a console host. Use CREATE_NO_WINDOW rather than
	// DETACHED_PROCESS, retaining redirected standard handles for diagnostics.
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | createNewProcessGroup, HideWindow: true}
}
