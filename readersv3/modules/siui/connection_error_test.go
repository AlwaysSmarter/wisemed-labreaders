package siui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type failingConnectionBackend struct{ ocsp bool }

func (b failingConnectionBackend) Validate(context.Context, string, []byte) error { return nil }
func (b failingConnectionBackend) Do(_ context.Context, _ string, address string, _ http.Header, _ []byte) (int, http.Header, []byte, error) {
	if strings.Contains(address, "/OCSP/") && !b.ocsp {
		return 200, http.Header{"Oscp_response": []string{"test-session"}}, nil, nil
	}
	return 0, nil, nil, errors.New("connection timed out")
}
func TestErrorReportsFailingConnectionRatherThanInvoiceDestination(t *testing.T) {
	for _, ocsp := range []bool{true, false} {
		c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://www.siui.ro", EInvoiceBaseURL: "https://www.siui.ro:444"}, backend: failingConnectionBackend{ocsp: ocsp}}
		op, _ := findOperation("facturare", "getEInvoice")
		_, _, err := c.invokeOperation(context.Background(), op, map[string]string{"serialCode": "TEST", "serialNumber": "1", "providerFiscalCode": "123"})
		expected := "Server: https://www.siui.ro:444/svapntws/services/SiuiEInvoiceWS\nPort: 444\nconnection timed out"
		if ocsp {
			expected = "Server: https://www.siui.ro/OCSP/validator\nPort: 443\nconnection timed out"
		}
		if err == nil || err.Error() != expected {
			t.Fatal(ocsp, err)
		}
	}
}
func TestConnectionErrorHidesDownloadQueryAndKeepsCause(t *testing.T) {
	cause := errors.New("download failed")
	err := withConnectionError("https://example.test:444/invoice.zip?token=secret", cause)
	if strings.Contains(err.Error(), "secret") || !errors.Is(err, cause) || !strings.Contains(err.Error(), "Port: 444") {
		t.Fatal(err)
	}
	if withConnectionError("https://other.test", err) != err {
		t.Fatal("original destination replaced")
	}
}
