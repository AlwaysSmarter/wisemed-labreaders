package siui

import "errors"

// Enumerate the configured store without requiring CNAS credentials or changing the selected profile.
func (m *Module) reloadCertificates(reason string) (err error) {
	defer func() {
		if err != nil {
			m.rt.Logf("[ERROR] SIUI certificate: reincarcare esuata (%s): %v", reason, err)
		} else {
			m.rt.Logf("SIUI certificate: reincarcate (%s)", reason)
		}
	}()
	m.mu.Lock()
	if m.pending > 0 {
		m.mu.Unlock()
		return errors.New("Certificatele nu pot fi reîncărcate: tokenul este ocupat. Reîncercați din Setări → CNAS.")
	}
	cfg := m.cfg
	m.pending++
	m.checking = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.pending--; m.checking = false; m.mu.Unlock() }()
	if refresher, ok := m.factory(cfg).(interface{ refreshCertificates() error }); ok {
		return refresher.refreshCertificates()
	}
	return nil
}
