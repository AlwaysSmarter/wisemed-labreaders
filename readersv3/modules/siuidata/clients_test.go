package siuidata

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

const clientFixture = `<provider xmlns="http://www.cnas.ro/siui/2.0" code="PROV1" name="Client Confidențial" uic="RO7991962" issueDate="2026-01-01T09:00:00">
<contracts><contract contractNo="CL100" contractDate="2026-01-01" validFrom="2026-01-01" validTo="2026-12-31" orgUnitCode="CAS-BV" companyCode="CNAS" categoryCode="CLIN" contractType="CLN"/></contracts>
<employees><employee stencilNo="PAR1" firstName="Doctor" lastName="Confidențial" validFrom="2026-01-01" validTo="2026-12-31"><specialities><speciality code="CARD"/></specialities></employee></employees></provider>`
const targetedCatalogues = `<Catalogues xmlns="http://www.cnas.ro/siui/2.0">
<Specialities><Speciality code="CARD" name="Cardiologie" validFrom="2020-01-01"/><Speciality code="DERM" name="Dermatologie" validFrom="2020-01-01"/></Specialities>
<ClinicServiceGroups><ClinicServiceGroup code="G1" description="Comun" validFrom="2020-01-01"/><ClinicServiceGroup code="G2" description="Dermato" validFrom="2020-01-01"/></ClinicServiceGroups>
<ClinSrvGrpSpecialties><ClinSrvGrpSpecialty serviceGroup="G1" specialty="CARD" validFrom="2020-01-01"/><ClinSrvGrpSpecialty serviceGroup="G1" specialty="DERM" validFrom="2020-01-01"/><ClinSrvGrpSpecialty serviceGroup="G2" specialty="DERM" validFrom="2020-01-01"/></ClinSrvGrpSpecialties>
<MedSrvPacks><MedSrvPack code="BAZA" name="Bază" validFrom="2020-01-01"/><MedSrvPack code="EXT" name="Extins" validFrom="2020-01-01"/></MedSrvPacks>
<ClinicServices><ClinicService code="A" name="Comun" serviceGroup="G1" isExam="true" opRoom="false" isConnectedService="false" validFrom="2020-01-01"/><ClinicService code="B" name="Dermato" serviceGroup="G2" isExam="true" opRoom="false" isConnectedService="false" validFrom="2020-01-01"/></ClinicServices>
<ClinSrvMedPacks><ClinSrvMedPack service="A" medSrvPack="BAZA" validFrom="2020-01-01"/><ClinSrvMedPack service="A" medSrvPack="EXT" validFrom="2020-01-01"/></ClinSrvMedPacks></Catalogues>`

func privateCall(t testing.TB, s *httptest.Server, method, path, ct string, body io.Reader) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest(method, s.URL+"/siui-data-clients"+path, body)
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("X-SIUI-Internal", "1")
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	response, e := s.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	data, e := io.ReadAll(response.Body)
	if e != nil {
		t.Fatal(e)
	}
	return response.StatusCode, data
}
func importClient(t testing.TB, s *httptest.Server, category, filename string, content []byte) (int, []byte) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	w.WriteField("category", category)
	p, e := w.CreateFormFile("file", filename)
	if e != nil {
		t.Fatal(e)
	}
	p.Write(content)
	w.Close()
	return privateCall(t, s, "POST", "/import", w.FormDataContentType(), &b)
}
func tenantToken(t testing.TB, m *Module, cui, key string, mutate func(*claims, *jwt.Token)) string {
	t.Helper()
	now := time.Now()
	c := &claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "client", Issuer: "wisemed-siui-data", Audience: jwt.ClaimStrings{"siui-data-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, CUI: cui}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	token.Header["kid"] = cui
	if mutate != nil {
		mutate(c, token)
	}
	signed, e := token.SignedString([]byte(key))
	if e != nil {
		t.Fatal(e)
	}
	return signed
}
func bearerCall(t testing.TB, s *httptest.Server, path, token string) (int, []byte) {
	t.Helper()
	r, _ := http.NewRequest("GET", s.URL+path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	response, e := s.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}

func TestClientTemporalProfilesAndTargetedServices(t *testing.T) {
	m, s := newTestModule(t, testPostgresSettings(t))
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", targetedCatalogues))), "ready")
	status, data := importClient(t, s, "CLIN", "first.xml", []byte(clientFixture))
	if status != 200 {
		t.Fatalf("import %d %s", status, data)
	}
	second := strings.NewReplacer("2026-01-01T09:00:00", "2026-06-01T10:00:00", `code="CARD"`, `code="DERM"`).Replace(clientFixture)
	if status, data = importClient(t, s, "CLIN", "second.xml", []byte(second)); status != 200 {
		t.Fatalf("second %d %s", status, data)
	}
	for _, tc := range []struct {
		route, query string
		total, size  int
	}{
		{"targeted_services", "atDate=2025-12-31", 0, 0},
		{"targeted_services", "atDate=2026-05-01", 1, 1},
		{"targeted_services", "atDate=2026-07-01", 2, 2},
		{"targeted_services", "atDate=2027-01-01", 0, 0},
		{"targeted_services", "atDate=2026-05-01&code=B", 0, 0},
		{"targeted_services", "atDate=2026-05-01&specialties[]=DERM", 0, 0},
		{"targeted_services", "atDate=2026-05-01&specialtyName=dermatologie", 0, 0},
		{"targeted_services", "atDate=2026-05-01&specialtyNameAll[]=dermato&specialtyNameAll[]=logie", 0, 0},
		{"targeted_services", "atDate=2026-05-01&specialtyNameAll[]=CARDIO&specialtyNameAll[]=logie&nameAll[]=com&nameAll[]=mun", 1, 1},
		{"targeted-simple-services", "atDate=2026-05-01&specialtyNameAll[]=cardio&specialtyNameAll[]=logie&qAll[]=comun&qAll[]=cardio", 2, 2},
		{"targeted-simple-services", "atDate=2026-05-01&specialtyNameAll[]=dermato&specialtyNameAll[]=logie", 0, 0},
		{"targeted_services", "atDate=2026-05-01&qAll[]=inexistent&qAll[]=serviciu", 0, 0},
		{"targeted-simple-services", "atDate=2026-05-01&nameAll[]=inexistent&nameAll[]=serviciu", 0, 0},
		{"targeted-simple-services", "atDate=2026-05-01", 2, 2},
		{"targeted-simple-services", "atDate=2026-05-01&specialties[]=DERM", 0, 0},
		{"targeted-simple-services", "atDate=2026-07-01&limit=1&start=1", 3, 1},
		{"targeted-simple-services", "atDate=2026-07-01&start=99", 3, 0},
	} {
		status, data := apiCall(t, s, "GET", prefix+tc.route+"/7991962?"+tc.query, "", nil)
		var p flatPage
		json.Unmarshal(data, &p)
		if status != 200 || !p.Success || p.Total != tc.total || len(p.Rows) != tc.size {
			t.Fatalf("%s?%s %d %s", tc.route, tc.query, status, data)
		}
		if strings.Contains(string(data), "Confidențial") || bytes.Contains(data, []byte("CL100")) || bytes.Contains(data, []byte("PAR1")) {
			t.Fatalf("private data leaked: %s", data)
		}
		if tc.route == "targeted-simple-services" {
			readFlatPage(t, data, status, tc.total, tc.size)
		} else if tc.query == "atDate=2026-05-01" {
			sp := p.Rows[0]["specialties"].([]any)
			if len(sp) != 1 || sp[0].(map[string]any)["code"] != "CARD" {
				t.Fatalf("unscoped specialties: %s", data)
			}
		}
	}
	for _, query := range []string{"type=PARA", "type[]=CLIN&type[]=PARA", "after=A", "atDate[]=2026-01-01&atDate[]=2026-02-01", "atDate=invalid", "cui=12345678"} {
		status, data := apiCall(t, s, "GET", prefix+"targeted_services/7991962?"+query, "", nil)
		if status != 400 {
			t.Fatalf("override %s %d %s", query, status, data)
		}
	}
	status, data = privateCall(t, s, "GET", "/data?cui=7991962&atDate=2026-05-01", "", nil)
	var overview map[string]any
	decodeData(t, data, &overview)
	if status != 200 || overview["clinicalSpecialities"].([]any)[0] != "CARD" || len(overview["versions"].([]any)) != 2 {
		t.Fatalf("overview %d %s", status, data)
	}
	status, data = importClient(t, s, "CLIN", "same.xml", []byte(second))
	if status != 200 || !bytes.Contains(data, []byte(`"created":false`)) {
		t.Fatalf("idempotency %d %s", status, data)
	}
	// A malformed newer version must roll back; the earlier profile stays usable.
	bad := strings.NewReplacer("2026-06-01T10:00:00", "2026-08-01T10:00:00", `validTo="2026-12-31"`, `validTo="2020-01-01"`).Replace(second)
	if status, _ = importClient(t, s, "CLIN", "bad.xml", []byte(bad)); status != 409 {
		t.Fatal("invalid periods accepted")
	}
	status, data = privateCall(t, s, "GET", "/data?cui=7991962&atDate=2026-09-01", "", nil)
	decodeData(t, data, &overview)
	if len(overview["versions"].([]any)) != 2 {
		t.Fatal("partial profile was published")
	}
	// Other categories never widen the CLIN profile.
	para := strings.NewReplacer("PROV1", "PARAPROV", `categoryCode="CLIN"`, `categoryCode="PARA"`, `code="CARD"`, `code="NEURO"`).Replace(clientFixture)
	if status, data = importClient(t, s, "PARA", "para.xml", []byte(para)); status != 200 {
		t.Fatalf("para %d %s", status, data)
	}
	status, data = privateCall(t, s, "GET", "/data?cui=7991962&atDate=2026-05-01", "", nil)
	decodeData(t, data, &overview)
	if len(overview["profiles"].([]any)) != 2 || len(overview["clinicalSpecialities"].([]any)) != 1 {
		t.Fatalf("category isolation %s", data)
	}
	for _, tc := range []struct {
		query string
		total int
	}{
		{"activeCategory=PARA&atDate=2026-05-01", 1}, {"activeCategory=CLIN&atDate=2026-05-01", 1}, {"activeCategory=PARA&atDate=2027-01-01", 0}, {"activeCategory=SICK&atDate=2026-05-01", 0},
	} {
		status, data := privateCall(t, s, "GET", "/data?"+tc.query, "", nil)
		var p flatPage
		json.Unmarshal(data, &p)
		if status != 200 || p.Total != tc.total {
			t.Fatalf("client category filter %s %d %s", tc.query, status, data)
		}
	}
	// A new complete snapshot with no staff revokes eligibility from its issueDate.
	empty := strings.Replace(second, "2026-06-01T10:00:00", "2026-10-01T10:00:00", 1)
	a, b := strings.Index(empty, "<employees>"), strings.Index(empty, "</employees>")+len("</employees>")
	empty = empty[:a] + "<employees/>" + empty[b:]
	if status, data = importClient(t, s, "CLIN", "empty.xml", []byte(empty)); status != 200 {
		t.Fatalf("empty %d %s", status, data)
	}
	status, data = apiCall(t, s, "GET", prefix+"targeted_services/7991962?atDate=2026-10-02", "", nil)
	readFlatPage(t, data, status, 0, 0)
	_ = m
}

func TestClientJWTIsolationAndPrivateBoundary(t *testing.T) {
	c := testPostgresSettings(t)
	key := strings.Repeat("A", 48)
	other := strings.Repeat("B", 48)
	c.ClientJWTSecrets = map[string]string{"7991962": key, "12345678": other}
	m, s := newTestModule(t, c)
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", targetedCatalogues))), "ready")
	if status, data := importClient(t, s, "CLIN", "client.xml", []byte(clientFixture)); status != 200 {
		t.Fatalf("seed %d %s", status, data)
	}
	token := tenantToken(t, m, "7991962", key, nil)
	if status, data := bearerCall(t, s, prefix+"targeted-simple-services/7991962?atDate=2026-05-01", token); status != 200 {
		t.Fatalf("own CUI %d %s", status, data)
	} else {
		readFlatPage(t, data, status, 2, 2)
	}
	for _, path := range []string{prefix + "targeted_services/12345678", prefix + "targeted-simple-services/12345678", prefix + "services", prefix + "records/Physician", prefix + "settings", prefix + "versions", prefix + "auth/token", "/siui-data-clients", "/siui-data-clients/data?cui=7991962", "/siui-data-clients/keys"} {
		status, data := bearerCall(t, s, path, token)
		if status != 403 {
			t.Fatalf("tenant escaped to %s %d %s", path, status, data)
		}
	}
	for _, mutate := range []func(*claims, *jwt.Token){func(c *claims, t *jwt.Token) { c.CUI = "12345678" }, func(c *claims, t *jwt.Token) { c.Admin = true }, func(c *claims, t *jwt.Token) { t.Header["kid"] = "global" }, func(c *claims, t *jwt.Token) { t.Header["kid"] = "12345678" }, func(c *claims, t *jwt.Token) { delete(t.Header, "kid") }, func(c *claims, t *jwt.Token) { c.ExpiresAt = jwt.NewNumericDate(c.IssuedAt.Add(time.Hour)) }} {
		forged := tenantToken(t, m, "7991962", key, mutate)
		if status, _ := bearerCall(t, s, prefix+"targeted_services/7991962", forged); status != 401 {
			t.Fatal("forged tenant identity accepted")
		}
	}
	// Having a valid WSM admin principal cannot turn a tenant JWT into a global token.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
		r.TLS = &tls.ConnectionState{}
		m.authenticate(http.HandlerFunc(m.serve)).ServeHTTP(w, r)
	})
	remote, e := apibridge.Invoke(context.Background(), handler, apibridge.Principal{Subject: "admin", TenantID: "org", Admin: true}, apibridge.Request{Method: "GET", Path: prefix + "targeted_services/12345678"})
	if e != nil || remote.Status != 403 {
		t.Fatalf("WSM bypass %v %+v", e, remote)
	}
	remote, e = apibridge.Invoke(context.Background(), m.rt.(*testRuntime).mux, apibridge.Principal{Subject: "admin", TenantID: "org", Admin: true}, apibridge.Request{Method: "GET", Path: "/siui-data-clients/data?cui=7991962"})
	if e == nil {
		t.Fatalf("private route reachable over WSM %+v", remote)
	}
	issuedStatus, data := privateCall(t, s, "POST", "/token", "application/json", strings.NewReader(`{"cui":"7991962"}`))
	if issuedStatus != 200 {
		t.Fatalf("issue %d %s", issuedStatus, data)
	}
	var issued map[string]any
	decodeData(t, data, &issued)
	status, data := bearerCall(t, s, prefix+"targeted_services/7991962", issued["access_token"].(string))
	if status == 401 || status == 403 {
		t.Fatalf("own CUI token rejected: %d %s", status, data)
	}
	status, data = apiCall(t, s, "GET", prefix+"settings", "", nil)
	if status != 200 || bytes.Contains(data, []byte(key)) || bytes.Contains(data, []byte(other)) {
		t.Fatalf("settings leaked secrets: %s", data)
	}
	status, data = privateCall(t, s, "GET", "/keys", "", nil)
	if status != 200 || bytes.Contains(data, []byte(key)) {
		t.Fatalf("keys leaked secret %s", data)
	}
	rotated := strings.Repeat("R", 48)
	status, data = privateCall(t, s, "PUT", "/keys", "application/json", strings.NewReader(`{"cui":"7991962","secret":"`+rotated+`"}`))
	if status != 200 {
		t.Fatalf("rotation %d %s", status, data)
	}
	if status, _ = bearerCall(t, s, prefix+"targeted_services/7991962", token); status != 401 {
		t.Fatal("old key remained valid after rotation")
	}
	raw, e := os.ReadFile(m.rt.ConfigPath())
	if e != nil || !bytes.Contains(raw, []byte(rotated)) {
		t.Fatal("key not persisted")
	}
	info, _ := os.Stat(m.rt.ConfigPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
	// A global bearer cannot enter the confidential browser-only pages either.
	now := time.Now()
	global, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "browser", Issuer: "wisemed-siui-data", Audience: jwt.ClaimStrings{"siui-data-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute))}, Admin: true}).SignedString(m.key)
	for _, cui := range []string{"7991962", "12345678"} {
		if status, data := bearerCall(t, s, prefix+"targeted_services/"+cui+"?atDate=2026-05-01", global); status != 200 {
			t.Fatalf("global CUI %s %d %s", cui, status, data)
		}
	}
	if status, _ = bearerCall(t, s, "/siui-data-clients/data?cui=7991962", global); status != 403 {
		t.Fatal("global JWT exposed confidential records")
	}
	m.guard = fakeGuard{true, false}
	if status, _ = privateCall(t, s, "GET", "/data", "", nil); status != 403 {
		t.Fatal("nonadministrator entered client UI")
	}
}

func TestPersonalizationParserPrivacy(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Bucharest")
	doc, e := parseClientDocument(context.Background(), strings.NewReader(clientFixture), "CLIN", "test.xml", loc)
	if e != nil || doc.CUI != "7991962" || len(doc.Employees) != 1 {
		t.Fatalf("parse %+v %v", doc, e)
	}
	// Unknown patient lists are consumed without retention, including nested MF enrolments.
	mf := strings.Replace(clientFixture, "</employee>", `<enlisteds><enlisted pid="CONFIDENTIAL-PATIENT"/></enlisteds></employee>`, 1)
	doc, e = parseClientDocument(context.Background(), strings.NewReader(mf), "MF", "mf.xml", loc)
	b, _ := json.Marshal(doc)
	if e != nil || bytes.Contains(b, []byte("CONFIDENTIAL-PATIENT")) {
		t.Fatalf("patient details retained: %v %s", e, b)
	}
	for _, xml := range []string{`<!DOCTYPE provider>` + clientFixture, clientFixture + clientFixture, strings.Replace(clientFixture, `uic="RO7991962"`, `uic="../invalid"`, 1), strings.Replace(clientFixture, `2026-01-01T09:00:00`, `invalid`, 1), clientFixture[:len(clientFixture)-10]} {
		if _, e = parseClientDocument(context.Background(), strings.NewReader(xml), "CLIN", "bad.xml", loc); e == nil {
			t.Fatal("malformed personalization accepted")
		}
	}
	// Internal mutation requires both session and CSRF header even on TLS.
	m := &Module{guard: fakeGuard{true, true}}
	r := httptest.NewRequest("POST", "/siui-data-clients/import", nil)
	r.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	m.clientsHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF request accepted")
	}
}

func TestPersonalizationZIPRollback(t *testing.T) {
	m, s := newTestModule(t, testPostgresSettings(t))
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("good.xml")
	w.Write([]byte(clientFixture))
	w, _ = z.Create("bad.xml")
	w.Write([]byte(`<provider uic="7991962">`))
	z.Close()
	if status, _ := importClient(t, s, "CLIN", "both.zip", b.Bytes()); status == 200 {
		t.Fatal("partial ZIP was accepted")
	}
	db, e := m.database(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var count int
	db.QueryRow(context.Background(), `SELECT count(*) FROM siui_client_snapshots`).Scan(&count)
	if count != 0 {
		t.Fatal("ZIP partially committed")
	}
}

func TestRealClientPersonalizations(t *testing.T) {
	dir := os.Getenv("SIUI_DATA_REAL_PERSONALIZATIONS")
	if dir == "" {
		t.Skip("set SIUI_DATA_REAL_PERSONALIZATIONS to the supplied personalization directory")
	}
	m, s := newTestModule(t, testPostgresSettings(t))
	cuis := map[string]bool{}
	day := ""
	for _, category := range []string{"CLIN", "PARA", "SPT", "AMB", "SICK"} {
		raw, e := os.ReadFile(filepath.Join(dir, category+".xml"))
		if e != nil {
			t.Fatal(e)
		}
		doc, e := parseClientDocument(context.Background(), bytes.NewReader(raw), category, category+".xml", m.loc)
		if e != nil {
			t.Fatalf("%s parse: %v", category, e)
		}
		cuis[doc.CUI] = true
		if d := doc.Effective.In(m.loc).Format("2006-01-02"); d > day {
			day = d
		}
		status, data := importClient(t, s, category, category+".xml", raw)
		if status != 200 {
			t.Fatalf("%s import %d %s", category, status, data)
		}
	}
	for cui := range cuis {
		status, data := privateCall(t, s, "GET", "/data?cui="+cui+"&atDate="+day, "", nil)
		var overview struct {
			Profiles []struct {
				Category             string
				Contracts, Employees []json.RawMessage
			}
			ClinicalSpecialities []string
		}
		decodeData(t, data, &overview)
		if status != 200 || len(overview.Profiles) != 5 || len(overview.ClinicalSpecialities) == 0 {
			t.Fatalf("overview real personalizations: status=%d categories=%d specialties=%d", status, len(overview.Profiles), len(overview.ClinicalSpecialities))
		}
		for _, p := range overview.Profiles {
			t.Logf("real %s: %d contracts/conventions; %d employees", p.Category, len(p.Contracts), len(p.Employees))
		}
		t.Logf("real client profile: %d eligible clinical specialties", len(overview.ClinicalSpecialities))
		// Use a synthetic public catalogue named after a real eligible speciality;
		// never expose a real contract or employee in the public result.
		public := strings.ReplaceAll(targetedCatalogues, `code="CARD"`, `code="`+overview.ClinicalSpecialities[0]+`"`)
		public = strings.ReplaceAll(public, `specialty="CARD"`, `specialty="`+overview.ClinicalSpecialities[0]+`"`)
		awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", public))), "ready")
		status, data = apiCall(t, s, "GET", prefix+"targeted-simple-services/"+cui+"?atDate="+day, "", nil)
		readFlatPage(t, data, status, 2, 2)
	}
}

func TestClientEligibilityUsesActualContractsAndPeriods(t *testing.T) {
	_, s := newTestModule(t, testPostgresSettings(t))
	awaitImport(t, s, upload(t, s, "CLIN", bytes.NewReader(zipXML(t, "clin.xml", targetedCatalogues))), "ready")
	for i, tc := range []struct {
		category, xml string
		count         int
	}{
		{"SPT", clientFixture, 1}, // Hospital personalizations can contain an actual CLIN contract.
		{"CLIN", strings.Replace(clientFixture, `categoryCode="CLIN"`, `categoryCode="PARA"`, 1), 0},
		{"CLIN", strings.Replace(clientFixture, `stencilNo="PAR1"`, `stencilNo=""`, 1), 0},
		{"CLIN", strings.Replace(clientFixture, `<speciality code="CARD"/>`, `<speciality code="CARD" validFrom="2026-09-01"/>`, 1), 0},
		{"CLIN", strings.Replace(clientFixture, `firstName="Doctor" lastName="Confidențial" validFrom="2026-01-01" validTo="2026-12-31"`, `firstName="Doctor" lastName="Confidențial" validFrom="2026-01-01" validTo="2026-02-01"`, 1), 0},
	} {
		cui := fmt.Sprintf("88888%d", i)
		xml := strings.Replace(tc.xml, "RO7991962", cui, 1)
		if status, data := importClient(t, s, tc.category, "profile.xml", []byte(xml)); status != 200 {
			t.Fatalf("case %d %d %s", i, status, data)
		}
		status, data := apiCall(t, s, "GET", prefix+"targeted_services/"+cui+"?atDate=2026-05-01", "", nil)
		var p flatPage
		json.Unmarshal(data, &p)
		if status != 200 || p.Total != tc.count {
			t.Fatalf("eligibility %d %d %s", i, status, data)
		}
	}
}

func TestTargetedUnknownCUIAlwaysReturnsEmpty(t *testing.T) {
	c := testPostgresSettings(t)
	key := strings.Repeat("Z", 48)
	c.ClientJWTSecrets = map[string]string{"7991962": key}
	m, s := newTestModule(t, c)
	token := tenantToken(t, m, "7991962", key, nil)
	// Neither a personalization nor a CLIN catalogue has been imported.
	for _, route := range []string{"targeted_services", "targeted-simple-services"} {
		status, data := bearerCall(t, s, prefix+route+"/7991962?limit=1&start=99", token)
		readFlatPage(t, data, status, 0, 0)
	}
}
