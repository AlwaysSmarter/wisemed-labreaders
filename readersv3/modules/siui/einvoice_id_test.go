package siui

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type invoiceIDBackend struct{ values []string }

func (b invoiceIDBackend) Validate(context.Context, string, []byte) error { return nil }
func (b invoiceIDBackend) Do(_ context.Context, _ string, address string, _ http.Header, _ []byte) (int, http.Header, []byte, error) {
	if strings.Contains(address, "/OCSP/") {
		return 200, http.Header{"Oscp_response": []string{"token"}}, nil, nil
	}
	op, _ := findOperation("facturare", "getEInvoiceId")
	body := `<s:Envelope xmlns:s="` + soapNS + `"><s:Body><getEInvoiceIdResponse xmlns="` + op.Namespace + `">`
	for _, v := range b.values {
		body += `<getEInvoiceIdReturn>` + v + `</getEInvoiceIdReturn>`
	}
	body += `</getEInvoiceIdResponse></s:Body></s:Envelope>`
	return 200, nil, []byte(body), nil
}
func TestInvoiceIDValidationMessages(t *testing.T) {
	const message = "Factura are asociate raportări/deconturi dar nu a fost validată."
	for _, tc := range []struct {
		name   string
		values []string
		failed bool
	}{
		{"two values accepted", []string{"1516712440813", ""}, false},
		{"three values accepted", []string{"1516712440813", "", ""}, false},
		{"documented error", []string{"1516712440813", message}, true},
		{"live SIUI response", []string{"1516712440813", "", message}, true},
		{"numeric second value", []string{"1516712440813", "1516712462512", ""}, false},
		{"numeric second value with validation error", []string{"1516712440813", "1516712462512", message}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://example.test"}, backend: invoiceIDBackend{tc.values}}
			op, _ := findOperation("facturare", "getEInvoiceId")
			values, submitted, err := c.invokeOperation(context.Background(), op, map[string]string{"serialCode": "TEST", "serialNumber": "1", "providerFiscalCode": "123"})
			if submitted || (err != nil) != tc.failed {
				t.Fatal(values, submitted, err)
			}
			if tc.failed {
				if values != nil || !strings.Contains(err.Error(), message) {
					t.Fatal("invalid ID exposed or message lost", values, err)
				}
			} else if !reflect.DeepEqual(values, tc.values) {
				t.Fatal(values)
			}
		})
	}
}
