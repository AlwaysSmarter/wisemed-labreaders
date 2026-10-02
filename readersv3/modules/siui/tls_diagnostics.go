package siui

import (
	"fmt"
	"strings"
)

// WINHTTP_CALLBACK_STATUS_SECURE_FAILURE flags, from the Windows SDK.
// Keep diagnostics free of request headers, licence, PIN and session tokens.
func secureFailureMessage(flags uint32) string {
	var reasons []string
	for _, item := range []struct {
		mask    uint32
		message string
	}{
		{0x1, "verificarea revocării certificatului serverului a eșuat; verifică accesul la CRL/OCSP al emitentului"},
		{0x2, "certificatul serverului este invalid"},
		{0x4, "certificatul serverului este revocat"},
		{0x8, "autoritatea certificatului serverului nu este de încredere în Windows; verifică lanțul CA/intermediar"},
		{0x10, "numele certificatului serverului nu corespunde adresei CNAS"},
		{0x20, "certificatul serverului este expirat sau încă nu este valabil; verifică data calculatorului și valabilitatea certificatului CNAS"},
		{0x40, "certificatul serverului nu permite utilizarea TLS necesară"},
		{0x80000000, "negocierea Schannel a eșuat; verifică jurnalul System/Schannel, politica TLS și accesul driverului la cheia tokenului"},
	} {
		if flags&item.mask != 0 {
			reasons = append(reasons, item.message)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "Windows nu a furnizat o cauză TLS detaliată; verifică jurnalul System/Schannel")
	}
	return fmt.Sprintf("TLS 12175 (flags=0x%08X): %s. Conexiunea HTTPS nu a putut fi verificată", flags, strings.Join(reasons, "; "))
}
