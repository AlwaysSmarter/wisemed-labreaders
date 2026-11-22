package ecard

import (
	"errors"
	"strings"
)

type Identity struct {
	CNP string `json:"cnp"`
	CID string `json:"cid"`
}

func missingCNP(s string) bool {
	return strings.TrimSpace(s) == "" || strings.TrimSpace(s) == "0000000000000"
}
func validateExpected(i Identity) error {
	if missingCNP(i.CNP) && strings.TrimSpace(i.CID) == "" {
		return errors.New("expected non-anonymous CNP or CID required")
	}
	return nil
}
func matchIdentity(expected, actual Identity) error {
	if e := validateExpected(expected); e != nil {
		return e
	}
	if missingCNP(actual.CNP) {
		if strings.TrimSpace(expected.CID) == "" || strings.TrimSpace(actual.CID) == "" || strings.TrimSpace(expected.CID) != strings.TrimSpace(actual.CID) {
			return errors.New("card CNP missing or anonymized; matching CID required")
		}
		return nil
	}
	if !missingCNP(expected.CNP) && strings.TrimSpace(expected.CNP) != strings.TrimSpace(actual.CNP) {
		return errors.New("card CNP mismatch")
	}
	if strings.TrimSpace(expected.CID) != "" && strings.TrimSpace(expected.CID) != strings.TrimSpace(actual.CID) {
		return errors.New("card CID mismatch")
	}
	return nil
}
