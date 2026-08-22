package anafdocsmart

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wisemed-labreaders/readersv3/core/module"
)

type Module struct{ rt module.Runtime }

type fileTransportMeta struct {
	ImportDir    string
	ProcessedDir string
	FailedDir    string
	ExportDir    string
	Pattern      string
}

type declaration struct {
	XMLName         xml.Name  `xml:"f1102"`
	Year            string    `xml:"an,attr"`
	Month           string    `xml:"luna_r,attr"`
	DocumentDate    string    `xml:"data_document,attr"`
	InstitutionName string    `xml:"nume_ip,attr"`
	ControlSum      string    `xml:"suma_control,attr"`
	Accounts        []account `xml:"cont"`
}

type account struct {
	Symbol     string `xml:"simbol_p_cont,attr"`
	SourceCode string `xml:"cod_sursa,attr"`
	Debit      string `xml:"rulaj_deb,attr"`
	Credit     string `xml:"rulaj_cred,attr"`
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-anaf-docsmart" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.Handle("/api/protocol/meta", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "protocol": "anaf-docsmart"})
	}))
	rt.Handle("/api/docsmart/parsed-files", http.HandlerFunc(m.handleParsedFiles))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	pollSeconds := intSetting(m.rt.ModuleSettings(m.ID()), "poll_seconds", 5)
	if pollSeconds <= 0 {
		<-ctx.Done()
		return nil
	}
	meta := m.fileTransport()
	m.rt.Logf("anaf-docsmart watcher active import_dir=%s processed_dir=%s failed_dir=%s export_dir=%s pattern=%s poll_seconds=%d", meta.ImportDir, meta.ProcessedDir, meta.FailedDir, meta.ExportDir, meta.Pattern, pollSeconds)
	m.scanImportDir()
	ticker := time.NewTicker(time.Duration(pollSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.scanImportDir()
		}
	}
}

func (m *Module) handleParsedFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	files, err := m.listParsedFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "files": files})
}

func (m *Module) listParsedFiles() ([]map[string]interface{}, error) {
	meta := m.fileTransport()
	scans := []struct {
		dir      string
		location string
		status   string
	}{
		{dir: meta.ImportDir, location: "inbox", status: "pending"},
		{dir: meta.ProcessedDir, location: "processed", status: "processed"},
		{dir: meta.FailedDir, location: "failed", status: "failed"},
	}

	out := make([]map[string]interface{}, 0, 16)
	for _, scan := range scans {
		files, err := filepath.Glob(filepath.Join(scan.dir, meta.Pattern))
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		for _, path := range files {
			out = append(out, parseXMLFile(path, scan.location, scan.status))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(asString(out[i]["file_name"]))
		right := strings.ToLower(asString(out[j]["file_name"]))
		if left == right {
			return asString(out[i]["location"]) < asString(out[j]["location"])
		}
		return left < right
	})
	return out, nil
}

func (m *Module) scanImportDir() {
	meta := m.fileTransport()
	files, err := filepath.Glob(filepath.Join(meta.ImportDir, meta.Pattern))
	if err != nil {
		m.rt.Logf("anaf-docsmart glob failed: %v", err)
		return
	}
	sort.Strings(files)
	for _, path := range files {
		if err := m.processFile(path); err != nil {
			m.rt.Logf("anaf-docsmart process failed file=%s reason=%v", filepath.Base(path), err)
			if archiveErr := archiveFile(path, meta.FailedDir); archiveErr != nil {
				m.rt.Logf("anaf-docsmart archive failed file=%s target=%s reason=%v", filepath.Base(path), meta.FailedDir, archiveErr)
			}
			continue
		}
		if archiveErr := archiveFile(path, meta.ProcessedDir); archiveErr != nil {
			m.rt.Logf("anaf-docsmart archive failed file=%s target=%s reason=%v", filepath.Base(path), meta.ProcessedDir, archiveErr)
		}
	}
}

func (m *Module) processFile(xmlPath string) error {
	if _, err := parseXMLFileDetailed(xmlPath); err != nil {
		return err
	}
	settings := m.rt.ModuleSettings(m.ID())
	templatePath, err := m.resolveTemplatePath(settings)
	if err != nil {
		return err
	}
	workDir := m.rt.ResolvePath(firstNonEmpty(asString(settings["work_dir"]), "./work"))
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create work_dir: %w", err)
	}
	meta := m.fileTransport()
	baseName := strings.TrimSuffix(filepath.Base(xmlPath), filepath.Ext(xmlPath))
	outputPath := uniquePath(filepath.Join(meta.ExportDir, baseName+".filled.pdf"))
	runDir := filepath.Join(workDir, baseName+"-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run_dir: %w", err)
	}
	defer func() {
		if !boolSetting(settings, "keep_work", false) {
			_ = os.RemoveAll(runDir)
		}
	}()

	workPDF := filepath.Join(runDir, "template.pdf")
	workXML := filepath.Join(runDir, "import.xml")
	if err := copyFile(templatePath, workPDF); err != nil {
		return fmt.Errorf("copy template: %w", err)
	}
	if err := copyFile(xmlPath, workXML); err != nil {
		return fmt.Errorf("copy xml: %w", err)
	}

	acrobatApp := firstNonEmpty(asString(settings["acrobat_app"]), "Adobe Acrobat")
	if err := importXFAToPDF(acrobatApp, workPDF, workXML, outputPath); err != nil {
		return err
	}
	m.rt.Logf("anaf-docsmart generated xml=%s template=%s output=%s", xmlPath, templatePath, outputPath)
	return nil
}

func (m *Module) resolveTemplatePath(settings map[string]interface{}) (string, error) {
	explicit := strings.TrimSpace(asString(settings["template_file"]))
	if explicit != "" {
		resolved := m.rt.ResolvePath(explicit)
		if _, err := os.Stat(resolved); err != nil {
			return "", fmt.Errorf("template_file not found: %s", resolved)
		}
		return resolved, nil
	}
	templatesDir := m.rt.ResolvePath(firstNonEmpty(asString(settings["templates_dir"]), "./templates"))
	pattern := firstNonEmpty(asString(settings["template_pattern"]), "*.pdf")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return "", fmt.Errorf("create templates_dir: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(templatesDir, pattern))
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return "", fmt.Errorf("no PDF template found in %s matching %s", templatesDir, pattern)
	}
	return files[0], nil
}

func (m *Module) fileTransport() fileTransportMeta {
	settings := m.rt.ModuleSettings("transport-file")
	return fileTransportMeta{
		ImportDir:    m.rt.ResolvePath(firstNonEmpty(asString(settings["import_dir"]), "./inbox")),
		ProcessedDir: m.rt.ResolvePath(firstNonEmpty(asString(settings["processed_dir"]), "./processed")),
		FailedDir:    m.rt.ResolvePath(firstNonEmpty(asString(settings["failed_dir"]), "./failed")),
		ExportDir:    m.rt.ResolvePath(firstNonEmpty(asString(settings["export_dir"]), "./outbox")),
		Pattern:      firstNonEmpty(asString(settings["pattern"]), "*.xml"),
	}
}

func parseXMLFile(path, location, status string) map[string]interface{} {
	item := map[string]interface{}{
		"id":               path,
		"file_name":        filepath.Base(path),
		"path":             path,
		"location":         location,
		"status":           status,
		"year":             "",
		"month":            "",
		"document_date":    "",
		"institution_name": "",
		"control_sum":      "",
		"accounts_count":   0,
		"debit_total":      "0.00",
		"credit_total":     "0.00",
		"top_accounts":     []map[string]interface{}{},
	}
	doc, err := parseXMLFileDetailed(path)
	if err != nil {
		item["error"] = err.Error()
		return item
	}
	item["year"] = doc.Year
	item["month"] = doc.Month
	item["document_date"] = doc.DocumentDate
	item["institution_name"] = doc.InstitutionName
	item["control_sum"] = doc.ControlSum
	item["accounts_count"] = len(doc.Accounts)
	item["debit_total"] = formatFloat(sumAccounts(doc.Accounts, func(a account) string { return a.Debit }))
	item["credit_total"] = formatFloat(sumAccounts(doc.Accounts, func(a account) string { return a.Credit }))

	top := make([]map[string]interface{}, 0, 5)
	for idx, acc := range doc.Accounts {
		if idx >= 5 {
			break
		}
		top = append(top, map[string]interface{}{
			"symbol":      acc.Symbol,
			"source_code": acc.SourceCode,
			"debit":       acc.Debit,
			"credit":      acc.Credit,
		})
	}
	item["top_accounts"] = top
	return item
}

func parseXMLFileDetailed(path string) (declaration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return declaration{}, fmt.Errorf("read xml: %w", err)
	}
	var doc declaration
	if err := xml.Unmarshal(data, &doc); err != nil {
		return declaration{}, fmt.Errorf("parse xml: %w", err)
	}
	return doc, nil
}

func sumAccounts(items []account, pick func(account) string) float64 {
	total := 0.0
	for _, item := range items {
		value := strings.TrimSpace(strings.ReplaceAll(pick(item), ",", "."))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			total += parsed
		}
	}
	return total
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func intSetting(settings map[string]interface{}, key string, fallback int) int {
	value := strings.TrimSpace(asString(settings[key]))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolSetting(settings map[string]interface{}, key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(asString(settings[key])))
	switch value {
	case "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return fallback
	}
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	dir := filepath.Dir(path)
	for idx := 1; ; idx++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%02d%s", name, idx, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func archiveFile(sourcePath, targetDir string) error {
	targetPath := uniquePath(filepath.Join(targetDir, filepath.Base(sourcePath)))
	if err := os.Rename(sourcePath, targetPath); err == nil {
		return nil
	}
	if err := copyFile(sourcePath, targetPath); err != nil {
		return err
	}
	return os.Remove(sourcePath)
}

func copyFile(sourcePath, targetPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, data, 0o644)
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
