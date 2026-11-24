package ecard

import (
	"fmt"
	"strings"
)

// The high word is a card-change counter, not a capability flag. Zero is a
// valid count (including wraparound). The monitor's epoch also participates
// in the worker identity and invalidates PINs on card/reader loss.
func cardContinuityStamp(reader string, state, atrLength uint32, atr []byte) (string, error) {
	if reader == "" {
		return "", fmt.Errorf("selected reader unavailable")
	}
	if state&0x20 == 0 {
		return "", fmt.Errorf("card not present")
	}
	if state&(0x4|0x8|0x200) != 0 {
		return "", fmt.Errorf("reader or card unavailable (state 0x%08X)", state)
	}
	if atrLength == 0 || atrLength > uint32(len(atr)) {
		return "", fmt.Errorf("card ATR unavailable")
	}
	return fmt.Sprintf("%s:%d:%x", reader, state>>16, atr[:atrLength]), nil
}

// Resolve model-only legacy configuration the same way as the host: prefer an
// exact physical name and otherwise require one unambiguous model match.
func continuityReader(selected string, connected []string) (string, error) {
	for _, name := range connected {
		if name == selected {
			return name, nil
		}
	}
	matches := []string{}
	for _, name := range connected {
		if selected == "" || strings.EqualFold(name, selected) || strings.HasPrefix(strings.ToLower(name), strings.ToLower(selected)+" ") {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("selected reader is ambiguous; select a physical reader")
	}
	return "", fmt.Errorf("selected reader unavailable")
}
