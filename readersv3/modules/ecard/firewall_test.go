package ecard

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"runtime"
	"testing"
	"unicode/utf16"
)

func TestEncodedPowerShellPreservesScript(t *testing.T) {
	want := firewallScript + "\nWrite-Output 'Validare și 🔒'\n"
	raw, err := base64.StdEncoding.DecodeString(encodedPowerShell(want))
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("invalid encoded command: %v", err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	if got := string(utf16.Decode(units)); got != want {
		t.Fatalf("PowerShell script changed during encoding: %q", got)
	}
}

func TestCEASFirewallChanges(t *testing.T) {
	online := Settings{UMHost: "umceas.siui.ro", UMPort: 443}
	offline := online
	offline.CEASOffline = true
	for _, tc := range []struct {
		name           string
		previous, next Settings
		want           bool
	}{
		{"unchanged online", online, online, false},
		{"online endpoint edit", online, Settings{UMHost: "testumceas.siui.ro", UMPort: 8443}, false},
		{"enable offline", online, offline, true},
		{"disable offline", offline, online, true},
		{"unchanged offline", offline, offline, false},
		{"offline host edit", offline, Settings{CEASOffline: true, UMHost: "testumceas.siui.ro", UMPort: 443}, true},
		{"offline port edit", offline, Settings{CEASOffline: true, UMHost: online.UMHost, UMPort: 8443}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ceasFirewallChanged(tc.previous, tc.next); got != tc.want {
				t.Fatalf("firewall change = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOfflineFirewallPlatformAndConfig(t *testing.T) {
	s, e := readSettings(map[string]interface{}{"ceas_offline": true})
	if e != nil || !s.CEASOffline {
		t.Fatalf("offline setting not decoded: %v", e)
	}
	if runtime.GOOS == "windows" {
		t.Skip("do not mutate the test machine firewall")
	}
	if e = applyCEASFirewall(context.Background(), s); e == nil {
		t.Fatal("offline accepted on unsupported OS")
	}
	s.CEASOffline = false
	if e = applyCEASFirewall(context.Background(), s); e != nil {
		t.Fatal(e)
	}
}
