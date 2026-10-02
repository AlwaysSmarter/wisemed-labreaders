package localhttp

import (
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

// printDocument carries the already computed business data for both HTTP print
// and JSON-only remote transports. Rendering never repeats result calculation.
type printDocument struct {
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	OrderDate    string        `json:"order_date"`
	RoundNo      int           `json:"round_no"`
	Scope        string        `json:"scope,omitempty"`
	ScopeLabel   string        `json:"scope_label,omitempty"`
	AnalyteTag   string        `json:"analyte_tag,omitempty"`
	FormCode     string        `json:"form_code,omitempty"`
	DetailsTitle string        `json:"details_title,omitempty"`
	Details      []printDetail `json:"details"`
	Columns      []string      `json:"columns"`
	Rows         [][]string    `json:"rows"`
}
type printDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func writePrintDocument(w http.ResponseWriter, r *http.Request, document printDocument) {
	if document.Details == nil {
		document.Details = []printDetail{}
	}
	if document.Rows == nil {
		document.Rows = [][]string{}
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "json") {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "document": document})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderPrintDocumentHTML(document)))
}

func renderPrintDocumentHTML(document printDocument) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ro"><head><meta charset="utf-8"><title>` + esc(document.Title) + `</title><style>
body{font-family:Arial,sans-serif;margin:24px;color:#111}h1,h2{margin:0 0 12px}.meta,.form-code{margin-bottom:20px;color:#444;font-size:12px}table{width:100%;border-collapse:collapse;margin:0 0 20px}th,td{border:1px solid #222;padding:8px 10px;font-size:12px;vertical-align:top}th{background:#f2f2f2}.details{width:auto;min-width:340px}.sign{height:32px;min-width:120px}@media print{body{margin:8mm}.print-btn{display:none}}
</style></head><body><button class="print-btn" onclick="window.print()">Print</button><h1>` + esc(document.Title) + `</h1><div class="meta">Data: ` + esc(document.OrderDate))
	if document.ScopeLabel != "" {
		b.WriteString(` · Scope: ` + esc(document.ScopeLabel))
	}
	if document.RoundNo > 0 {
		b.WriteString(` · Runda: ` + strconv.Itoa(document.RoundNo))
	}
	if document.AnalyteTag != "" {
		b.WriteString(` · Analiza: ` + esc(document.AnalyteTag))
	}
	b.WriteString(`</div>`)
	if document.FormCode != "" {
		b.WriteString(`<div class="form-code"><strong>Cod formular:</strong> ` + esc(document.FormCode) + `</div>`)
	}
	if document.DetailsTitle != "" {
		b.WriteString(`<h2>` + esc(document.DetailsTitle) + `</h2>`)
	}
	if len(document.Details) > 0 {
		b.WriteString(`<table class="details"><tbody>`)
		for _, item := range document.Details {
			b.WriteString(`<tr><th>` + esc(item.Label) + `</th><td>` + esc(item.Value) + `</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`<table><thead><tr>`)
	for _, column := range document.Columns {
		b.WriteString(`<th>` + esc(column) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, row := range document.Rows {
		b.WriteString(`<tr>`)
		for i, cell := range row {
			if document.Kind == "worklist" && i >= 6 {
				b.WriteString(`<td class="sign">`)
			} else {
				b.WriteString(`<td>`)
			}
			b.WriteString(esc(cell) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}
	if len(document.Rows) == 0 {
		b.WriteString(`<tr><td colspan="` + strconv.Itoa(len(document.Columns)) + `">Nu exista cereri pentru filtrul selectat.</td></tr>`)
	}
	b.WriteString(`</tbody></table></body></html>`)
	return b.String()
}

func (m *Module) buildCaryWorklistDocument(orderDate string, roundNo int, bundles []coremodel.OrderBundle, analyteIndex map[string]coremodel.Analyte, definitions []coremodel.DailyDetailDefinition, values []coremodel.DailyDetailValue) printDocument {
	type headerItem struct {
		Tag      string
		Name     string
		AMartor  string
		Worklist string
	}
	valueIndex := map[string]string{}
	for _, item := range values {
		key := item.DefinitionKey + "|" + item.ScopeDate + "|" + strconv.Itoa(item.RoundNo) + "|" + strings.ToUpper(strings.TrimSpace(item.AnalyteTag))
		valueIndex[key] = item.ValueText
	}
	headerMap := map[string]headerItem{}
	for _, bundle := range bundles {
		for _, analysis := range bundle.Analyses {
			tag := strings.TrimSpace(analysis.Analysis.AnalyteTag)
			if tag == "" {
				continue
			}
			if _, ok := headerMap[tag]; ok {
				continue
			}
			analyte := analyteIndex[tag]
			amartor := firstNonEmpty(
				valueIndex["amartor|"+orderDate+"|0|"+strings.ToUpper(tag)],
				valueIndex["amartor|"+orderDate+"|"+strconv.Itoa(roundNo)+"|"+strings.ToUpper(tag)],
			)
			worklistLabel := strings.TrimSpace(asString(analyte.ProtocolOptions["worklist_label"]))
			if worklistLabel == "" {
				worklistLabel = strings.TrimSpace(analyte.ResultMeasureUnit)
			}
			headerMap[tag] = headerItem{
				Tag:      tag,
				Name:     firstNonEmpty(analyte.Name, analysis.Analysis.AnalyteName, tag),
				AMartor:  amartor,
				Worklist: worklistLabel,
			}
		}
	}
	headers := make([]headerItem, 0, len(headerMap))
	for _, item := range headerMap {
		headers = append(headers, item)
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	rows := make([][]string, 0)
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Order.SampleNo != bundles[j].Order.SampleNo {
			return bundles[i].Order.SampleNo < bundles[j].Order.SampleNo
		}
		return bundles[i].Order.SampleID < bundles[j].Order.SampleID
	})
	for _, bundle := range bundles {
		sort.Slice(bundle.Analyses, func(i, j int) bool {
			return firstNonEmpty(bundle.Analyses[i].Analysis.AnalyteName, bundle.Analyses[i].Analysis.AnalyteTag) < firstNonEmpty(bundle.Analyses[j].Analysis.AnalyteName, bundle.Analyses[j].Analysis.AnalyteTag)
		})
		for _, item := range bundle.Analyses {
			flags := item.Analysis.Flags
			worklistLabel := strings.TrimSpace(asString(flags["worklist_label"]))
			if worklistLabel == "" {
				if domain := strings.TrimSpace(asString(flags["domain_label"])); domain != "" {
					worklistLabel = domain
				} else if domain := strings.TrimSpace(asString(flags["domain"])); domain != "" {
					worklistLabel = domain
				}
				if unit := strings.TrimSpace(item.Analysis.Unit); unit != "" {
					if worklistLabel != "" {
						worklistLabel += " / " + unit
					} else {
						worklistLabel = unit
					}
				}
			}
			if worklistLabel == "" {
				analyte := analyteIndex[strings.TrimSpace(item.Analysis.AnalyteTag)]
				worklistLabel = strings.TrimSpace(asString(analyte.ProtocolOptions["worklist_label"]))
				if worklistLabel == "" {
					worklistLabel = strings.TrimSpace(analyte.ResultMeasureUnit)
				}
			}
			rows = append(rows, []string{
				bundle.Order.SampleID, bundle.Order.OrderDate, worklistLabel,
				firstNonEmpty(asString(flags["measured_concentration"]), item.Analysis.RawValue),
				firstNonEmpty(asString(flags["dilution_factor"]), "-"),
				firstNonEmpty(asString(flags["final_concentration"]), item.Analysis.ResultValue), "", "",
			})
		}
	}
	details := make([]printDetail, 0, len(headers))
	for _, item := range headers {
		details = append(details, printDetail{Label: item.Name, Value: item.AMartor})
	}
	return printDocument{Kind: "worklist", Title: "Lista de lucru", OrderDate: orderDate, RoundNo: roundNo,
		DetailsTitle: "A martor", Details: details,
		Columns: []string{"Cod proba", "Data analizei", "Domeniu de lucru / UM", "Concentratie masurata", "Dilutie", "Concentratie finala", "Executant", "Responsabil"}, Rows: rows}
}

func (m *Module) dailyWorksheetMeta(scopeDate, scope string, roundNo int, analyteTag string) (string, []printDetail, error) {
	definitions, err := m.combinedDailyDetailDefinitions()
	if err != nil {
		return "", nil, err
	}
	values, err := m.listDailyDetailValues(scopeDate, roundNo)
	if err != nil {
		return "", nil, err
	}
	usesRound := scope == "day_round" || scope == "day_round_analyte"
	usesAnalyte := scope == "day_analyte" || scope == "day_round_analyte"
	currentRound := 0
	currentAnalyte := ""
	if usesRound {
		currentRound = roundNo
	}
	if usesAnalyte {
		currentAnalyte = strings.TrimSpace(analyteTag)
	}
	findValue := func(key string) string {
		for _, item := range values {
			if strings.TrimSpace(item.DefinitionKey) != key {
				continue
			}
			if item.RoundNo != currentRound {
				continue
			}
			if strings.TrimSpace(item.AnalyteTag) != currentAnalyte {
				continue
			}
			return strings.TrimSpace(item.ValueText)
		}
		return ""
	}
	formCode := findValue("cod_formular_fisa_lucru")
	rows := []printDetail{}
	for _, def := range definitions {
		key := strings.TrimSpace(def.Key)
		if key == "" {
			continue
		}
		if key != "cod_formular_fisa_lucru" && def.Scope != scope {
			continue
		}
		value := findValue(key)
		if value == "" {
			value = strings.TrimSpace(def.DefaultValue)
		}
		if key == "cod_formular_fisa_lucru" {
			if formCode == "" {
				formCode = value
			}
			continue
		}
		if value == "" {
			continue
		}
		rows = append(rows, printDetail{Label: firstNonEmpty(def.Label, key), Value: value})
	}
	return formCode, rows, nil
}

func (m *Module) buildDailyWorksheetDocument(orderDate, scope string, roundNo int, analyteTag, formCode string, detailRows []printDetail, bundles []coremodel.OrderBundle) printDocument {
	scopeLabel := map[string]string{
		"day":               "Pe zi",
		"day_round":         "Pe zi si runda",
		"day_analyte":       "Pe zi si analiza",
		"day_round_analyte": "Pe zi, runda si analiza",
	}[scope]
	if scopeLabel == "" {
		scopeLabel = scope
	}
	rows := make([][]string, 0, len(bundles))
	for _, bundle := range bundles {
		analyses := make([]string, 0, len(bundle.Analyses))
		for _, analysisBundle := range bundle.Analyses {
			analysis := analysisBundle.Analysis
			if analyteTag != "" && !strings.EqualFold(strings.TrimSpace(analysis.AnalyteTag), analyteTag) {
				continue
			}
			result := firstNonEmpty(strings.TrimSpace(analysis.ResultValue), strings.TrimSpace(analysis.RawValue), "-")
			analyses = append(analyses, firstNonEmpty(analysis.AnalyteTag, analysis.AnalyteName)+` = `+result)
		}
		if analyteTag != "" && len(analyses) == 0 {
			continue
		}
		rows = append(rows, []string{
			bundle.Order.SampleID, firstNonEmpty(asString(bundle.Order.Meta["sent_sample_code"]), "-"),
			firstNonEmpty(bundle.Order.FileID, "-"), firstNonEmpty(bundle.Order.PatientID, "-"),
			firstNonEmpty(bundle.Order.PatientName, "-"), strings.Join(analyses, " | "),
		})
	}
	return printDocument{Kind: "worksheet", Title: "Fisa de lucru", OrderDate: orderDate, RoundNo: roundNo,
		Scope: scope, ScopeLabel: scopeLabel, AnalyteTag: analyteTag, FormCode: firstNonEmpty(strings.TrimSpace(formCode), "-"),
		Details: detailRows, Columns: []string{"Proba", "Cod trimis", "Fisa", "Sample code", "Specimen code", "Analize"}, Rows: rows}
}
