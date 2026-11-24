package ecard

import "sync"

// A pending PIN for one operation; never serialized.
type operationPIN struct {
	mu       sync.Mutex
	value    []byte
	identity string
}

func (p *operationPIN) clear() { p.mu.Lock(); defer p.mu.Unlock(); p.clearLocked() }
func (p *operationPIN) clearLocked() {
	for i := range p.value {
		p.value[i] = 0
	}
	p.value = nil
	p.identity = ""
}
func (p *operationPIN) save(value, identity string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearLocked()
	if identity == "" {
		return
	}
	p.value = []byte(value)
	p.identity = identity
}
func invalidatesPIN(r Reply) bool {
	switch r.SDKCode {
	case -11, -14, -15, -28, -40, -41, -42, -52, -61:
		return true
	}
	switch r.Code {
	case "invalid_pin", "card_blocked", "card_changed", "card_changed_during_identity_read":
		return true
	}
	return false
}

// cardPIN survives jobs, but only for one principal and uninterrupted card presence.
// revision prevents an in-flight operation from restoring an invalidated secret.
type cardPIN struct {
	mu              sync.Mutex
	value           []byte
	identity, owner string
	revision        uint64
}

func (p *cardPIN) clearLocked() {
	for i := range p.value {
		p.value[i] = 0
	}
	p.value = nil
	p.identity = ""
	p.owner = ""
	p.revision++
}
func (p *cardPIN) clear() { p.mu.Lock(); defer p.mu.Unlock(); p.clearLocked() }
func (p *cardPIN) lookup(owner, identity string) (string, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if identity == "" || (len(p.value) > 0 && (p.identity != identity || p.owner != owner)) {
		p.clearLocked()
	}
	return string(p.value), p.revision
}
func (p *cardPIN) remember(value, owner, identity string, revision uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if revision != p.revision || identity == "" || value == "" {
		return
	}
	p.clearLocked()
	p.value = []byte(value)
	p.owner = owner
	p.identity = identity
}
func (p *operationPIN) candidate() (string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.value), p.identity
}
