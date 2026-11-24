package ecard

import "testing"

func TestCardContinuityCounterZero(t *testing.T) {
	atr := []byte{0x3b, 0x01}
	zero, err := cardContinuityStamp("reader", 0x22, 2, atr)
	if err != nil || zero != "reader:0:3b01" {
		t.Fatalf("zero counter rejected: %q %v", zero, err)
	}
	inUse, err := cardContinuityStamp("reader", 0x122, 2, atr)
	if err != nil || inUse != zero {
		t.Fatalf("SDK connection changed identity: %q %v", inUse, err)
	}
	next, err := cardContinuityStamp("reader", 0x20022, 2, atr)
	if err != nil || next == zero {
		t.Fatalf("card change not detected: %q %v", next, err)
	}
	for _, state := range []uint32{0x10, 0x24, 0x28, 0x220} {
		if _, err := cardContinuityStamp("reader", state, 2, atr); err == nil {
			t.Fatalf("unavailable state accepted: %x", state)
		}
	}
	for _, length := range []uint32{0, 3} {
		if _, err := cardContinuityStamp("reader", 0x22, length, atr); err == nil {
			t.Fatalf("bad ATR length accepted: %d", length)
		}
	}
}

func TestContinuityReaderMatchesSDKPhysicalSelection(t *testing.T) {
	for _, tt := range []struct {
		name, selected string
		connected      []string
		want           string
	}{
		{"model", "Gemalto USB Smart Card Reader", []string{"Gemalto USB Smart Card Reader 0"}, "Gemalto USB Smart Card Reader 0"},
		{"case", "gemalto usb smart card reader", []string{"Gemalto USB Smart Card Reader 0"}, "Gemalto USB Smart Card Reader 0"},
		{"exact", "Reader 0", []string{"Reader 0", "Reader 0 1"}, "Reader 0"},
		{"automatic", "", []string{"Reader 0"}, "Reader 0"},
		{"ambiguous model", "Reader", []string{"Reader 0", "Reader 1"}, ""},
		{"ambiguous automatic", "", []string{"Reader 0", "Reader 1"}, ""},
		{"missing", "Other", []string{"Reader 0"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := continuityReader(tt.selected, tt.connected)
			if got != tt.want || (err != nil) != (tt.want == "") {
				t.Fatalf("got %q, error %v", got, err)
			}
		})
	}
}
