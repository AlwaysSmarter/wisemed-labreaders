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

type pinRecord struct {
	Sealed  []byte `json:"sealed"`
	Blocked bool   `json:"blocked"`
}
type pinVault struct {
	mu         sync.Mutex
	path       string
	manual     map[string][]byte
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

// Persist suspension BEFORE handing a PIN to a driver. A crash, cancellation or
// failed operation leaves it suspended across restarts. Only verified signing
// success may re-enable the same encrypted record; manual PINs never re-enable it.
func (v *pinVault) begin(s settings) ([]byte, func(bool), error) {
	noop := func(bool) {}
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
			return p, func(success bool) {
				defer clear(p)
				v.mu.Lock()
				defer v.mu.Unlock()
				if success && generation == v.generation && time.Now().Before(v.until) {
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
	if !ok || r.Blocked {
		return nil, noop, nil
	}
	p, e := openPIN(v.path, r.Sealed, key)
	if e != nil {
		return nil, noop, errors.New("PIN-ul nu poate fi decriptat; actualizați-l în Setări CNAS")
	}
	r.Blocked = true
	rows[key] = r
	if e = v.write(rows); e != nil {
		clear(p)
		return nil, noop, errors.New("Nu se poate proteja contorul încercărilor PIN")
	}
	if v.logf != nil {
		v.logf("SIUI token: o singura incercare automata PIN pentru certificatul %s", s.Thumbprint)
	}
	original := string(r.Sealed)
	var once sync.Once
	return p, func(success bool) {
		once.Do(func() {
			clear(p)
			if !success {
				if v.logf != nil {
					v.logf("[ERROR] SIUI token: autentificare automata suspendata; folositi introducerea manuala sau actualizati PIN-ul")
				}
				return
			}
			v.mu.Lock()
			defer v.mu.Unlock()
			rows, e := v.read()
			if e != nil {
				return
			}
			current := rows[key]
			if string(current.Sealed) == original {
				current.Blocked = false
				rows[key] = current
				_ = v.write(rows)
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
