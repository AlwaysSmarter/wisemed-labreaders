//go:build !windows

package runner

func currentRunsAsService() bool {
	return false
}
