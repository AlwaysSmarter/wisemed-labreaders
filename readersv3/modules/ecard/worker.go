package ecard

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (m *Module) run(parent context.Context, id string) {
	m.mu.Lock()
	var state string
	var blob []byte
	var total int
	var owner string
	e := m.store.db.QueryRow(`SELECT state,request,total,owner FROM jobs WHERE id=?`, id).Scan(&state, &blob, &total, &owner)
	if e != nil || state != "queued" {
		m.mu.Unlock()
		return
	}
	settings := m.settings
	ctx, cancel := context.WithTimeout(parent, time.Duration(settings.JobTimeout)*time.Second)
	a := &activeJob{owner: owner, cancel: cancel, pin: make(chan string, 1)}
	m.active[id] = a
	_ = m.store.state(id, "running", "")
	m.mu.Unlock()
	m.emitEvent(owner, id, "worker", "operation_started", map[string]interface{}{"report_version": 1, "sdk_order": settings.SDKOrder})
	final, code := "failed", "internal_error"
	defer func() {
		cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = m.store.state(id, final, code)
		m.emitEvent(owner, id, "worker", "operation_finished", map[string]string{"state": final, "code": code})
		delete(m.active, id)
	}()

	var req Request
	if e = m.store.open(blob, id, &req); e != nil {
		code = "payload_decryption_failed"
		return
	}
	if req.Operation == "activate" || req.Operation == "change-pin" || req.Operation == "reset-pin" {
		m.rememberedPIN.clear()
	}
	cache := &operationPIN{}
	m.mu.Lock()
	a.cachedPIN = cache
	m.mu.Unlock()
	defer cache.clear()
	probe := m.continuity
	if probe == nil {
		probe = readerContinuity
	}
	selectedReader := settings.Reader
	continuityError := ""
	identity := func() string {
		stamp, err := probe(selectedReader)
		continuityError = ""
		if err != nil {
			continuityError = err.Error()
			return ""
		}
		if stamp == "" {
			continuityError = "empty card continuity stamp"
			return ""
		}
		return strconv.FormatUint(m.cardEpoch.Load(), 10) + ":" + stamp
	}
	currentSDK := ""
	inputKind := ""
	var candidateRevision uint64
	reusable := func() bool {
		return (req.Operation == "read" || req.Operation == "sign" || req.Operation == "certificates" || req.Operation == "write") && (inputKind == "" || inputKind == "AUTENTIFICARE")
	}
	promote := func() {
		value, stamp := cache.candidate()
		if value != "" && stamp != "" && stamp == identity() {
			m.rememberedPIN.remember(value, owner, stamp, candidateRevision)
			cache.clear()
		}
	}
	pin := func(pinCtx context.Context, prompt string) (string, error) {
		if pinCtx.Err() != nil {
			cache.clear()
			return "", pinCtx.Err()
		}
		before := identity()
		// CNAS can authenticate twice inside open before emitting a success event
		// or a final reply. Reuse the pending PIN while continuity is intact and
		// no PIN/authentication failure has invalidated it.
		if reusable() {
			promote()
		}
		value, revision := m.rememberedPIN.lookup(owner, before)
		candidateRevision = revision
		if reusable() && value != "" {
			m.mu.Lock()
			a.pinSeen = true
			m.mu.Unlock()
			m.emitEvent(owner, id, currentSDK, "pin_reused", nil)
			return value, nil
		}
		// Unavailable continuity or a non-authentication input requires manual entry.
		cache.clear()

		m.mu.Lock()
		a.pinSeen = true
		a.prompt = prompt
		a.challenge = uuid.NewString()
		_ = m.store.state(id, "waiting_pin", "")
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			a.prompt = ""
			a.challenge = ""
			_ = m.store.state(id, "running", "")
			m.mu.Unlock()
		}()
		reason := "no_cached_pin"
		if before == "" {
			reason = "card_continuity_unavailable"
		} else if !reusable() {
			reason = "explicit_input_required"
		}
		m.emitEvent(owner, id, currentSDK, "pin_required", map[string]string{"prompt": prompt, "reason": reason, "input_kind": inputKind, "continuity_error": continuityError, "reader": selectedReader})
		select {
		case v := <-a.pin:
			if pinCtx.Err() != nil {
				return "", pinCtx.Err()
			}
			after := identity()
			if reusable() && before != "" && before == after {
				cache.save(v, after)
			}
			return v, nil
		case <-pinCtx.Done():
			return "", pinCtx.Err()
		}
	}
	call := func(b backend, p map[string]interface{}) (Reply, error) {
		c, stop := context.WithTimeout(ctx, time.Duration(settings.OperationTimeout)*time.Second)
		defer stop()
		callInfo := map[string]interface{}{"action": p["action"]}
		for _, key := range []string{"item_id", "encoding", "certificate"} {
			if value, ok := p[key]; ok {
				callInfo[key] = value
			}
		}
		inputKind = ""
		m.emitEvent(owner, id, currentSDK, "call_started", callInfo)
		reply, err := b.Call(c, p, func(prompt string) (string, error) { return pin(c, prompt) })
		if !reply.OK && (reply.Message == "" || reply.Code == "selected_reader_not_connected_or_supported") {
			reply.Message = errorDescription(reply.Code, reply.SDKCode)
		}
		if invalidatesPIN(reply) {
			cache.clear()
			m.rememberedPIN.clear()
		} else if err == nil {
			promote()
		} else {
			cache.clear()
		}
		outcome := map[string]interface{}{"action": p["action"], "response": reply}
		if value, ok := p["item_id"]; ok {
			outcome["item_id"] = value
		}
		if err != nil {
			outcome["transport_error"] = err.Error()
		}
		m.emitEvent(owner, id, currentSDK, "call_finished", outcome)
		return reply, err
	}
	var b backend
	var answer Reply
	for index, sdk := range settings.SDKOrder {
		if ctx.Err() != nil {
			final = "cancelled"
			code = "cancelled_before_operation"
			return
		}
		previousSDK := currentSDK
		currentSDK = sdk
		m.mu.Lock()
		a.sdk = sdk
		m.mu.Unlock()
		kind := "sdk_selected"
		if previousSDK != "" {
			kind = "sdk_changed"
		}
		reason := "configured_sdk_order"
		if previousSDK != "" {
			reason = "previous_sdk_failed"
		}
		m.emitEvent(owner, id, sdk, kind, map[string]interface{}{"previous_sdk": previousSDK, "sdk": sdk, "attempt": index + 1, "reason": reason, "previous_error": answer.Code, "previous_sdk_code": answer.SDKCode})
		b, e = m.factory(ctx, sdk, settings)
		if e != nil {
			answer = Reply{Code: "sdk_unavailable", Message: errorDescription("sdk_unavailable", 0)}
			m.emitEvent(owner, id, sdk, "host_start_failed", map[string]interface{}{"response": answer, "transport_error": e.Error()})
			e = nil
		} else {
			if eventBackend, ok := b.(interface{ SetEventSink(func(Reply)) }); ok {
				eventBackend.SetEventSink(func(r Reply) {
					if r.Event == "reader_selected" {
						var selected struct {
							Reader string `json:"reader"`
						}
						if json.Unmarshal(r.Data, &selected) == nil && selected.Reader != "" {
							selectedReader = selected.Reader
						}
					}
					if r.Event == "pin" {
						inputKind = r.InputKind
						if inputKind != "" && inputKind != "AUTENTIFICARE" {
							cache.clear()
							m.rememberedPIN.clear()
						}
						return
					}
					if r.Event == "StareAutentificareSchimbata" {
						state := string(r.Data)
						if strings.Contains(state, "AUTENTIFICARE_ESUATA") {
							cache.clear()
							m.rememberedPIN.clear()
						}
						if strings.Contains(state, "\"AUTENTIFICAT\"") || strings.Contains(state, "\"AUTENTIFICAT_OFFLINE\"") {
							promote()
						}
					}
					m.emitEvent(owner, id, sdk, r.Event, r.Data)
				})
			}
			m.emitEvent(owner, id, sdk, "sdk_open_started", map[string]interface{}{"operation": req.Operation, "fields": req.Fields})
			_, e = m.store.db.Exec(`UPDATE jobs SET sdk=? WHERE id=?`, sdk, id)
			if e != nil {
				b.Close()
				code = "storage_failed"
				return
			}
			answer, e = call(b, map[string]interface{}{"action": "open", "reader": settings.Reader, "um_host": settings.UMHost, "um_port": settings.UMPort, "request": req})
			if e == nil && answer.OK {
				if err := m.saveSDKAttempt(id, index, sdk, req, answer, nil, false); err != nil {
					b.Close()
					code = "attempt_storage_failed"
					return
				}
				break
			}
			b.Close()
			b = nil
		}
		retry := ctx.Err() == nil && (e == nil || req.Operation == "read") && canFallback(req.Operation, answer, a.pinSeen) && index < len(settings.SDKOrder)-1
		if err := m.saveSDKAttempt(id, index, sdk, req, answer, e, retry); err != nil {
			code = "attempt_storage_failed"
			return
		}
		// Read-only diagnostics may retry with a fresh session after any error.
		// Signing and mutations retain the strict no-replay policy.
		if !retry {
			if e != nil {
				code = "host_start_failed"
			} else {
				code = answer.Code
			}
			return
		}
	}
	if b == nil {
		code = "no_sdk_available"
		return
	}
	defer b.Close()
	if req.Operation == "sign" {
		var actual Identity
		if e = json.Unmarshal(answer.Data, &actual); e != nil {
			code = "card_identity_unavailable"
			return
		}
		if e = matchIdentity(req.Expected, actual); e != nil {
			code = e.Error()
			return
		}
		for n := 0; n < total; n++ {
			if ctx.Err() != nil {
				final = "cancelled"
				code = "remaining_items_not_signed"
				return
			}
			var payload []byte
			if e = m.store.db.QueryRow(`SELECT payload FROM items WHERE job=? AND n=?`, id, n).Scan(&payload); e != nil {
				code = "storage_failed"
				return
			}
			var item Item
			if e = m.store.open(payload, id+":"+strconv.Itoa(n), &item); e != nil {
				code = "payload_decryption_failed"
				return
			}
			// Host pins the card generation and certificate for this session, and checks
			// them before AND after each signature. No SDK switch after open succeeds.
			answer, e = call(b, map[string]interface{}{"action": "sign", "item_id": item.ID, "text": item.Text, "encoding": req.Encoding, "certificate": req.Certificate})
			if e != nil {
				final = "unknown"
				code = "signature_result_unknown_no_replay"
				return
			}
			if !answer.OK {
				code = answer.Code
				_ = m.saveResult(id, n, map[string]interface{}{"id": item.ID, "ok": false, "code": answer.Code, "sdk_code": answer.SDKCode}, false)
				return
			}
			if e = m.saveResult(id, n, map[string]interface{}{"id": item.ID, "ok": true, "signature": answer.Data}, true); e != nil {
				final = "unknown"
				code = "signature_not_persisted_no_replay"
				return
			}
		}
	} else {
		// 'open' executes reads only; mutations require a second explicit command.
		if req.Operation == "write" || req.Operation == "activate" || req.Operation == "change-pin" || req.Operation == "reset-pin" || req.Operation == "enroll" {
			answer, e = call(b, map[string]interface{}{"action": "execute", "request": req})
			if e != nil {
				final = "unknown"
				code = "operation_result_unknown_no_replay"
				return
			}
		}
		if !answer.OK {
			code = answer.Code
			return
		}
		if _, e = m.store.db.Exec(`INSERT INTO items(job,n,id,payload) VALUES(?,0,'result',?)`, id, []byte{}); e != nil {
			code = "storage_failed"
			return
		}
		if e = m.saveResult(id, 0, answer, true); e != nil {
			code = "result_storage_failed"
			return
		}
	}
	final = "completed"
	code = ""
}
func (m *Module) saveResult(id string, n int, v interface{}, completed bool) error {
	b, e := m.store.seal(v, id+":result:"+strconv.Itoa(n))
	if e != nil {
		return e
	}
	tx, e := m.store.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.Exec(`UPDATE items SET result=? WHERE job=? AND n=?`, b, id, n)
	if e != nil {
		return e
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("result item missing")
	}
	if completed {
		_, e = tx.Exec(`UPDATE jobs SET completed=completed+1 WHERE id=?`, id)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
