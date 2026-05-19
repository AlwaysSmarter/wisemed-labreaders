//go:build linux || darwin

package barcodeprinter

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func sendToPrinter(printerName string, data []byte) error {
	if err := runPrintCommand("lp", lpArgs(printerName), data); err == nil {
		return nil
	}
	if err := runPrintCommand("lpr", lprArgs(printerName), data); err == nil {
		return nil
	}
	return runPrintCommand("lpr", legacyLprArgs(printerName), data)
}

func lpArgs(printerName string) []string {
	args := []string{"-o", "raw"}
	if strings.TrimSpace(printerName) != "" {
		args = append(args, "-d", strings.TrimSpace(printerName))
	}
	return args
}

func lprArgs(printerName string) []string {
	args := []string{"-l"}
	if strings.TrimSpace(printerName) != "" {
		args = append(args, "-P", strings.TrimSpace(printerName))
	}
	return args
}

func legacyLprArgs(printerName string) []string {
	args := []string{}
	if strings.TrimSpace(printerName) != "" {
		args = append(args, "-P", strings.TrimSpace(printerName))
	}
	return args
}

func runPrintCommand(name string, args []string, data []byte) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(data)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s: %w", msg, err)
	}
	return nil
}

func listPrinters() []string {
	cmd := exec.Command("lpstat", "-a")
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}
	lines := strings.Split(string(out), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		names = append(names, parts[0])
	}
	return names
}
