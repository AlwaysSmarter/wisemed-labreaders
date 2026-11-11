package wisemedai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpeechModelSeedAndCompatibility(t *testing.T) {
	var seeds []speechModelManifest
	if e := json.Unmarshal(speechModelSeed, &seeds); e != nil {
		t.Fatal(e)
	}
	if len(seeds) != 6 {
		t.Fatal("seed variants", len(seeds))
	}
	for _, v := range seeds {
		if e := validateSpeechModel(v); e != nil {
			t.Fatal(v.ID, e)
		}
	}
	original := seeds[0]
	for _, edit := range []func(*speechModelManifest){
		func(v *speechModelManifest) { v.Runtime = "arbitrary-code" },
		func(v *speechModelManifest) { v.Files[0].Path = "../escape" },
		func(v *speechModelManifest) { v.Files[0].URL = "http://example.com/model" },
		func(v *speechModelManifest) { v.Files[0].Role = "tokens" },
		func(v *speechModelManifest) { v.Family = "ctc" },
	} {
		raw, _ := json.Marshal(original)
		var v speechModelManifest
		json.Unmarshal(raw, &v)
		edit(&v)
		if validateSpeechModel(v) == nil {
			t.Fatal("incompatible manifest accepted", v)
		}
	}
}
func TestSpeechModelSourceRejectsPrivateTargets(t *testing.T) {
	client := modelSourceClient()
	defer client.CloseIdleConnections()
	for _, url := range []string{"https://127.0.0.1/file", "https://[::1]/file", "https://169.254.169.254/file"} {
		if res, e := client.Get(url); e == nil {
			res.Body.Close()
			t.Fatal("private upstream allowed", url)
		}
	}
}

type modelRoundTrip func(*http.Request) (*http.Response, error)

func (f modelRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostgresSpeechModelCatalogAndResumableUpload(t *testing.T) {
	m, _ := integrationModule(t)
	ctx := context.Background()
	var tenant, user string
	if e := m.db.QueryRow(ctx, `SELECT tenant_id,user_id FROM wai_memberships ORDER BY role LIMIT 1`).Scan(&tenant, &user); e != nil {
		t.Fatal(e)
	}
	admin := principal{Tenant: tenant, User: user, Role: "admin", InstanceAdmin: true}
	ordinary := principal{Tenant: tenant, User: user, Role: "admin"}
	call := func(p principal, method, path string, body any) *httptest.ResponseRecorder {
		var input io.Reader
		if b, ok := body.([]byte); ok {
			input = bytes.NewReader(b)
		} else if body != nil {
			b, _ := json.Marshal(body)
			input = bytes.NewReader(b)
		}
		r := httptest.NewRequest(method, "http://localhost/v1/"+path, input)
		w := httptest.NewRecorder()
		m.speechModels(w, r, p, strings.Split(strings.Split(path, "?")[0], "/"))
		return w
	}
	var v speechModelManifest
	v.Schema = "wisemed.speech-model.v1"
	v.ID = "synthetic-whisper"
	v.Version = "v1"
	v.Label = "Synthetic catalog test"
	v.Runtime = "sherpa-onnx-2.0.1"
	v.Family = "whisper"
	v.Platforms = []string{"android", "ios"}
	v.Language = "ro"
	v.License = "Synthetic test fixtures"
	data := [][]byte{[]byte("encoder-fixture"), []byte("decoder-fixture"), []byte("tokens-fixture")}
	for j, role := range []string{"encoder", "decoder", "tokens"} {
		v.Files = append(v.Files, speechModelFile{Path: role + ".bin", Role: role, Size: int64(len(data[j])), Hash: digest(string(data[j])), URL: "https://example.com/" + role})
	}
	if w := call(ordinary, "POST", "speech-models", v); w.Code != 403 {
		t.Fatal("tenant admin modified global catalog", w.Code)
	}
	if w := call(admin, "POST", "speech-models", v); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(admin, "POST", "speech-models", v); w.Code != 200 {
		t.Fatal("idempotent manifest", w.Code)
	}
	changed := v
	changed.Label = "different"
	if w := call(admin, "POST", "speech-models", changed); w.Code != 409 {
		t.Fatal("mutable pinned version", w.Code)
	}
	publish := "speech-models/" + v.ID + "/" + v.Version + "/publish"
	attest := map[string]bool{"compatibility_tested": true, "redistribution_allowed": true}
	if w := call(admin, "POST", publish, attest); w.Code != 409 {
		t.Fatal("incomplete model published", w.Code)
	}
	first := v.Files[0]
	path := "speech-models/blobs/" + first.Hash
	if w := call(admin, "PUT", path+"?offset=0", data[0][:5]); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(admin, "PUT", path+"?offset=0", data[0][:5]); w.Code != 409 || !strings.Contains(w.Body.String(), `"uploaded":5`) {
		t.Fatal("resume offset", w.Code, w.Body.String())
	}
	if w := call(admin, "PUT", path+"?offset=5", bytes.Repeat([]byte("x"), len(data[0])-5)); w.Code != 422 {
		t.Fatal("corrupt upload accepted", w.Code)
	}
	for j, f := range v.Files {
		if w := call(admin, "PUT", "speech-models/blobs/"+f.Hash+"?offset=0", data[j]); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call(ordinary, "GET", path, nil); w.Code != 404 {
		t.Fatal("unpublished blob leaked", w.Code)
	}
	if w := call(admin, "GET", path, nil); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data[0]) {
		t.Fatal("admin preview failed", w.Code)
	}
	if w := call(admin, "POST", publish, map[string]bool{}); w.Code != 422 {
		t.Fatal("untested model published", w.Code)
	}
	if w := call(admin, "POST", publish, attest); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call(ordinary, "GET", "speech-models", nil)
	var out struct {
		Models []struct {
			Canonical string `json:"canonical_json"`
			Hash      string `json:"sha256"`
		} `json:"models"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || w.Code != 200 || len(out.Models) != 1 {
		t.Fatal("published catalog", w.Code, w.Body.String(), e)
	}
	if digest(out.Models[0].Canonical) != out.Models[0].Hash {
		t.Fatal("canonical hash mismatch")
	}
	if w := call(ordinary, "GET", path, nil); w.Code != 200 {
		t.Fatal("published fallback not downloadable", w.Code)
	}
	if w := call(ordinary, "GET", "speech-models/manage", nil); w.Code != 403 {
		t.Fatal("admin catalog leaked", w.Code)
	}
	r := httptest.NewRequest("GET", "http://localhost/v1/"+path, nil)
	r.Header.Set("Range", "bytes=2-6")
	w = httptest.NewRecorder()
	m.speechModels(w, r, ordinary, strings.Split(path, "/"))
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[0][2:7]) {
		t.Fatal("range download", w.Code, w.Body.String())
	}
	for i := 0; i < 3; i++ {
		if e := m.queueModelJob(ctx, v.ID, v.Version, "mirror"); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	m.db.QueryRow(ctx, `SELECT count(*) FROM wai_speech_model_jobs WHERE model_id=$1 AND state='queued'`, v.ID).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate worker jobs", n)
	}
	// A mirror response with the expected size but wrong bytes is never committed.
	bad := speechModelFile{Path: "test.bin", Size: 3, Hash: digest("abc"), URL: "https://example.com/bad"}
	client := &http.Client{Transport: modelRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("bad"))}, nil
	})}
	if e := m.mirrorModelFile(ctx, client, bad); e == nil {
		t.Fatal("corrupt upstream mirror committed")
	}
}
