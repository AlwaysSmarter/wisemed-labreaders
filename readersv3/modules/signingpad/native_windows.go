package signingpad

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

type signotecDriver struct {
	dll         *windows.DLL
	procs       map[string]*windows.Proc
	opened      bool
	deviceIndex int
	eventSink   *signotecEventSink
}

func openNative(path string) (padDriver, error) {
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("load STPadLib.dll (%s): %w; DLL and executable must have matching architectures", runtime.GOARCH, err)
	}
	d := &signotecDriver{dll: dll, procs: map[string]*windows.Proc{}}
	for _, name := range []string{"DeviceGetCount", "DeviceOpen", "DeviceClose", "SignatureStart", "SignatureRetry", "SignatureConfirm", "SignatureCancel", "SignatureSaveAsStreamEx", "ControlExit"} {
		symbol := "ST" + name
		if runtime.GOARCH == "386" {
			symbol += "_stdcall"
		}
		p, err := dll.FindProc(symbol)
		if err != nil {
			dll.Release()
			return nil, fmt.Errorf("missing SDK export %s: %w", symbol, err)
		}
		d.procs[name] = p
	}
	return d, nil
}

func (d *signotecDriver) call(name string, args ...uintptr) (int32, error) {
	value, _, _ := d.procs[name].Call(args...)
	result := int32(value)
	if result < 0 {
		return result, fmt.Errorf("Signotec ST%s returned SDK error %d", name, result)
	}
	return result, nil
}
func (d *signotecDriver) Count() (int32, error) { return d.call("DeviceGetCount") }
func (d *signotecDriver) openDevice() error {
	if !d.opened {
		count, err := d.Count()
		if err != nil {
			return err
		}
		if count <= int32(d.deviceIndex) {
			return fmt.Errorf("configured pad index %d is unavailable; detected %d Signotec USB pads", d.deviceIndex, count)
		}
		if _, err := d.call("DeviceOpen", uintptr(d.deviceIndex), 1); err != nil {
			return err
		}
		d.opened = true
	}
	return nil
}
func (d *signotecDriver) Start() error {
	if err := d.openDevice(); err != nil {
		return err
	}
	_, err := d.call("SignatureStart")
	return err
}
func (d *signotecDriver) Retry() error  { _, err := d.call("SignatureRetry"); return err }
func (d *signotecDriver) Cancel() error { _, err := d.call("SignatureCancel", 0); return err }
func (d *signotecDriver) Confirm() ([]byte, error) {
	count, err := d.call("SignatureConfirm")
	if err != nil {
		return nil, err
	}
	if count <= int32(d.deviceIndex) {
		return nil, fmt.Errorf("signature is empty; start a new capture")
	}
	var size int32
	_, err = d.call("SignatureSaveAsStreamEx", 0, uintptr(unsafe.Pointer(&size)), 150, 0, 0, 1, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if size <= 0 || size > 16*1024*1024 {
		return nil, fmt.Errorf("invalid PNG size: %d", size)
	}
	data := make([]byte, int(size))
	_, err = d.call("SignatureSaveAsStreamEx", uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 150, 0, 0, 1, 0, 0, 0)
	runtime.KeepAlive(data)
	if err != nil {
		return nil, err
	}
	if size <= 0 || int(size) > len(data) {
		return nil, fmt.Errorf("invalid exported PNG size")
	}
	return data[:int(size)], nil
}
func (d *signotecDriver) Close() {
	if d.eventSink != nil {
		signotecCallbackSink.CompareAndSwap(d.eventSink, nil)
		d.procs["ControlSetCallback"].Call(0, 0)
	}
	if d.opened {
		d.call("DeviceClose", uintptr(d.deviceIndex))
	}
	d.procs["ControlExit"].Call()
	d.dll.Release()
}

func (d *signotecDriver) SetDeviceIndex(index int) { d.deviceIndex = index }
