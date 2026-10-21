//go:build windows

package siuidata

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func processUsage() (cpuSeconds float64, rssBytes uint64, err error) {
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return
	}
	ticks := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	cpuSeconds = float64(ticks(kernel)+ticks(user)) / 1e7
	var m struct {
		Size, Faults                                                                                 uint32
		PeakWorkingSet, WorkingSet, PeakPaged, Paged, PeakNonPaged, NonPaged, Pagefile, PeakPagefile uintptr
	}
	m.Size = uint32(unsafe.Sizeof(m))
	p := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	ok, _, e := p.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&m)), uintptr(m.Size))
	if ok == 0 {
		err = e
		return
	}
	rssBytes = uint64(m.WorkingSet)
	return
}
