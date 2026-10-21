package siuidata

import (
	"bytes"
	"context"
	"crypto/tls"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fixture = `<Catalogues xmlns="http://www.cnas.ro/siui/2.0">
<Specialities><Speciality code="CARD" name="Cardiologie" validFrom="2020-01-01"/><Speciality code="OLD" name="Veche" validFrom="2020-01-01" validTo="2025-01-01"/></Specialities>
<ClinicServiceGroups><ClinicServiceGroup code="G1" description="Text informativ fără asocieri" validFrom="2020-01-01"/></ClinicServiceGroups>
<ClinSrvGrpSpecialties><ClinSrvGrpSpecialty serviceGroup="G1" specialty="CARD" validFrom="2020-01-01" validTo="2026-10-21"/><ClinSrvGrpSpecialty serviceGroup="G1" specialty="OLD" validFrom="2020-01-01" validTo="2025-01-01"/></ClinSrvGrpSpecialties>
<MedSrvPacks><MedSrvPack code="BAZA" name="Pachet de bază" validFrom="2020-01-01"/></MedSrvPacks>
<ClinicServices><ClinicService code="001" name="Consultație" serviceGroup="G1" isExam="true" opRoom="0" isConnectedService="1" validFrom="2026-10-21" validTo="2026-10-21" extra="preserved"/>
<ClinicService code="PAST" name="Expirat" isExam="false" opRoom="false" isConnectedService="false" validFrom="2020-01-01" validTo="2026-10-20"/>
<ClinicService code="FUTURE" name="Viitor" isExam="false" opRoom="false" isConnectedService="false" validFrom="2026-10-22"/>
</ClinicServices><ClinSrvMedPacks><ClinSrvMedPack service="001" medSrvPack="BAZA" points="12.3500" pointsCtrl="0" pointsSrg="7.5" valueLei="42.00" validFrom="2020-01-01"/><ClinSrvMedPack service="PAST" medSrvPack="BAZA" validFrom="2020-01-01"/></ClinSrvMedPacks></Catalogues>`

func testDate() time.Time { return time.Date(2026, 10, 20, 22, 0, 0, 0, time.UTC) }

type fakeGuard struct{ allow, admin bool }

func (g fakeGuard) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.allow {
			failure(w, 401, "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (g fakeGuard) SessionIdentity(*http.Request) (string, bool, bool) {
	return "operator", g.admin, g.allow
}
func request(m *Module, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, prefix+path, nil)
	r.TLS = &tls.ConnectionState{}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	m.authenticate(http.HandlerFunc(m.serve)).ServeHTTP(w, r)
	return w
}
func TestAuthentication(t *testing.T) {
	m := &Module{guard: fakeGuard{true, true}, cfg: defaultSettings(), key: bytes.Repeat([]byte{7}, 32), started: time.Now()}
	issued := request(m, "POST", "auth/token", "")
	if issued.Code != 200 {
		t.Fatal(issued.Body.String())
	}
	var tok map[string]any
	decodeData(t, issued.Body.Bytes(), &tok)
	token := tok["access_token"].(string)
	m.guard = fakeGuard{}
	if w := request(m, "POST", "auth/token", token); w.Code != 403 {
		t.Fatal("JWT renewed itself")
	}
	for _, value := range []string{"", token + "tampered"} {
		if w := request(m, "GET", "status", value); w.Code != 401 {
			t.Fatal("anonymous/forged token accepted")
		}
	}
	now := time.Now()
	c := claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "reader", Issuer: "wisemed-siui-data", Audience: jwt.ClaimStrings{"siui-data-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}}
	readToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.key)
	if w := request(m, "POST", "upload", readToken); w.Code != 403 {
		t.Fatal("reader allowed to import")
	}
	for _, mutate := range []func(*claims){func(c *claims) { c.Issuer = "other" }, func(c *claims) { c.Audience = jwt.ClaimStrings{"other"} }, func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute)) }, func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour)) }} {
		altered := c
		mutate(&altered)
		value, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, altered).SignedString(m.key)
		if w := request(m, "GET", "status", value); w.Code != 401 {
			t.Fatal("invalid claims accepted")
		}
	}
	r := httptest.NewRequest("GET", prefix+"status", nil)
	r.Header.Set("Authorization", "Bearer "+readToken)
	w := httptest.NewRecorder()
	m.authenticate(http.HandlerFunc(m.serve)).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("HTTP bearer accepted")
	}
	m.guard = fakeGuard{true, true}
	r = httptest.NewRequest("POST", prefix+"upload", nil)
	r.Header.Set("Origin", "https://attacker.invalid")
	w = httptest.NewRecorder()
	m.authenticate(http.HandlerFunc(m.serve)).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
}
func TestStreamingParser(t *testing.T) {
	var ordinal int64
	src := newCatalogueRows(context.Background(), strings.NewReader(fixture), models["CLIN"], "2026-10-21", &ordinal)
	activeCount := 0
	for src.Next() {
		v, e := src.Values()
		if e != nil {
			t.Fatal(e)
		}
		if v[1] == "ClinicService" && v[9] == true {
			activeCount++
		}
	}
	if src.Err() != nil || activeCount != 1 {
		t.Fatalf("parser: %v count=%d", src.Err(), activeCount)
	}
	for _, bad := range []string{fixture[:len(fixture)-8], `<!DOCTYPE Catalogues>` + fixture, strings.Replace(fixture, `isExam="true"`, `isExam="yes"`, 1), strings.Replace(fixture, `points="12.3500"`, `points="NaN"`, 1), strings.Replace(fixture, `validFrom="2026-10-21"`, `validFrom="garbage"`, 1)} {
		src = newCatalogueRows(context.Background(), strings.NewReader(bad), models["CLIN"], "2026-10-21", &ordinal)
		for src.Next() {
			if _, e := src.Values(); e != nil {
				break
			}
		}
		if src.Err() == nil {
			t.Fatal("invalid XML accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src = newCatalogueRows(ctx, strings.NewReader(fixture), models["CLIN"], "2026-10-21", &ordinal)
	if src.Next() || src.Err() == nil {
		t.Fatal("cancellation ignored")
	}
	if normalizeText("Știință şi Consultaţie") != "stiinta si consultatie" {
		t.Fatal("normalization")
	}
}
func BenchmarkStreamingDecode(b *testing.B) {
	for range b.N {
		var n int64
		s := newCatalogueRows(context.Background(), strings.NewReader(fixture), models["CLIN"], "2026-10-21", &n)
		for s.Next() {
			if _, e := s.Values(); e != nil {
				b.Fatal(e)
			}
		}
		if s.Err() != nil {
			b.Fatal(s.Err())
		}
	}
}
