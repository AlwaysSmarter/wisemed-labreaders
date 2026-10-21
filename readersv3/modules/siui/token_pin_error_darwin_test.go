package siui

import "testing"

func TestMacPINRejectionRequiresAuthenticationFailureDomain(t *testing.T) {
	for _, code := range []int64{-1, -2, -3, -4, -5, -6, -7, -8, -9, -67820} {
		for _, correctDomain := range []bool{true, false} {
			a := &securityAPI{
				errorCode:   func(uintptr) int64 { return code },
				errorDomain: func(uintptr) uintptr { return 2 },
				stringCreate: func(_ uintptr, domain string, _ uint32) uintptr {
					if domain != "CryptoTokenKit" {
						t.Fatalf("wrong domain %q", domain)
					}
					return 3
				},
				equal:   func(uintptr, uintptr) bool { return correctDomain },
				release: func(uintptr) {},
			}
			if got := a.isPINRejection(1); got != (code == -5 && correctDomain) {
				t.Fatalf("code=%d domain=%v rejected=%v", code, correctDomain, got)
			}
		}
	}
}
