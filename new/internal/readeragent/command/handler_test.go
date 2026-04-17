package command

import (
	"path/filepath"
	"testing"

	"wisemed-labreaders/new/internal/readeragent/analyzer"
	"wisemed-labreaders/new/internal/readeragent/storage"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "reader.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ad, err := analyzer.GetAdapter("maglumi-800")
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{ReaderID: "r1", Adapter: ad, Store: store}
}

func TestHandlePing(t *testing.T) {
	h := newTestHandler(t)
	ok, data, errText := h.Handle("ping", nil)
	if !ok || errText != "" {
		t.Fatalf("expected success, got ok=%v err=%q", ok, errText)
	}
	if data["pong"] != true {
		t.Fatalf("expected pong=true")
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	h := newTestHandler(t)
	ok, _, errText := h.Handle("nope", nil)
	if ok || errText == "" {
		t.Fatalf("expected error for unknown command")
	}
}

func TestSetCommConfig_ValidNetwork(t *testing.T) {
	h := newTestHandler(t)
	ok, data, errText := h.Handle("set_comm_config", map[string]interface{}{
		"transport": "network",
		"mode":      "server",
		"settings": map[string]interface{}{
			"ip":   "127.0.0.1",
			"port": 5004,
		},
	})
	if !ok || errText != "" {
		t.Fatalf("expected success, got ok=%v err=%q", ok, errText)
	}
	if data["saved"] != true {
		t.Fatalf("expected saved=true")
	}
}

func TestSetCommConfig_InvalidNetworkPort(t *testing.T) {
	h := newTestHandler(t)
	ok, _, errText := h.Handle("set_comm_config", map[string]interface{}{
		"transport": "network",
		"mode":      "server",
		"settings": map[string]interface{}{
			"ip":   "127.0.0.1",
			"port": 70000,
		},
	})
	if ok || errText == "" {
		t.Fatalf("expected validation error for invalid port")
	}
}
