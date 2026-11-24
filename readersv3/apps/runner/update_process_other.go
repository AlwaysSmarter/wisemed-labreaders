//go:build !windows

package runner

import "os/exec"

func configureUpdateProcess(cmd *exec.Cmd) {}
