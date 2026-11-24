//go:build !windows

package ecard

import "errors"

func readerPresence() (map[string]bool, error) {
	return nil, errors.New("PC/SC monitoring requires Windows")
}

func readerContinuity(string) (string, error) {
	return "", errors.New("card continuity requires Windows")
}
