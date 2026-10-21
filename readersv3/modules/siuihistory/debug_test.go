package siuihistory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallPeriod(t *testing.T) {
	r := httptest.NewRequest("GET", "/calls?from=2026-10-21T10:00:00%2B03:00&to=2026-10-21T08:00:00.123Z", nil)
	from, to, err := callPeriod(r)
	if err != nil || from != "2026-10-21T07:00:00.000000000Z" || to != "2026-10-21T08:00:00.123000000Z" {
		t.Fatal(from, to, err)
	}
	for _, q := range []string{"from=bad", "to=bad", "from=2026-10-22T00:00:00Z&to=2026-10-21T00:00:00Z"} {
		if _, _, err := callPeriod(httptest.NewRequest("GET", "/calls?"+q, nil)); err == nil {
			t.Fatal("accepted invalid period", q)
		}
	}
}
func TestDebugRecordsBridgeFailuresWithoutDatabaseOrSecrets(t *testing.T) {
	m := setup(t)
	token := "secret-test-key"
	sum := sha256.Sum256([]byte(token))
	m.cfg.Keys = []bridgeKey{{Tenant: "tenant", Bridge: "bridge-01", Hash: hex.EncodeToString(sum[:])}}
	for _, path := range []string{"/api/history/identity", "/api/history/calls"} {
		r := httptest.NewRequest("GET", path+"?token=do-not-store", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		m.debugAuthenticate(httptest.NewRecorder(), r)
	}
	w := request(m, "GET", "/api/history/debug/requests?bridge=bridge-01", "", nil)
	var data struct {
		Requests []debugRequest `json:"requests"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Requests) != 2 || data.Requests[0].Status != 503 || !strings.Contains(data.Requests[0].Error, "MySQL") || data.Requests[1].Status != 200 {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "do-not-store") {
		t.Fatal("secret leaked")
	}
	w = request(m, "GET", "/api/history/debug/requests", token, nil)
	if w.Code != 403 {
		t.Fatal("bridge can read administrator debug", w.Code)
	}
	m.requests = append(m.requests, debugRequest{Tenant: "another-tenant", Bridge: "bridge-01", Path: "hidden"})
	w = request(m, "GET", "/api/history/debug/requests", "", nil)
	if strings.Contains(w.Body.String(), "hidden") {
		t.Fatal("tenant leak")
	}
}

func TestDashboardRejectsBridgeCredentialAndInvalidPeriods(t *testing.T) {
	m := setup(t)
	token := "bridge-dashboard-key"
	sum := sha256.Sum256([]byte(token))
	m.cfg.Keys = []bridgeKey{{Tenant: "tenant", Bridge: "bridge-01", Hash: hex.EncodeToString(sum[:])}}
	w := request(m, "GET", "/api/history/dashboard?from=2026-10-21T00:00:00Z&to=2026-10-22T00:00:00Z", token, nil)
	if w.Code != 403 {
		t.Fatal("bridge may inspect central metrics", w.Code, w.Body.String())
	}
	for _, period := range []string{"", "from=bad&to=bad", "from=2026-10-22T00:00:00Z&to=2026-10-21T00:00:00Z", "from=2024-01-01T00:00:00Z&to=2026-10-21T00:00:00Z"} {
		w = request(m, "GET", "/api/history/dashboard?"+period, "", nil)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
