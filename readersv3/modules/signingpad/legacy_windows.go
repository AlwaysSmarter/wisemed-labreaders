package signingpad

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

type signotecEventSink struct {
	events   chan padEvent
	points   signaturePoints
	hotspots map[int32]string
}

var signotecCallbackSink atomic.Pointer[signotecEventSink]

// One trampoline for the process: NewCallback allocates non-reclaimable slots.
// STControlSetCallback's CBPTR is cdecl, including when its setter is stdcall.
var signotecCallback = windows.NewCallbackCDecl(func(event, data, size, custom uintptr) uintptr {
	sink := signotecCallbackSink.Load()
	if sink == nil {
		return 0
	}
	action := ""
	if event == 4 && data != 0 && size >= 16 {
		point := *(*[4]int32)(unsafe.Pointer(data))
		sink.points.add(point[0], point[1], point[2])
		action = "activity"

	}
	if event == 0 {
		action = "disconnect"
	}
	if event == 1 && data != 0 && size >= 4 {
		action = sink.hotspots[*(*int32)(unsafe.Pointer(data))]
	}
	if event == 2 {
		action = "cancel"
	}
	if action != "" {
		select {
		case sink.events <- padEvent{Action: action}:
		default:
		}
	}
	return 0
})

func (d *signotecDriver) legacyExports() error {
	for _, name := range []string{"ControlSetCallback", "DisplayGetWidth", "DisplayGetHeight", "DisplaySetFont", "DisplaySetFontColor", "DisplaySetText", "SensorSetSignRect", "SensorClearHotSpots", "SensorAddHotSpot"} {
		if d.procs[name] != nil {
			continue
		}
		symbol := "ST" + name
		if runtime.GOARCH == "386" {
			symbol += "_stdcall"
		}
		proc, err := d.dll.FindProc(symbol)
		if err != nil {
			return fmt.Errorf("legacy capture requires %s: %w", symbol, err)
		}
		d.procs[name] = proc
	}
	return nil
}
func (d *signotecDriver) displayText(x, y int32, text string) error {
	value, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	_, err = d.call("DisplaySetText", uintptr(x), uintptr(y), 0, uintptr(unsafe.Pointer(value)))
	runtime.KeepAlive(value)
	return err
}
func (d *signotecDriver) BeginLegacy(name string) (<-chan padEvent, error) {
	if err := d.legacyExports(); err != nil {
		return nil, err
	}
	if err := d.openDevice(); err != nil {
		return nil, err
	}
	width, err := d.call("DisplayGetWidth")
	if err != nil {
		return nil, err
	}
	height, err := d.call("DisplayGetHeight")
	if err != nil {
		return nil, err
	}
	if width < 200 || height < 120 {
		return nil, fmt.Errorf("pad display too small for confirmation controls")
	}
	if _, err = d.call("SensorClearHotSpots"); err != nil {
		return nil, err
	}
	font, _ := windows.UTF16PtrFromString("Arial")
	fontSize := int32(16)
	if width < 400 {
		fontSize = 12
	}
	_, err = d.call("DisplaySetFont", uintptr(unsafe.Pointer(font)), uintptr(fontSize), 0)
	runtime.KeepAlive(font)
	if err != nil {
		return nil, err
	}
	if _, err = d.call("DisplaySetFontColor", 0); err != nil {
		return nil, err
	}
	if err = d.displayText(8, 4, padDisplayName(name)); err != nil {
		return nil, err
	}
	top := fontSize + 16
	buttonHeight := fontSize + 24
	buttonTop := height - buttonHeight
	if _, err = d.call("SensorSetSignRect", 0, uintptr(top), uintptr(width), uintptr(buttonTop-top-4)); err != nil {
		return nil, err
	}
	sink := &signotecEventSink{events: make(chan padEvent, 16), hotspots: map[int32]string{}}
	for index, item := range []struct{ label, action string }{{"Anuleaza", "cancel"}, {"Reia", "retry"}, {"Confirma", "confirm"}} {
		left := int32(index) * width / 3
		right := int32(index+1) * width / 3
		id, err := d.call("SensorAddHotSpot", uintptr(left), uintptr(buttonTop), uintptr(right-left), uintptr(buttonHeight))
		if err != nil {
			return nil, err
		}
		sink.hotspots[id] = item.action
		if err = d.displayText(left+6, buttonTop+8, item.label); err != nil {
			return nil, err
		}
	}
	d.eventSink = sink
	signotecCallbackSink.Store(sink)
	d.procs["ControlSetCallback"].Call(signotecCallback, 0)
	if _, err = d.call("SignatureStart"); err != nil {
		return nil, err
	}
	return sink.events, nil
}
func (d *signotecDriver) SigString() (string, error) {
	if d.eventSink == nil {
		return "", fmt.Errorf("signature capture is not initialized")
	}
	return d.eventSink.points.sigString()
}
func (d *signotecDriver) ConfirmLegacy(imageType string) ([]byte, error) {
	fileType := uintptr(1)
	switch imageType {
	case "tiff":
		fileType = 0
	case "bmp":
		fileType = 2
	case "jpg":
		fileType = 3
	case "gif":
		fileType = 4
	}
	return d.confirmImage(500, 150, fileType, 8)
}

func (d *signotecDriver) LastActivity() time.Time {
	if d.eventSink == nil {
		return time.Time{}
	}
	return d.eventSink.points.lastActivity()
}
