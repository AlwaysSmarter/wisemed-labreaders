package wisemedai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestThyroidVocabularyCoversReferenceStructures(t *testing.T) {
	tpl := thyroidUltrasoundTemplate(catalogSpec{Name: "Ecografie tiroidiană", Specialty: "Radiologie și imagistică medicală"})
	if err := tpl.validate(); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]string{
		"tiroida":        {"capsulă", "contur boselat"},
		"lob_drept":      {"microplaje cu ecogenități diferite", "striuri fibroase", "aspect pseudonodular", "halou hipoecogen gros", "vascularizație perilezională"},
		"lob_stang":      {"aspect fin granular", "formațiune microchistică", "calcificare în coajă de ou", "structură mixtă", "formațiuni adiacente"},
		"istm":           {"istmico-lobar stâng", "istmico-lobar drept", "halou hipoecogen subțire parțial"},
		"vascularizatie": {"vascularizație discret accentuată difuz", "vascularizație în limite normale"},
		"ganglioni":      {"micropoliadenopatie laterocervicală bilaterală", "caracter nespecific", "fără adenopatie laterocervicală"},
	}
	compiled := (&Module{}).compileContext(tpl, nil)
	for id, wants := range expected {
		var terms []string
		for _, section := range compiled.Sections {
			if section.ID == id {
				terms = section.SpeechTerms
			}
		}
		for _, want := range wants {
			if !containsTerm(terms, want) {
				t.Errorf("%s missing %s", id, want)
			}
		}
	}
	for _, field := range tpl.Fields {
		if field.Repeatable != (strings.HasPrefix(field.Key, "noduli_") || field.Key == "ganglioni") {
			t.Errorf("unexpected repeated field %s", field.Key)
		}
	}
	// No clinical observation from the source is emitted with an empty dictation.
	result, err := buildResult(tpl, nil, nil, "synthetic", 1)
	if err != nil || strings.Contains(result.HTML, "hipoecogen") || strings.Contains(result.HTML, "adenopatie") || strings.Contains(result.HTML, "[nedocumentat]") {
		t.Fatal(err, result.HTML)
	}
}
func containsTerm(terms []string, want string) bool {
	for _, term := range terms {
		if term == want {
			return true
		}
	}
	return false
}
func TestSpeechVocabularyOverridesAndBudgets(t *testing.T) {
	base := []string{"hipoecogenă", "striuri fibroase", "aspect fin granular"}
	entries := mergeTemplateSpeechTerms(base, []speechTerm{{Text: "hipoecogenă", Status: "rejected"}, {Text: "striuri fibroase", Status: "candidate"}, {Text: "ecostructură", Status: "approved"}})
	if got := activeSpeechTerms(entries); !reflect.DeepEqual(got, []string{"ecostructură", "aspect fin granular"}) {
		t.Fatal(got)
	}
	many := []speechTerm{}
	for i := 0; i < 100; i++ {
		many = append(many, speechTerm{Text: "ecostructură " + strings.Repeat("a", i+1), Status: "approved"})
	}
	got := activeSpeechTerms(many)
	budget := 0
	for _, term := range got {
		budget += len(term) + 1
	}
	if budget > activeSpeechBudget || len(got) > 64 {
		t.Fatal("budget exceeded", len(got), budget)
	}
	tpl := testTemplate()
	tpl.Sections[0].SpeechTerms = []string{"0.3.2 cm"}
	if tpl.validate() == nil {
		t.Fatal("measurement accepted as dictionary term")
	}
}

func TestPostgresThyroidPhoneAndOpenAIShareVocabulary(t *testing.T) {
	m, rt := integrationModule(t)
	ctx := context.Background()
	secret, err := os.ReadFile(rt.ResolvePath(m.cfg.BootstrapPasswordFile))
	if err != nil {
		t.Fatal(err)
	}
	_, login := request(t, m, "", "POST", "auth/login", map[string]any{"email": m.cfg.BootstrapEmail, "password": string(secret)})
	token := login["token"].(string)
	profile := login["profile"].(map[string]any)
	p := principal{Tenant: profile["tenant_id"].(string), User: profile["user_id"].(string), InstanceAdmin: true, Role: "admin"}
	var raw []byte
	err = m.db.QueryRow(ctx, `SELECT v.document FROM wai_templates t JOIN wai_template_versions v ON v.id=t.current_version WHERE t.tenant_id=$1 AND t.name='Ecografie tiroidiană'`, p.Tenant).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var tpl Template
	if err = json.Unmarshal(raw, &tpl); err != nil {
		t.Fatal(err)
	}
	section := "camp_ecostructura_stanga"
	for _, entry := range []struct{ text, status string }{{"ecostructură fin granulară", "approved"}, {"hipoecogenă", "rejected"}} {
		code, body := request(t, m, token, "POST", "templates/"+tpl.ID+"/sections/"+section+"/speech-terms?version_id="+tpl.VersionID, map[string]any{"text": entry.text, "status": entry.status})
		if code != 200 {
			t.Fatal(code, body)
		}
	}
	code, pack := request(t, m, token, "GET", "templates/"+tpl.ID+"/speech-adaptation?version_id="+tpl.VersionID, nil)
	if code != 200 {
		t.Fatal(code, pack)
	}
	phone := map[string][]string{}
	for _, entry := range pack["sections"].([]any) {
		s := entry.(map[string]any)
		for _, term := range s["terms"].([]any) {
			phone[s["id"].(string)] = append(phone[s["id"].(string)], term.(string))
		}
	}
	if !containsTerm(phone[section], "ecostructură fin granulară") || containsTerm(phone[section], "hipoecogenă") {
		t.Fatal(phone[section])
	}
	m.cfg.Provider = "openai"
	m.cfg.OpenAIKeyEnv = "WAI_TEST_THYROID_KEY"
	t.Setenv(m.cfg.OpenAIKeyEnv, "synthetic-key")
	var model compiledInput
	m.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Messages []promptMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, message := range body.Messages {
			if message.Role == "user" {
				if err := json.Unmarshal([]byte(message.Content), &model); err != nil {
					t.Fatal(err)
				}
			}
		}
		reply := `{"choices":[{"finish_reason":"stop","message":{"content":"{\"facts\":[]}"}}],"usage":{"prompt_tokens":100,"completion_tokens":10}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply)), Header: http.Header{}}, nil
	})}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = m.extract(ctx, tx, p, tpl, []Chunk{{ID: "c1", Text: "Lob stâng cu aspect fin granular."}}); err != nil {
		t.Fatal(err)
	}
	if model.TemplateContext == nil || !strings.Contains(model.TemplateContext.Instructions, "noduli_istm") {
		t.Fatal("thyroid instructions not sent")
	}
	for _, s := range model.Sections {
		if !reflect.DeepEqual(s.SpeechTerms, phone[s.ID]) && len(s.SpeechTerms)+len(phone[s.ID]) > 0 {
			t.Errorf("different phone/model terms for %s", s.ID)
		}
	}
}
