//go:build windows

package ecard

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

var pcsc = windows.NewLazySystemDLL("winscard.dll")
var establish = pcsc.NewProc("SCardEstablishContext")
var release = pcsc.NewProc("SCardReleaseContext")
var listReaders = pcsc.NewProc("SCardListReadersW")
var getStatus = pcsc.NewProc("SCardGetStatusChangeW")

type readerState struct {
	Reader                    *uint16
	User                      uintptr
	Current, Event, ATRLength uint32
	ATR                       [36]byte
}

func scanReaderStates() (map[string]readerState, error) {
	var context uintptr
	r, _, _ := establish.Call(2, 0, 0, uintptr(unsafe.Pointer(&context)))
	if uint32(r) != 0 {
		return nil, fmt.Errorf("SCardEstablishContext: 0x%08X", uint32(r))
	}
	defer release.Call(context)
	result := map[string]readerState{}
	var size uint32
	r, _, _ = listReaders.Call(context, 0, 0, uintptr(unsafe.Pointer(&size)))
	if uint32(r) == 0x8010002E {
		return result, nil
	}
	if uint32(r) != 0 {
		return nil, fmt.Errorf("SCardListReaders: 0x%08X", uint32(r))
	}
	if size == 0 {
		return result, nil
	}
	if size > 65536 {
		return nil, fmt.Errorf("PC/SC reader list too large")
	}
	names := make([]uint16, size)
	r, _, _ = listReaders.Call(context, 0, uintptr(unsafe.Pointer(&names[0])), uintptr(unsafe.Pointer(&size)))
	if uint32(r) != 0 {
		return nil, fmt.Errorf("SCardListReaders: 0x%08X", uint32(r))
	}
	for start := 0; start < len(names) && names[start] != 0; {
		end := start
		for end < len(names) && names[end] != 0 {
			end++
		}
		if end == len(names) {
			break
		}
		name := windows.UTF16ToString(names[start:end])
		state := readerState{Reader: &names[start]}
		r, _, _ = getStatus.Call(context, 0, uintptr(unsafe.Pointer(&state)), 1)
		if uint32(r) != 0 {
			return nil, fmt.Errorf("SCardGetStatusChange: 0x%08X", uint32(r))
		}
		state.Reader = nil
		result[name] = state
		start = end + 1
	}
	return result, nil
}

func readerPresence() (map[string]bool, error) {
	states, e := scanReaderStates()
	if e != nil {
		return nil, e
	}
	out := map[string]bool{}
	for name, s := range states {
		out[name] = s.Event&0x20 != 0
	}
	return out, nil
}
func readerContinuity(selected string) (string, error) {
	states, e := scanReaderStates()
	if e != nil {
		return "", e
	}
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, name)
	}
	selected, e = continuityReader(selected, names)
	if e != nil {
		return "", e
	}
	s := states[selected]
	return cardContinuityStamp(selected, s.Event, s.ATRLength, s.ATR[:])
}
