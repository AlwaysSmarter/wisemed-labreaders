//go:build linux

package anafdocsmart

import "fmt"

func importXFAToPDF(acrobatApp, pdfPath, xmlPath, outputPath string) error {
	return fmt.Errorf("XFA import via Adobe Acrobat is not supported on Linux")
}
