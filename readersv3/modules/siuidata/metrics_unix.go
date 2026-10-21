//go:build !windows

package siuidata

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func processUsage() (cpuSeconds float64, rssBytes uint64, err error) {
	var r syscall.Rusage
	if err = syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		return
	}
	cpuSeconds = float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
	if runtime.GOOS == "linux" {
		var b []byte
		b, err = os.ReadFile("/proc/self/statm")
		if err != nil {
			return
		}
		fields := strings.Fields(string(b))
		if len(fields) > 1 {
			var pages uint64
			pages, err = strconv.ParseUint(fields[1], 10, 64)
			rssBytes = pages * uint64(os.Getpagesize())
		}
		return
	}
	// macOS ps reports current resident memory in KiB (rusage only exposes peak RSS).
	var b []byte
	b, err = exec.Command("/bin/ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return
	}
	rssBytes, err = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	rssBytes *= 1024
	return
}
