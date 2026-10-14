package siui

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type invoiceEndpointBackend struct {
	operationBackend
	urls []string
}

func (b *invoiceEndpointBackend) Do(ctx context.Context, method, address string, h http.Header, body []byte) (int, http.Header, []byte, error) {
	b.urls = append(b.urls, address)
	return b.operationBackend.Do(ctx, method, address, h, body)
}
func TestInvoiceUsesSeparateOriginAndSIUIOCSP(t *testing.T) {
	b := &invoiceEndpointBackend{operationBackend: operationBackend{t: t}}
	c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://www.siui.ro:444", EInvoiceBaseURL: "https://www.siui.ro:443", PEBaseURL: "https://sipe.siui.ro"}, backend: b}
	op, _ := findOperation("facturare", "getEInvoiceId")
	_, _, err := c.invokeOperation(context.Background(), op, map[string]string{"serialCode": "TEST", "serialNumber": "1", "providerFiscalCode": "123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.urls) != 2 || !strings.HasPrefix(b.urls[0], "https://www.siui.ro:444/OCSP/") || b.urls[1] != "https://www.siui.ro:443/svapntws/services/SiuiEInvoiceWS" {
		t.Fatal(b.urls)
	}
	if c.cfg.serviceBaseURL("EPrescriptionWS") != "https://sipe.siui.ro" || c.cfg.serviceBaseURL("SiuiWS") != c.cfg.BaseURL {
		t.Fatal("other endpoints changed")
	}
	c.cfg.EInvoiceBaseURL = ""
	if c.cfg.serviceBaseURL("SiuiEInvoiceWS") != c.cfg.BaseURL {
		t.Fatal("legacy fallback")
	}
}
