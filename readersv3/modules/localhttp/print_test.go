package localhttp

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"wisemed-labreaders/readersv3/core/module"
	model "wisemed-labreaders/readersv3/modules/core/model"
)

type printStoreStub struct {
	orderStore
	dailyDetailStore
	bundles []model.OrderBundle
	round   int
	date    string
}

func (s *printStoreStub) ListOrderBundles(round int, date string) ([]model.OrderBundle, error) {
	s.round, s.date = round, date
	return s.bundles, nil
}
func (*printStoreStub) ListRoundNumbers(string) ([]int, error) { return []int{1, 3}, nil }
func (*printStoreStub) ListDailyDetailValues(string, int) ([]model.DailyDetailValue, error) {
	return []model.DailyDetailValue{{DefinitionKey: "cod_formular_fisa_lucru", ScopeDate: "2026-09-22", ValueText: "FORM-123", AnalyteTag: "A"}, {DefinitionKey: "amartor", ScopeDate: "2026-09-22", AnalyteTag: "A", ValueText: "0.25"}}, nil
}
func (*printStoreStub) ListDailyDetailDefinitions() ([]model.DailyDetailDefinition, error) {
	return nil, nil
}

type printRuntimeStub struct {
	module.Runtime
	store *printStoreStub
}

func (r printRuntimeStub) ReaderID() string { return "print-reader" }
func (r printRuntimeStub) Service(name string) (interface{}, bool) {
	if name == "storage" {
		return r.store, true
	}
	if name == "analyzer-config" {
		return map[string]interface{}{"protocol": "cary60-uvvis"}, true
	}
	return nil, false
}
func (printRuntimeStub) ModuleSettings(string) map[string]interface{} { return nil }

func printFixtures() []model.OrderBundle {
	return []model.OrderBundle{{Order: model.Order{SampleID: "sample-2", SampleNo: 2, OrderDate: "2026-09-22", PatientName: "<script>alert(1)</script>", Meta: map[string]interface{}{"sent_sample_code": "SENT-2"}}, Analyses: []model.OrderAnalysisBundle{
		{Analysis: model.OrderAnalysis{AnalyteTag: "B", AnalyteName: "Beta", ResultValue: "23", Unit: "mg/L", Flags: map[string]interface{}{"domain": "0-50", "measured_concentration": "21", "final_concentration": "42", "dilution_factor": "2"}}},
		{Analysis: model.OrderAnalysis{AnalyteTag: "A", AnalyteName: "Alpha", RawValue: "7", ResultValue: "8"}},
	}}, {Order: model.Order{SampleID: "sample-1", SampleNo: 1, OrderDate: "2026-09-22"}, Analyses: []model.OrderAnalysisBundle{{Analysis: model.OrderAnalysis{AnalyteTag: "B", AnalyteName: "Beta", RawValue: "10"}}}}}
}

func TestPrintEndpointsJSONAndHTMLUseSameDocument(t *testing.T) {
	for _, endpoint := range []string{"/api/orders/worklist?order_date=2026-09-22", "/api/daily-details/worksheet?order_date=2026-09-22&scope=day_analyte&analyte_tag=A&round_no=9"} {
		t.Run(endpoint, func(t *testing.T) {
			store := &printStoreStub{bundles: printFixtures()}
			m := &Module{rt: printRuntimeStub{store: store}, analytes: []model.Analyte{{Tag: "A", Name: "Alpha", ResultMeasureUnit: "mmol/L"}}}
			handler := m.handleOrdersWorklist
			if strings.Contains(endpoint, "worksheet") {
				handler = m.handleDailyDetailsWorksheet
			}
			jsonResponse := httptest.NewRecorder()
			handler(jsonResponse, httptest.NewRequest(http.MethodGet, endpoint+"&format=json", nil))
			if jsonResponse.Code != http.StatusOK || !strings.HasPrefix(jsonResponse.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("JSON failed: %d %s", jsonResponse.Code, jsonResponse.Body)
			}
			var response struct {
				OK       bool          `json:"ok"`
				Document printDocument `json:"document"`
			}
			if err := json.Unmarshal(jsonResponse.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.OK || len(response.Document.Rows) == 0 || strings.Contains(jsonResponse.Body.String(), "<!doctype") || strings.Contains(jsonResponse.Body.String(), "<table") {
				t.Fatal("print response contains UI instead of data")
			}
			htmlResponse := httptest.NewRecorder()
			handler(htmlResponse, httptest.NewRequest(http.MethodGet, endpoint, nil))
			if htmlResponse.Code != http.StatusOK || !strings.HasPrefix(htmlResponse.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("HTML failed: %d", htmlResponse.Code)
			}
			if got, want := htmlResponse.Body.String(), renderPrintDocumentHTML(response.Document); got != want {
				t.Fatal("HTTP and JSON document paths diverged")
			}
			for _, row := range response.Document.Rows {
				for _, value := range row {
					if value != "" && !strings.Contains(htmlResponse.Body.String(), html.EscapeString(value)) {
						t.Fatalf("HTML lost document value %q", value)
					}
				}
			}
			if strings.Contains(endpoint, "worksheet") {
				if store.round != 0 || response.Document.FormCode != "FORM-123" || len(response.Document.Rows) != 1 || response.Document.Rows[0][5] != "A = 8" {
					t.Fatalf("worksheet scope/form/filter changed: %+v", response.Document)
				}
				if strings.Contains(htmlResponse.Body.String(), "<script>") {
					t.Fatal("print data escaped into executable markup")
				}
			} else {
				if store.round != 3 || response.Document.Rows[0][0] != "sample-1" || response.Document.Rows[1][2] != "mmol/L" || response.Document.Rows[2][2] != "0-50 / mg/L" || !reflect.DeepEqual(response.Document.Rows[2][3:6], []string{"21", "2", "42"}) {
					t.Fatalf("worklist ordering/values changed: %+v", response.Document)
				}
				if len(response.Document.Details) != 2 || response.Document.Details[0].Value != "0.25" {
					t.Fatalf("A martor data missing: %+v", response.Document.Details)
				}
			}
		})
	}
}

func TestWorksheetJSONEmptyDataHasNoMarkup(t *testing.T) {
	document := (&Module{}).buildDailyWorksheetDocument("2026-09-22", "day", 0, "", "", nil, nil)
	w := httptest.NewRecorder()
	writePrintDocument(w, httptest.NewRequest(http.MethodGet, "/api/daily-details/worksheet?format=json", nil), document)
	var response struct {
		Document printDocument `json:"document"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Document.Rows == nil || len(response.Document.Rows) != 0 {
		t.Fatal("empty rows must be an array")
	}
	if strings.Contains(w.Body.String(), "Nu exista") || strings.Contains(w.Body.String(), "<td") {
		t.Fatal("empty-state markup leaked into JSON")
	}
}
