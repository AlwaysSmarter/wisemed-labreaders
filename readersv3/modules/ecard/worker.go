package ecard

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (m *Module) run(parent context.Context, id string) {
	m.mu.Lock()
	var state string
	var blob []byte
	var total int
	e := m.store.db.QueryRow(`SELECT state,request,total FROM jobs WHERE id=?`, id).Scan(&state, &blob, &total)
	if e != nil || state != "queued" {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(m.settings.JobTimeout)*time.Second)
	a := &activeJob{cancel: cancel, pin: make(chan string, 1)}
	m.active[id] = a
	_ = m.store.state(id, "running", "")
	m.mu.Unlock()
	final, code := "failed", "internal_error"
	defer func() { cancel(); m.mu.Lock(); delete(m.active, id); _ = m.store.state(id, final, code); m.mu.Unlock() }()
	var req Request
	if e = m.store.open(blob, id, &req); e != nil {
		code = "payload_decryption_failed"
		return
	}
	pin := func(pinCtx context.Context, prompt string) (string, error) {
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
		select {
		case v := <-a.pin:
			return v, nil
		case <-pinCtx.Done():
			return "", pinCtx.Err()
		}
	}
	call := func(b backend, p map[string]interface{}) (Reply, error) {
		c, stop := context.WithTimeout(ctx, time.Duration(m.settings.OperationTimeout)*time.Second)
		defer stop()
		return b.Call(c, p, func(prompt string) (string, error) { return pin(c, prompt) })
	}
	var b backend
	var answer Reply
	for index, sdk := range m.settings.SDKOrder {
		if ctx.Err() != nil {
			final = "cancelled"
			code = "cancelled_before_operation"
			return
		}
		b, e = m.factory(ctx, sdk, m.settings)
		if e != nil {
			answer = Reply{Code: "sdk_unavailable"}
			e = nil
		} else {
			_, e = m.store.db.Exec(`UPDATE jobs SET sdk=? WHERE id=?`, sdk, id)
			if e != nil {
				b.Close()
				code = "storage_failed"
				return
			}
			answer, e = call(b, map[string]interface{}{"action": "open", "reader": m.settings.Reader, "um_host": m.settings.UMHost, "um_port": m.settings.UMPort, "request": req})
			if e == nil && answer.OK {
				break
			}
			b.Close()
			b = nil
		}
		// Startup/parse fallback is only allowed before any PIN interaction, including
		// physical PIN pad input reported by the host. No ambiguous timeouts retry.
		if e != nil || !canFallback(req.Operation, answer, a.pinSeen) || index == len(m.settings.SDKOrder)-1 {
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
			answer, e = call(b, map[string]interface{}{"action": "sign", "text": item.Text, "encoding": req.Encoding, "certificate": req.Certificate})
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
