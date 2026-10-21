package siui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type pinOutcome uint8

const (
	pinUnrelated pinOutcome = iota
	pinSuccess
	pinRejected
)
const pinFailureLimit = 3

func signingPINOutcome(rejected bool, err error) pinOutcome {
	if rejected {
		return pinRejected
	}
	if err == nil {
		return pinSuccess
	}
	return pinUnrelated
}

type pinRecord struct {
	Sealed   []byte `json:"sealed"`
	Blocked  bool   `json:"blocked"`
	Failures int    `json:"failures"`
}
type pinVault struct {
	mu         sync.Mutex
	path       string
	manual     map[string][]byte
	active     map[string]uint64
	until      time.Time
	generation uint64
	logf       func(string, ...interface{})
}

func pinIdentity(s settings) string {
	h := sha256.Sum256([]byte(profileKey(s.Store, s.Thumbprint) + ":" + s.PKCS11Module))
	return hex.EncodeToString(h[:])
}
func (v *pinVault) read() (map[string]pinRecord, error) {
	rows := map[string]pinRecord{}
	b, e := os.ReadFile(v.path)
	if os.IsNotExist(e) {
		return rows, nil
	}
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &rows)
	return rows, e
}
func protectedWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".token-secret-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func (v *pinVault) write(rows map[string]pinRecord) error {
	b, e := json.Marshal(rows)
	if e != nil {
		return e
	}
	return protectedWrite(v.path, b)
}
func (v *pinVault) status(s settings) (bool, bool, error) {
	if v == nil {
		return false, false, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	rows, e := v.read()
	r := rows[pinIdentity(s)]
	return len(r.Sealed) > 0, r.Blocked, e
}
func (v *pinVault) save(s settings, pin string, forget, manual bool) error {
	if s.Thumbprint == "" {
		return errors.New("Selectați și salvați certificatul înainte de configurarea PIN-ului")
	}
	if !forget && (len(pin) == 0 || len(pin) > 256) {
		return errors.New("PIN-ul trebuie să aibă între 1 și 256 octeți")
	}
	for _, c := range pin {
		if c == 0 || c == '\n' || c == '\r' {
			return errors.New("PIN invalid")
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pinIdentity(s)
	if v.manual == nil {
		v.manual = map[string][]byte{}
	}
	v.generation++
	delete(v.active, key)
	for k, p := range v.manual {
		clear(p)
		delete(v.manual, k)
	}
	if manual {
		rows, e := v.read()
		if e != nil {
			return errors.New("Nu se poate citi depozitul PIN")
		}
		if r, ok := rows[key]; ok {
			r.Blocked = true
			rows[key] = r
			if e = v.write(rows); e != nil {
				return e
			}
		}
		v.manual[key] = []byte(pin)
		v.until = time.Now().Add(150 * time.Second)
		generation := v.generation
		time.AfterFunc(150*time.Second, func() {
			v.mu.Lock()
			defer v.mu.Unlock()
			if v.generation == generation {
				for k, p := range v.manual {
					clear(p)
					delete(v.manual, k)
				}
			}
		})
		return nil
	}
	rows, e := v.read()
	if e != nil {
		return errors.New("Nu se poate citi depozitul PIN")
	}
	if forget {
		delete(rows, key)
	} else {
		b, e := sealPIN(v.path, []byte(pin), key)
		if e != nil {
			return errors.New("Nu se poate cripta PIN-ul")
		}
		rows[key] = pinRecord{Sealed: b}
	}
	return v.write(rows)
}

// begin serializes automatic attempts and persists explicit PIN rejection counts.
func (v *pinVault) begin(s settings) ([]byte, func(pinOutcome), error) {
	noop := func(pinOutcome) {}
	if v == nil {
		return nil, noop, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pinIdentity(s)
	if p := v.manual[key]; len(p) > 0 {
		delete(v.manual, key)
		if time.Now().Before(v.until) {
			generation := v.generation
			return p, func(outcome pinOutcome) {
				defer clear(p)
				v.mu.Lock()
				defer v.mu.Unlock()
				if outcome == pinSuccess && generation == v.generation {
					if rows, err := v.read(); err == nil {
						if r, ok := rows[key]; ok {
							r.Failures = 0
							rows[key] = r
							_ = v.write(rows)
						}
					}
				}
				if outcome != pinRejected && generation == v.generation && time.Now().Before(v.until) {
					v.manual[key] = append([]byte(nil), p...)
				}
			}, nil
		}
		clear(p)
	}
	rows, e := v.read()
	if e != nil {
		return nil, noop, errors.New("Depozitul PIN nu poate fi citit; autentificarea automată este oprită")
	}
	r, ok := rows[key]
	_, busy := v.active[key]
	if !ok || r.Blocked || busy {
		return nil, noop, nil
	}
	p, e := openPIN(v.path, r.Sealed, key)
	if e != nil {
		return nil, noop, errors.New("PIN-ul nu poate fi decriptat; actualizați-l în Setări CNAS")
	}
	if v.active == nil {
		v.active = map[string]uint64{}
	}
	generation := v.generation
	v.active[key] = generation
	if v.logf != nil {
		v.logf("SIUI token: o singura incercare automata PIN pentru certificatul %s", s.Thumbprint)
	}
	original := string(r.Sealed)
	var once sync.Once
	return p, func(outcome pinOutcome) {
		once.Do(func() {
			clear(p)
			v.mu.Lock()
			defer v.mu.Unlock()
			if current, ok := v.active[key]; ok && current == generation {
				delete(v.active, key)
			}
			if outcome == pinUnrelated || generation != v.generation {
				return
			}
			rows, e := v.read()
			if e != nil {
				return
			}
			current := rows[key]
			if string(current.Sealed) != original {
				return
			}
			if outcome == pinSuccess {
				current.Failures = 0
			} else {
				current.Failures++
				current.Blocked = current.Failures >= pinFailureLimit
			}
			rows[key] = current
			if e = v.write(rows); e != nil {
				if v.logf != nil {
					v.logf("[ERROR] SIUI token: nu se poate salva suspendarea PIN-ului")
				}
				return
			}
			if v.logf != nil && outcome == pinRejected {
				v.logf("[ERROR] SIUI token: PIN respins (%d/%d); suspendat=%t", current.Failures, pinFailureLimit, current.Blocked)
			}
		})
	}, nil
}

func randomBytes(n int) ([]byte, error) { b := make([]byte, n); _, e := rand.Read(b); return b, e }

func (v *pinVault) available(s settings) bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pinIdentity(s)
	if len(v.manual[key]) > 0 && time.Now().Before(v.until) {
		return true
	}
	rows, e := v.read()
	return e == nil && len(rows[key].Sealed) > 0 && !rows[key].Blocked
}

func (v *pinVault) manualAvailable(s settings) bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.manual[pinIdentity(s)]) > 0 && time.Now().Before(v.until)
}

func (v *pinVault) failureCount(s settings) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	rows, err := v.read()
	return rows[pinIdentity(s)].Failures, err
}
