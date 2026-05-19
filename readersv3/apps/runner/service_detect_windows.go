//go:build windows

package runner

import "golang.org/x/sys/windows/svc"

func currentRunsAsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}
