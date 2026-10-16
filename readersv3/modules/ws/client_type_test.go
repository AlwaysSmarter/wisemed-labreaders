package ws

import (
	"testing"
	"wisemed-labreaders/readersv3/core/module"
)

type clientTypeRuntime struct {
	module.Runtime
	protocol string
}

func (r clientTypeRuntime) Service(name string) (interface{}, bool) {
	if name == "analyzer-config" {
		return map[string]interface{}{"protocol": r.protocol}, true
	}
	return nil, false
}
func TestClientType(t *testing.T) {
	for _, tc := range []struct{ protocol, want string }{
		{"siui", "siui-bridge"}, {" SIUI ", "siui-bridge"},
		{"barcodeprinter", "reader"}, {"", "reader"},
	} {
		m := &Module{rt: clientTypeRuntime{protocol: tc.protocol}}
		wantRole := "reader"
		if tc.want == "siui-bridge" {
			wantRole = "service"
		}
		if got := m.clientRole(); got != wantRole {
			t.Fatalf("role: got %q want %q", got, wantRole)
		}
		if got := m.clientType(); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.protocol, got, tc.want)
		}
	}
}
