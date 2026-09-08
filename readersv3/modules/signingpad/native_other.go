//go:build !windows

package signingpad

import "fmt"

func openNative(path string) (padDriver, error) {
	return nil, fmt.Errorf("Signotec USB requires Windows and STPadLib.dll; this platform cannot load a Windows DLL")
}
