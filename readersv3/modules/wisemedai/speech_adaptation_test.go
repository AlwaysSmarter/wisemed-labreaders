package wisemedai

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSpeechDiffSafety(t *testing.T) {
	tests := []struct{ raw, final, want string }{
		{"aspect hipoegogen omogen", "aspect hipoecogen omogen", "hipoecogen"},
		{"ficat omogen", "ficat omogen mărit", ""},
		{"nu prezintă noduli", "prezintă noduli", ""},
		{"nodul 11 mm", "nodul 13 mm", ""},
		{"aspect normal", "aspect Renaldo", ""},
		{"pacient ionescu", "pacient hipoecogen", ""},
	}
	for _, tc := range tests {
		term, _ := speechCorrection(tc.raw, tc.final)
		if term != tc.want {
			t.Errorf("%q -> %q = %q", tc.raw, tc.final, term)
		}
	}
	if normalizeSpeech("  hipoecogen\u0061\u0306  renal ") != "hipoecogenă renal" {
		t.Fatal("Unicode normalization")
	}
	if speechKey("hipoecogen") == speechKey("hipoecogene") {
		t.Fatal("inflected terms merged")
	}
	for _, s := range []string{"Ion Popescu", "1234567891234", "11 mm", "nume pacient", "test@example.ro"} {
		if safeSpeechTerm(s) {
			t.Fatal("identifier accepted", s)
		}
	}
}
func TestSpeechSanitizationRequiresVerifiedAlignment(t *testing.T) {
	wav := testSpeechWAV(1000)
	review := speechReview{Verified: true, Listened: true, Deidentified: true, Transcript: "Ion Popescu ficat omogen",
		Alignment:  []speechSpan{{0, 300, "Ion Popescu"}, {300, 650, "ficat"}, {650, 1000, "omogen"}},
		Redactions: []speechSpan{{Start: 0, End: 300}}}
	out, text, e := sanitizedSpeech(wav, review)
	if e != nil || text != "ficat omogen" || !bytes.Equal(out[44:44+300*32], make([]byte, 300*32)) {
		t.Fatal("redaction failed", e, text)
	}
	review.Redactions = nil
	if _, _, e = sanitizedSpeech(wav, review); e == nil {
		t.Fatal("PII entered dataset")
	}
	review.Verified = false
	if _, _, e = sanitizedSpeech(wav, review); e == nil {
		t.Fatal("unverified report entered dataset")
	}
	review.Verified = true
	review.Redactions = []speechSpan{{Start: 0, End: 300}}
	review.Alignment[1].Start = 100
	if _, _, e = sanitizedSpeech(wav, review); e == nil {
		t.Fatal("overlapping alignment accepted")
	}
}
func testSpeechWAV(ms int) []byte {
	b := make([]byte, 44+ms*32)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(b)-44))
	for i := 44; i < len(b); i++ {
		b[i] = 11
	}
	return b
}
func TestPostgresSpeechAdaptationAndCommunity(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	secret, e := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if e != nil {
		t.Fatal(e)
	}
	code, login := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(secret)})
	if code != 200 {
		t.Fatal(code, login)
	}
	token := login["token"].(string)
	profile := login["profile"].(map[string]any)
	tenant, user := profile["tenant_id"].(string), profile["user_id"].(string)
	var raw []byte
	if e = m.db.QueryRow(ctx, `SELECT document FROM wai_template_versions WHERE tenant_id=$1 AND document->>'name'='Ecografie abdominală' ORDER BY version DESC LIMIT 1`, tenant).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var tpl Template
	if e = json.Unmarshal(raw, &tpl); e != nil {
		t.Fatal(e)
	}
	section := tpl.Sections[len(tpl.Sections)-1].ID
	session, chunk, event := uuid.NewString(), uuid.NewString(), uuid.NewString()
	encrypted, _ := m.box.seal(map[string]any{}, tenant+":"+session)
	if _, e = m.db.Exec(ctx, `INSERT INTO wai_sessions(id,tenant_id,user_id,template_version,status,revision,context)VALUES($1,$2,$3,$4,'approved',2,$5)`, session, tenant, user, tpl.VersionID, encrypted); e != nil {
		t.Fatal(e)
	}
	encrypted, _ = m.box.seal("aspect hipoecogen omogen", tenant+":"+session+":"+chunk)
	if _, e = m.db.Exec(ctx, `INSERT INTO wai_session_chunks(id,tenant_id,session_id,sequence,revision,is_final,timestamp,text,digest)VALUES($1,$2,$3,1,1,true,now(),$4,'test')`, chunk, tenant, session, encrypted); e != nil {
		t.Fatal(e)
	}
	if _, e = m.db.Exec(ctx, `INSERT INTO wai_approvals(id,tenant_id,session_id,user_id,revision)VALUES($1,$2,$3,$4,2)`, uuid.NewString(), tenant, session, user); e != nil {
		t.Fatal(e)
	}
	code, consent := request(t, m, token, "PUT", "profile/speech-consent", map[string]any{"enabled": true, "notice_version": speechNotice})
	if code != 200 {
		t.Fatal(code, consent)
	}
	segment := speechSegment{ID: uuid.NewString(), ChunkID: chunk, Raw: "aspect hipoegogen omogen", Corrected: []string{"aspect hipoecogen omogen"}, Model: "synthetic", Runtime: "synthetic", Mode: "local", StartMS: 0, EndMS: 1000, CaptureStarted: time.Now().UTC().Format(time.RFC3339Nano), ConsentGeneration: consent["generation"].(string)}
	feedback := speechFeedbackInput{ID: event, Version: tpl.VersionID, Session: session, Revision: 2, Section: section, Final: "accepted report", Segments: []speechSegment{segment}}
	path := "templates/" + tpl.ID + "/sections/" + section + "/speech-feedback?version_id=" + tpl.VersionID
	const parallel = 5
	statuses := make(chan int, parallel)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "http://localhost/v1/"+path, strings.NewReader(toJSON(feedback)))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			m.serve(w, r)
			statuses <- w.Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatal("concurrent feedback", status)
		}
	}
	var count int
	if e = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_speech_feedback WHERE tenant_id=$1`, tenant).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate feedback", count, e)
	}
	if e = m.db.QueryRow(ctx, `SELECT observations FROM wai_speech_terms WHERE tenant_id=$1 AND template_id=$2 AND section_id=$3`, tenant, tpl.ID, section).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate learning", count, e)
	}
	code, pack := request(t, m, token, "GET", "templates/"+tpl.ID+"/speech-adaptation?version_id="+tpl.VersionID, nil)
	if code != 200 || pack["canonical_json"] == nil {
		t.Fatal(code, pack)
	}
	// Revalidation uses a hash of the exact canonical UTF-8 package.
	if digest(pack["canonical_json"].(string)) != pack["sha256"] {
		t.Fatal("invalid package checksum")
	}
	r := httptest.NewRequest("GET", "http://localhost/v1/templates/"+tpl.ID+"/speech-adaptation?version_id="+tpl.VersionID, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("If-None-Match", string(rune(34))+pack["sha256"].(string)+string(rune(34)))
	w := httptest.NewRecorder()
	m.serve(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal("ETag not respected", w.Code)
	}
	// The exact same template ID is inaccessible under a different organization.
	w = httptest.NewRecorder()
	m.speechAdaptation(w, r, principal{Tenant: uuid.NewString(), User: user, Role: "admin"}, []string{"templates", tpl.ID, "speech-adaptation"})
	if w.Code != 404 {
		t.Fatal("cross-tenant package leak", w.Code)
	}
	code, outTerms := request(t, m, token, "POST", "templates/"+tpl.ID+"/sections/"+section+"/speech-terms?version_id="+tpl.VersionID, map[string]any{"text": "hipoecogen", "status": "approved"})
	if code != 200 {
		t.Fatal("manual term", code, outTerms)
	}
	// Stable section IDs survive a new version with renamed/reordered sections.
	var renamed Template
	if e = json.Unmarshal(raw, &renamed); e != nil {
		t.Fatal(e)
	}
	for j := range renamed.Sections {
		if renamed.Sections[j].ID == section {
			renamed.Sections[j].Label = "Redenumită"
		}
	}
	for left, right := 0, len(renamed.Sections)-1; left < right; left, right = left+1, right-1 {
		renamed.Sections[left], renamed.Sections[right] = renamed.Sections[right], renamed.Sections[left]
	}
	tx, e := m.db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = saveTemplate(ctx, tx, tenant, &renamed); e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	code, nextPack := request(t, m, token, "GET", "templates/"+tpl.ID+"/speech-adaptation?version_id="+renamed.VersionID, nil)
	if code != 200 {
		t.Fatal(code, nextPack)
	}
	found := false
	for _, entry := range nextPack["sections"].([]any) {
		sec := entry.(map[string]any)
		terms := sec["terms"].([]any)
		if sec["id"] == section {
			found = true
			if sec["label"] != "Redenumită" || len(terms) != 1 || terms[0] != "hipoecogen" {
				t.Fatal("vocabulary lost after reorder", sec)
			}
		} else if len(terms) != 0 {
			t.Fatal("section vocabulary contamination", sec)
		}
	}
	if !found {
		t.Fatal("section missing after reorder")
	}
	// Unknown sections cannot contaminate vocabulary.
	code, _ = request(t, m, token, "POST", "templates/"+tpl.ID+"/sections/missing/speech-terms", map[string]any{"text": "hipoecogen", "status": "approved"})
	if code != 404 {
		t.Fatal("unknown section accepted", code)
	}
	wav := testSpeechWAV(1000)
	contribution := speechContribution{ID: uuid.NewString(), SourceSession: session, Template: tpl.ID, Version: tpl.VersionID, Section: section, Revision: 2, Segment: segment, Final: "accepted report", Notice: speechNotice, Bytes: len(wav), Hash: digest(string(wav)), DurationMS: 1000, Codec: "pcm_s16le_wav", SampleRate: 16000, Channels: 1, Language: "ro"}
	code, out := request(t, m, token, "POST", "community/speech-contributions", contribution)
	if code != 201 {
		t.Fatal(code, out)
	}
	base := "community/speech-contributions/" + contribution.ID
	code, _ = request(t, m, token, "POST", base+"/finalize", map[string]any{})
	if code != 409 {
		t.Fatal("partial finalized", code)
	}
	upload := func(offset int, b []byte) int {
		r := httptest.NewRequest("PUT", "http://localhost/v1/"+base+"/audio?offset="+strconvI(offset), bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		m.serve(w, r)
		return w.Code
	}
	if upload(0, wav[:16000]) != 200 || upload(0, wav[:16000]) != 200 || upload(16000, wav[16000:]) != 200 {
		t.Fatal("resumable upload")
	}
	code, out = request(t, m, token, "POST", base+"/finalize", map[string]any{})
	if code != 200 || out["state"] != "pending_deidentification" {
		t.Fatal(code, out)
	}
	review := speechReview{Decision: "approved_for_dataset", SpeakerKey: "synthetic-speaker-01", SingleSpeaker: true, Transcript: "ficat omogen", Verified: true, Listened: true, Deidentified: true, Alignment: []speechSpan{{0, 500, "ficat"}, {500, 1000, "omogen"}}}
	code, out = request(t, m, token, "POST", base+"/review", review)
	if code != 200 {
		t.Fatal(code, out)
	}
	code, out = request(t, m, token, "POST", "community/speech-contributions/export", map[string]any{"dataset_id": uuid.NewString()})
	if code != 200 || len(out["items"].([]any)) != 1 {
		t.Fatal(code, out)
	}
	exported := out["items"].([]any)[0].(map[string]any)
	if exported["transcript"] != "ficat omogen" || exported["speaker_id"] == user {
		t.Fatal("unsafe manifest", exported)
	}
	code, out = request(t, m, token, "PUT", "profile/speech-consent", map[string]any{"enabled": false, "notice_version": speechNotice})
	if code != 200 {
		t.Fatal(code, out)
	}
	code, _ = request(t, m, token, "GET", base, nil)
	if code != 404 {
		t.Fatal("withdrawn data accessible", code)
	}
	if e = m.db.QueryRow(ctx, `SELECT count(*) FROM wai_speech_dataset_entries WHERE revoked`).Scan(&count); e != nil || count != 1 {
		t.Fatal("export provenance revocation", e)
	}
	// Same old capture cannot be contributed after reactivation.
	_, _ = request(t, m, token, "PUT", "profile/speech-consent", map[string]any{"enabled": true, "notice_version": speechNotice})
	code, _ = request(t, m, token, "POST", "community/speech-contributions", contribution)
	if code != 409 {
		t.Fatal("retroactive consent accepted", code)
	}
}
func strconvI(i int) string { return fmt.Sprint(i) }
