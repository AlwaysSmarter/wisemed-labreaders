package barcodeprinter

import (
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	wsmod "wisemed-labreaders/readersv3/modules/ws"
)

//go:embed ui/*
var uiAssets embed.FS

type Module struct {
	rt module.Runtime

	mu       sync.RWMutex
	settings map[string]string
	db       *sql.DB
}

type localHTTPControl interface {
	ApplyRuntimeSettings(addr, lang string, tls bool)
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "barcode-printer" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.settings = readSettings(rt.ModuleSettings(m.ID()))
	m.settings["local_http_address"] = firstNonEmpty(valueAsString(rt.ModuleSettings("local-http")["address"]), "127.0.0.1:18080")
	m.settings["local_http_language"] = firstNonEmpty(valueAsString(rt.ModuleSettings("local-http")["language"]), "ro")
	m.settings["local_http_tls"] = firstNonEmpty(valueAsString(rt.ModuleSettings("local-http")["tls"]), "false")
	m.settings["local_http_cors_allowed_origins"] = firstNonEmpty(valueAsString(rt.ModuleSettings("local-http")["cors_allowed_origins"]), "https://ldse.wisemed.eu")
	if err := m.openDB(); err != nil {
		return err
	}

	m.rt.Handle("/barcode/settings", m.withCORS(http.HandlerFunc(m.handleSettingsPage)))
	m.rt.Handle("/barcode/app.js", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/app.js", "application/javascript; charset=utf-8"))))
	m.rt.Handle("/barcode/styles.css", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/styles.css", "text/css; charset=utf-8"))))
	m.rt.Handle("/barcode/print", m.withCORS(http.HandlerFunc(m.handleLegacyPrint)))
	m.rt.Handle("/api/barcode/printers", m.withCORS(http.HandlerFunc(m.handlePrinters)))
	m.rt.Handle("/api/barcode/print", m.withCORS(http.HandlerFunc(m.handlePrintJSON)))
	m.rt.Handle("/api/barcode/test-print", m.withCORS(http.HandlerFunc(m.handleTestPrint)))
	m.rt.Handle("/api/barcode/settings", m.withCORS(http.HandlerFunc(m.handleSettingsAPI)))
	m.rt.Handle("/api/barcode/jobs", m.withCORS(http.HandlerFunc(m.handleJobs)))
	m.rt.Handle("/api/barcode/stats/daily", m.withCORS(http.HandlerFunc(m.handleDailyStats)))

	if svc, ok := rt.Service("ws-action-dispatcher"); ok {
		if d, ok := svc.(*wsmod.ActionDispatcher); ok {
			d.Register(m)
		}
	}
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	<-ctx.Done()
	if m.db != nil {
		_ = m.db.Close()
	}
	return nil
}

func (m *Module) HandleWSAction(action string, payload map[string]interface{}) (map[string]interface{}, bool, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "barcode.print", "barcode/print", "print_barcode":
		params := interfaceMapToStringMap(payload)
		clientIP := strings.TrimSpace(params["request_ip"])
		if clientIP == "" {
			clientIP = "ws"
		}
		err := m.printWithParams(params, clientIP)
		if err != nil {
			return nil, true, err
		}
		return map[string]interface{}{"printed": true}, true, nil
	case "barcode.printers", "barcode/list_printers":
		return map[string]interface{}{"printers": listPrinters()}, true, nil
	default:
		return nil, false, nil
	}
}

func (m *Module) handleSettingsPage(w http.ResponseWriter, _ *http.Request) {
	m.handleStaticAsset("ui/index.html", "text/html; charset=utf-8")(w, nil)
}

func (m *Module) handleStaticAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		blob, err := fs.ReadFile(uiAssets, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(blob)
	}
}

func (m *Module) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.applyCORSHeaders(w, r)
		if r != nil && r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (m *Module) currentCORSAllowedOrigins() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return firstNonEmpty(m.settings["local_http_cors_allowed_origins"], "https://ldse.wisemed.eu")
}

func parseAllowedOrigins(raw string) []string {
	raw = strings.NewReplacer("\r", "\n", ";", "\n", ",", "\n").Replace(raw)
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func (m *Module) originAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, item := range parseAllowedOrigins(m.currentCORSAllowedOrigins()) {
		if item == "*" || strings.EqualFold(item, origin) {
			return true
		}
	}
	return false
}

func (m *Module) applyCORSHeaders(w http.ResponseWriter, r *http.Request) {
	if w == nil || r == nil {
		return
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || !m.originAllowed(origin) {
		return
	}
	headers := w.Header()
	headers.Set("Vary", "Origin")
	headers.Add("Vary", "Access-Control-Request-Method")
	headers.Add("Vary", "Access-Control-Request-Headers")
	headers.Set("Access-Control-Allow-Origin", origin)
	headers.Set("Access-Control-Allow-Credentials", "true")
	headers.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	if requested := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); requested != "" {
		headers.Set("Access-Control-Allow-Headers", requested)
	} else {
		headers.Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin, X-Requested-With")
	}
}

func (m *Module) handleLegacyPrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	params := map[string]string{}
	_ = r.ParseForm()
	for k, vals := range r.Form {
		if len(vals) > 0 {
			params[k] = vals[len(vals)-1]
		}
	}
	if err := m.printWithParams(params, extractIP(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (m *Module) handlePrintJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	payload := map[string]interface{}{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid json"})
		return
	}
	params := interfaceMapToStringMap(payload)
	if err := m.printWithParams(params, extractIP(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (m *Module) handleTestPrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	m.mu.RLock()
	params := mergeMaps(m.settings, map[string]string{
		"bc":   fmt.Sprintf("TEST-%d", time.Now().Unix()%100000),
		"pn":   "TEST PRINT",
		"tc":   "T",
		"no":   "1",
		"code": "",
	})
	m.mu.RUnlock()
	if err := m.printWithParams(params, extractIP(r)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (m *Module) handlePrinters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "printers": listPrinters()})
}

func (m *Module) handleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "settings": m.settingsSnapshot()})
	case http.MethodPut:
		payload := map[string]interface{}{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "invalid json"})
			return
		}
		next := interfaceMapToStringMap(payload)
		if len(next) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "empty settings"})
			return
		}
		nextAddr := strings.TrimSpace(firstNonEmpty(next["local_http_address"], m.settings["local_http_address"], "127.0.0.1:18080"))
		nextLang := strings.TrimSpace(firstNonEmpty(next["local_http_language"], m.settings["local_http_language"], "ro"))
		nextTLS := strings.TrimSpace(firstNonEmpty(next["local_http_tls"], m.settings["local_http_tls"], "false"))
		m.mu.Lock()
		for k, v := range next {
			m.settings[k] = v
		}
		m.mu.Unlock()
		if err := m.persistSettingsToConfig(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		if svc, ok := m.rt.Service("local-http-control"); ok {
			if ctl, ok := svc.(localHTTPControl); ok {
				ctl.ApplyRuntimeSettings(nextAddr, nextLang, strings.EqualFold(nextTLS, "true"))
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "settings": m.settingsSnapshot()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
	}
}

func (m *Module) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
	items, err := m.listPrintJobs(dateFrom, dateTo, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "jobs": items})
}

func (m *Module) handleDailyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
	items, err := m.dailyStats(dateFrom, dateTo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "daily": items})
}

func (m *Module) printWithParams(params map[string]string, clientIP string) error {
	m.mu.RLock()
	resolved := mergeMaps(m.settings, params)
	m.mu.RUnlock()
	bcp, err := newZPLPrinterFromParams(resolved)
	if err != nil {
		_ = m.insertPrintLog(clientIP, resolved, 0, "fail", err.Error())
		return err
	}
	count := 1
	if raw := strings.TrimSpace(firstNonEmpty(resolved["no"], resolved["copies"])); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			count = n
		}
	}
	zpl := bcp.RenderZPL()
	m.logPrintPayload(resolved["othercfg_sel_printer"], count, zpl)
	for i := 0; i < count; i++ {
		if err := sendToPrinter(resolved["othercfg_sel_printer"], []byte(zpl)); err != nil {
			_ = m.insertPrintLog(clientIP, resolved, count, "fail", err.Error())
			return err
		}
	}
	_ = m.insertPrintLog(clientIP, resolved, count, "ok", "")
	return nil
}

func (m *Module) logPrintPayload(printerName string, count int, zpl string) {
	printerName = strings.TrimSpace(printerName)
	if printerName == "" {
		printerName = "default"
	}
	payload := []byte(zpl)
	m.rt.Logf("barcode-printer: sending printer=%q copies=%d bytes=%d payload_b64=%s", printerName, count, len(payload), base64.StdEncoding.EncodeToString(payload))
	m.rt.Logf("barcode-printer: payload begin\n%sbarcode-printer: payload end", ensureTrailingNewline(zpl))
}

func (m *Module) openDB() error {
	dbPath := strings.TrimSpace(m.settings["log_db_path"])
	if dbPath == "" {
		dbPath = "./barcodeprinter-log.db"
	}
	resolved := m.rt.ResolvePath(dbPath)
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", resolved)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS barcode_print_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  file_id TEXT NOT NULL,
  patient_name TEXT NOT NULL,
  bc_type TEXT NOT NULL,
  labels_count INTEGER NOT NULL,
  printer_name TEXT NOT NULL,
  status TEXT NOT NULL,
  error_text TEXT NOT NULL
)`); err != nil {
		_ = db.Close()
		return err
	}
	m.db = db
	return nil
}

func (m *Module) insertPrintLog(clientIP string, params map[string]string, count int, status, errText string) error {
	if m.db == nil {
		return nil
	}
	fileID := strings.TrimSpace(firstNonEmpty(params["fileid"], params["bc"], params["code"]))
	name := strings.TrimSpace(firstNonEmpty(params["name"], params["pn"]))
	bctype := strings.TrimSpace(firstNonEmpty(params["bc_bctype"], params["othercfg_printer_barcode"], "B3"))
	printer := strings.TrimSpace(firstNonEmpty(params["bc_selprinter"], params["othercfg_sel_printer"]))
	_, err := m.db.Exec(`
INSERT INTO barcode_print_jobs(created_at, client_ip, file_id, patient_name, bc_type, labels_count, printer_name, status, error_text)
VALUES(?,?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339),
		firstNonEmpty(clientIP, "unknown"),
		fileID,
		name,
		bctype,
		count,
		printer,
		status,
		errText,
	)
	return err
}

func (m *Module) listPrintJobs(dateFrom, dateTo string, limit int) ([]map[string]interface{}, error) {
	if m.db == nil {
		return []map[string]interface{}{}, nil
	}
	where := []string{"1=1"}
	args := []interface{}{}
	if dateFrom != "" {
		where = append(where, "date(created_at) >= date(?)")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "date(created_at) <= date(?)")
		args = append(args, dateTo)
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
SELECT created_at, client_ip, file_id, patient_name, bc_type, labels_count, printer_name, status, error_text
FROM barcode_print_jobs
WHERE %s
ORDER BY id DESC
LIMIT ?`, strings.Join(where, " AND "))
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var createdAt, clientIP, fileID, patientName, bcType, printerName, status, errorText string
		var labelsCount int
		if err := rows.Scan(&createdAt, &clientIP, &fileID, &patientName, &bcType, &labelsCount, &printerName, &status, &errorText); err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{
			"created_at":   createdAt,
			"client_ip":    clientIP,
			"file_id":      fileID,
			"name":         patientName,
			"bc_type":      bcType,
			"labels_count": labelsCount,
			"printer_name": printerName,
			"status":       status,
			"error":        errorText,
		})
	}
	return items, nil
}

func (m *Module) dailyStats(dateFrom, dateTo string) ([]map[string]interface{}, error) {
	if m.db == nil {
		return []map[string]interface{}{}, nil
	}
	where := []string{"1=1"}
	args := []interface{}{}
	if dateFrom != "" {
		where = append(where, "date(created_at) >= date(?)")
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where = append(where, "date(created_at) <= date(?)")
		args = append(args, dateTo)
	}
	q := fmt.Sprintf(`
SELECT date(created_at) as day,
       count(*) as prints,
       sum(labels_count) as labels,
       sum(CASE WHEN status='ok' THEN 1 ELSE 0 END) as ok_count,
       sum(CASE WHEN status='fail' THEN 1 ELSE 0 END) as fail_count
FROM barcode_print_jobs
WHERE %s
GROUP BY day
ORDER BY day DESC`, strings.Join(where, " AND "))
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var day string
		var prints, labels, okCount, failCount int
		if err := rows.Scan(&day, &prints, &labels, &okCount, &failCount); err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{
			"day":    day,
			"prints": prints,
			"labels": labels,
			"ok":     okCount,
			"fail":   failCount,
		})
	}
	return items, nil
}

func (m *Module) persistSettingsToConfig() error {
	path := m.rt.ConfigPath()
	if strings.TrimSpace(path) == "" {
		return errors.New("config path unavailable")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cfg := map[string]interface{}{}
	if err := yaml.Unmarshal(blob, &cfg); err != nil {
		return err
	}
	modules, _ := cfg["modules"].(map[string]interface{})
	if modules == nil {
		modules = map[string]interface{}{}
		cfg["modules"] = modules
	}
	section := map[string]interface{}{}
	for k, v := range m.settings {
		if k == "local_http_address" || k == "local_http_language" || k == "local_http_tls" || k == "local_http_cors_allowed_origins" {
			continue
		}
		section[k] = v
	}
	modules[m.ID()] = section
	localHTTP, _ := modules["local-http"].(map[string]interface{})
	if localHTTP == nil {
		localHTTP = map[string]interface{}{}
		modules["local-http"] = localHTTP
	}
	localHTTP["address"] = firstNonEmpty(m.settings["local_http_address"], "127.0.0.1:18080")
	localHTTP["language"] = firstNonEmpty(m.settings["local_http_language"], "ro")
	localHTTP["tls"] = strings.EqualFold(firstNonEmpty(m.settings["local_http_tls"], "false"), "true")
	localHTTP["cors_allowed_origins"] = firstNonEmpty(m.settings["local_http_cors_allowed_origins"], "https://ldse.wisemed.eu")
	cfg["local_http"] = map[string]interface{}{
		"address":              localHTTP["address"],
		"language":             localHTTP["language"],
		"enabled":              firstNonEmpty(valueAsString(localHTTP["enabled"]), "true") == "true",
		"tls":                  firstNonEmpty(valueAsString(localHTTP["tls"]), "false") == "true",
		"cors_allowed_origins": localHTTP["cors_allowed_origins"],
	}
	updated, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func (m *Module) settingsSnapshot() map[string]string {
	m.mu.RLock()
	settings := mergeMaps(m.settings, map[string]string{
		"local_http_address":              firstNonEmpty(valueAsString(m.rt.ModuleSettings("local-http")["address"]), m.settings["local_http_address"], "127.0.0.1:18080"),
		"local_http_language":             firstNonEmpty(valueAsString(m.rt.ModuleSettings("local-http")["language"]), m.settings["local_http_language"], "ro"),
		"local_http_tls":                  firstNonEmpty(valueAsString(m.rt.ModuleSettings("local-http")["tls"]), m.settings["local_http_tls"], "false"),
		"local_http_cors_allowed_origins": firstNonEmpty(valueAsString(m.rt.ModuleSettings("local-http")["cors_allowed_origins"]), m.settings["local_http_cors_allowed_origins"], "https://ldse.wisemed.eu"),
	})
	m.mu.RUnlock()

	cfg, err := config.Load(m.rt.ConfigPath())
	if err != nil || cfg == nil {
		return settings
	}

	settings = mergeMaps(settings, readSettings(cfg.ModuleSettings(m.ID())))
	settings["local_http_address"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.Address), valueAsString(cfg.ModuleSettings("local-http")["address"]), settings["local_http_address"], "127.0.0.1:18080")
	settings["local_http_language"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.Language), valueAsString(cfg.ModuleSettings("local-http")["language"]), settings["local_http_language"], "ro")
	settings["local_http_tls"] = firstNonEmpty(valueAsString(cfg.LocalHTTP.TLS), valueAsString(cfg.ModuleSettings("local-http")["tls"]), settings["local_http_tls"], "false")
	settings["local_http_cors_allowed_origins"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.CORS), valueAsString(cfg.ModuleSettings("local-http")["cors_allowed_origins"]), settings["local_http_cors_allowed_origins"], "https://ldse.wisemed.eu")
	return settings
}

func readSettings(raw map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case int:
			out[k] = strconv.Itoa(t)
		case int64:
			out[k] = strconv.FormatInt(t, 10)
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			if t {
				out[k] = "1"
			} else {
				out[k] = "0"
			}
		}
	}
	return out
}

func interfaceMapToStringMap(payload map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range payload {
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case int:
			out[k] = strconv.Itoa(t)
		case bool:
			if t {
				out[k] = "1"
			} else {
				out[k] = "0"
			}
		}
	}
	return out
}

func mergeMaps(base, over map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}

func valueAsString(value interface{}) string {
	switch t := value.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func extractIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
