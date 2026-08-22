//go:build !darwin && !linux

package anafdocsmart

import "fmt"

func importXFAToPDF(_ string, _ string, _ string, _ string) error {
	return fmt.Errorf("ANAF DocSmart PDF import is supported only on macOS with Adobe Acrobat")
}
