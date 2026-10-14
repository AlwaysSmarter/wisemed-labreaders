package siui

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type ocspLogBackend struct {
	events *[]string
	token  string
	status int
}

func (b ocspLogBackend) Validate(context.Context, string, []byte) error { return nil }
func (b ocspLogBackend) Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error) {
	*b.events = append(*b.events, "OCSP response")
	status := b.status
	if status == 0 {
		status = 200
	}
	return status, http.Header{"Oscp_response": []string{b.token}}, nil, nil
}
func TestOCSPSuccessLoggedForEverySessionBeforeReturning(t *testing.T) {
	var events []string
	b := ocspLogBackend{events: &events, token: "secret-session"}
	c := client{cfg: settings{Username: "test", Licence: "secret-licence", BaseURL: "https://example.test"}, backend: b, operation: "sendEInvoice", callID: "call-123", logf: func(format string, args ...interface{}) {
		message := fmt.Sprintf(format, args...)
		if strings.Contains(message, "SIUI OCSP: conectare") || strings.Contains(message, "SIUI OCSP: durata") || strings.Contains(message, "folosesc token-ul") {
			return
		}
		if !strings.Contains(message, "token obtinut cu succes") || !strings.Contains(message, "operation=sendEInvoice id=call-123") {
			t.Error(message)
		}
		if strings.Contains(message, "secret-") {
			t.Fatal("credential logged")
		}
		events = append(events, "success log")
	}}
	for i := 0; i < 2; i++ {
		if _, err := c.sessionHeaders(context.Background()); err != nil {
			t.Fatal(err)
		}
		events = append(events, "ready for SOAP")
	}
	expected := []string{"OCSP response", "success log", "ready for SOAP", "OCSP response", "success log", "ready for SOAP"}
	if !reflect.DeepEqual(events, expected) {
		t.Fatal(events)
	}
	for _, token := range []string{"", "invalid\r\ntoken"} {
		events = nil
		b.token = token
		c.backend = b
		c.logf = func(format string, args ...interface{}) {
			message := fmt.Sprintf(format, args...)
			if strings.Contains(message, "SIUI OCSP: conectare") || strings.Contains(message, "SIUI OCSP: durata") || strings.Contains(message, "folosesc token-ul") {
				return
			}
			if !strings.Contains(message, "[ERROR]") || strings.Contains(message, "secret-session") {
				t.Fatal(message)
			}
			events = append(events, "error log")
		}
		if _, err := c.sessionHeaders(context.Background()); err == nil {
			t.Fatal("expected invalid token")
		}
		if !reflect.DeepEqual(events, []string{"OCSP response", "error log"}) {
			t.Fatal("false success", events)
		}
	}
}

func TestOCSPHTTPFailureLoggedWithoutSuccess(t *testing.T) {
	var events, logs []string
	c := client{cfg: settings{Username: "test", Licence: "test", BaseURL: "https://example.test"}, backend: ocspLogBackend{events: &events, status: 471}, operation: "getInsured", logf: func(format string, args ...interface{}) {
		if !strings.Contains(format, "SIUI OCSP: durata") {
			logs = append(logs, fmt.Sprintf(format, args...))
		}
	}}
	if _, err := c.sessionHeaders(context.Background()); err == nil || !strings.Contains(err.Error(), "CNAS authentication HTTP 471") {
		t.Fatal(err)
	}
	if len(logs) != 2 || !strings.Contains(logs[1], "HTTP=471 operation=getInsured") || !strings.Contains(logs[1], "token neobtinut") {
		t.Fatal(logs)
	}
}
