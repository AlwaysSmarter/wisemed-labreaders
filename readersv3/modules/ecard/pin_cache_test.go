package ecard

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

type pinRetryBackend struct {
	reply Reply
	got   *string
}

func (b *pinRetryBackend) Close() {}
func (b *pinRetryBackend) Call(ctx context.Context, _ map[string]interface{}, pin func(string) (string, error)) (Reply, error) {
	v, e := pin("PIN")
	*b.got = v
	return b.reply, e
}
func TestPINReuseBetweenSDKsAndInvalidation(t *testing.T) {
	for _, kind := range []string{"reuse", "wrong", "removed", "unavailable"} {
		t.Run(kind, func(t *testing.T) {
			m := testModule(t)
			m.continuity = func(string) (string, error) { return "reader:1:atr", nil }
			req := testRequest()
			req.Operation = "read"
			seed(t, m.store, "job", req, 0)
			count := 0
			values := make([]string, 2)
			m.factory = func(context.Context, string, Settings) (backend, error) {
				i := count
				count++
				if i == 0 {
					m.active["job"].pin <- "1234"
				} else if kind != "reuse" {
					m.active["job"].pin <- "5678"
				}
				if i == 1 && kind == "removed" {
					m.emitEvent("", "", "pcsc", "card_removed", nil)
				}
				if i == 1 && kind == "unavailable" {
					m.continuity = nil /* probe is captured; invalidate via monitor */
					m.emitEvent("", "", "pcsc", "monitor_unavailable", nil)
				}
				reply := Reply{OK: i == 1, Code: "sdk_error", SDKCode: -999}
				if i == 0 && kind == "wrong" {
					reply.SDKCode = -14
				}
				return &pinRetryBackend{reply: reply, got: &values[i]}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			m.run(ctx, "job")
			if kind == "wrong" {
				if count != 1 || values[1] != "" {
					t.Fatal("incorrect PIN retried on another SDK")
				}
				if value, _ := m.rememberedPIN.lookup("", "reader:1:atr"); value != "" {
					t.Fatal("incorrect PIN retained")
				}
				return
			}
			expected := "1234"
			if kind != "reuse" {
				expected = "5678"
			}
			if count != 2 || values[0] != "1234" || values[1] != expected {
				t.Fatalf("attempts=%d values=%v", count, values)
			}

		})
	}
}
func TestPendingPINWipes(t *testing.T) {
	p := &operationPIN{}
	p.save("1234", "card")
	buf := p.value
	p.clear()
	for _, b := range buf {
		if b != 0 {
			t.Fatal("buffer not wiped")
		}
	}
	if value, _ := p.candidate(); value != "" {
		t.Fatal("cleared cache reused")
	}
	for _, code := range []int{-11, -14, -15} {
		if !invalidatesPIN(Reply{SDKCode: code}) {
			t.Fatal(code)
		}
	}
}

// Exercise the public worker flow, including separate jobs and host input modes.
type sessionPINBackend struct {
	sink   func(Reply)
	kind   string
	before func()
	reply  Reply
	got    *[]string
}

func (b *sessionPINBackend) SetEventSink(s func(Reply)) { b.sink = s }
func (b *sessionPINBackend) Close()                     {}
func (b *sessionPINBackend) Call(ctx context.Context, _ map[string]interface{}, pin func(string) (string, error)) (Reply, error) {
	if b.before != nil {
		b.before()
	}
	b.sink(Reply{Event: "pin", InputKind: b.kind})
	value, err := pin("PIN")
	*b.got = append(*b.got, value)
	return b.reply, err
}
func TestPINAcrossOperations(t *testing.T) {
	for _, kind := range []string{"read", "certificates", "write", "removed", "reinserted", "owner", "wrong", "auth_failed", "new_pin", "continuity_lost"} {
		t.Run(kind, func(t *testing.T) {
			m := testModule(t)
			m.settings.SDKOrder = []string{"2.0.0.4"}
			stamp := "reader:1:atr"
			m.continuity = func(string) (string, error) { return stamp, nil }
			values := []string{}
			for n, id := range []string{"first", "second", "third"} {
				req := testRequest()
				req.Operation = "read"
				if n == 1 && (kind == "certificates" || kind == "write") {
					req.Operation = kind
				}
				seed(t, m.store, id, req, 0)
				if n == 1 {
					switch kind {
					case "removed":
						m.emitEvent("", "", "pcsc", "card_removed", nil)
					case "reinserted":
						stamp = "reader:3:atr"
					case "owner":
						_, _ = m.store.db.Exec(`UPDATE jobs SET owner='other' WHERE id=?`, id)
					case "continuity_lost":
						stamp = ""
					}
				}
				m.factory = func(context.Context, string, Settings) (backend, error) {
					// Queue a manual value even on cache hits; a new job discards unused input.
					manual := "5678"
					if n == 0 {
						manual = "1234"
					}
					m.active[id].pin <- manual
					inputKind := "AUTENTIFICARE"
					reply := Reply{OK: true}
					if n == 1 && kind == "wrong" {
						reply = Reply{Code: "invalid_pin", SDKCode: -14}
					}
					if n == 1 && kind == "new_pin" {
						inputKind = "SCHIMBARE_PIN"
					}
					b := &sessionPINBackend{kind: inputKind, reply: reply, got: &values}
					if n == 1 && kind == "auth_failed" {
						b.before = func() {
							b.sink(Reply{Event: "StareAutentificareSchimbata", Data: []byte(`{"states":["AUTENTIFICARE_ESUATA"]}`)})
						}
					}
					return b, nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				m.run(ctx, id)
				cancel()
			}
			expectedSecond := "1234"
			switch kind {
			case "removed", "reinserted", "owner", "new_pin", "continuity_lost", "auth_failed":
				expectedSecond = "5678"
			}
			if len(values) < 3 || values[0] != "1234" || values[1] != expectedSecond {
				t.Fatalf("PINs=%v", values)
			}
			if (kind == "wrong" || kind == "new_pin" || kind == "owner") && values[len(values)-1] != "5678" {
				t.Fatalf("invalidated PIN reused: %v", values)
			}
		})
	}
}
func TestCardPINInvalidationAndWiping(t *testing.T) {
	p := &cardPIN{}
	_, rev := p.lookup("owner", "card")
	p.remember("1234", "owner", "card", rev)
	old := p.value
	_, rev = p.lookup("owner", "card")
	p.clear()
	p.remember("1234", "owner", "card", rev)
	if value, _ := p.lookup("owner", "card"); value != "" {
		t.Fatal("in-flight PIN resurrected")
	}
	for _, b := range old {
		if b != 0 {
			t.Fatal("secret buffer not wiped")
		}
	}
}

// The CNAS wrapper may request authentication twice before returning from open.
// No intermediate authentication-success event is required for PIN reuse.
type batchPINBackend struct {
	selectedReader string
	sink           func(Reply)
	between        func()
	values         []string
}

func (b *batchPINBackend) SetEventSink(s func(Reply)) { b.sink = s }
func (b *batchPINBackend) Close()                     {}
func (b *batchPINBackend) Call(ctx context.Context, request map[string]interface{}, pin func(string) (string, error)) (Reply, error) {
	prompts := 1
	if request["action"] == "open" {
		prompts = 2
		if b.selectedReader != "" {
			data, _ := json.Marshal(map[string]string{"reader": b.selectedReader})
			b.sink(Reply{Event: "reader_selected", Data: data})
		}
	}
	for i := 0; i < prompts; i++ {
		if i == 1 && b.between != nil {
			b.between()
		}
		b.sink(Reply{Event: "pin", InputKind: "AUTENTIFICARE"})
		value, err := pin("PIN-ul dvs.?")
		if err != nil {
			return Reply{}, err
		}
		b.values = append(b.values, value)
	}
	if request["action"] == "open" {
		return Reply{OK: true, Data: []byte(`{"cid":"cid"}`)}, nil
	}
	return Reply{OK: true, Data: []byte(`"test-signature"`)}, nil
}
func TestBatchPINRepeatedAuthenticationBeforeOpenReturns(t *testing.T) {
	for _, kind := range []string{"reuse", "auth_failed", "removed", "new_card"} {
		t.Run(kind, func(t *testing.T) {
			m := testModule(t)
			m.settings.SDKOrder = []string{"cnas-wrapper-1.0.0.0"}
			stamp := "reader:1:atr"
			m.continuity = func(string) (string, error) {
				state := uint32(0x22)
				if stamp != "reader:1:atr" {
					state |= 2 << 16
				}
				return cardContinuityStamp("reader", state, 2, []byte{0x3b, 0x01})
			}
			seed(t, m.store, "batch", testRequest(), 2)
			b := &batchPINBackend{}
			m.factory = func(context.Context, string, Settings) (backend, error) {
				m.active["batch"].pin <- "1234"
				if kind != "reuse" {
					b.between = func() {
						switch kind {
						case "auth_failed":
							b.sink(Reply{Event: "StareAutentificareSchimbata", Data: []byte(`{"states":["AUTENTIFICARE_ESUATA"]}`)})
						case "removed":
							m.emitEvent("", "", "pcsc", "card_removed", nil)
						case "new_card":
							stamp = "reader:3:atr"
						}
						m.active["batch"].pin <- "5678"
					}
				}
				return b, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			m.run(ctx, "batch")
			expected := "1234"
			if kind != "reuse" {
				expected = "5678"
			}
			if len(b.values) != 4 || b.values[0] != "1234" {
				t.Fatalf("authentication calls=%v", b.values)
			}
			for _, value := range b.values[1:] {
				if value != expected {
					t.Fatalf("authentication values=%v", b.values)
				}
			}
			var state string
			if err := m.store.db.QueryRow(`SELECT state FROM jobs WHERE id='batch'`).Scan(&state); err != nil || state != "completed" {
				t.Fatalf("batch state=%s error=%v", state, err)
			}
		})
	}
}

func TestBatchPINUsesResolvedPhysicalReader(t *testing.T) {
	for _, viaEvent := range []bool{false, true} {
		t.Run(fmt.Sprint(viaEvent), func(t *testing.T) {
			m := testModule(t)
			m.settings.SDKOrder = []string{"cnas-wrapper-1.0.0.0"}
			m.settings.Reader = "Gemalto USB Smart Card Reader"
			physical := "Gemalto USB Smart Card Reader 0"
			m.continuity = func(selected string) (string, error) {
				if viaEvent && selected != physical {
					return "", fmt.Errorf("selected reader unavailable")
				}
				actual, err := continuityReader(selected, []string{physical})
				if err != nil {
					return "", err
				}
				return cardContinuityStamp(actual, 0x22, 2, []byte{0x3b, 1})
			}
			b := &batchPINBackend{}
			if viaEvent {
				b.selectedReader = physical
			}
			seed(t, m.store, "batch", testRequest(), 2)
			m.factory = func(context.Context, string, Settings) (backend, error) {
				m.active["batch"].pin <- "1234"
				return b, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			m.run(ctx, "batch")
			if len(b.values) != 4 {
				t.Fatalf("PIN was requested again: %v", b.values)
			}
			for _, value := range b.values {
				if value != "1234" {
					t.Fatal("PIN not reused")
				}
			}
			var state string
			if err := m.store.db.QueryRow(`SELECT state FROM jobs WHERE id='batch'`).Scan(&state); err != nil || state != "completed" {
				t.Fatalf("state=%s error=%v", state, err)
			}
		})
	}
}
