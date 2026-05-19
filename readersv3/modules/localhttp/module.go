package localhttp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
	"wisemed-labreaders/readersv3/shared/appmeta"
	"wisemed-labreaders/readersv3/shared/appupdates"
)

//go:embed ui/*
var uiAssets embed.FS

const sessionCookieName = "wmr_local_session"

type Module struct {
	rt module.Runtime

	mu         sync.RWMutex
	server     *http.Server
	address    string
	tlsEnabled bool
	cors       string
	restartCh  chan localHTTPRestart
	sessions   map[string]session
	language   string
	repeatMode string
	analytes   []coremodel.Analyte
	qcTargets  []coremodel.QCTarget
	updateInfo map[string]interface{}
}

type localHTTPRestart struct {
	address string
	tls     bool
}

type session struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	MedicalUnitID int       `json:"medical_unit_id"`
	UserType      int       `json:"user_type"`
	FirstName     string    `json:"first_name,omitempty"`
	LastName      string    `json:"last_name,omitempty"`
	UserEmail     string    `json:"user_email,omitempty"`
	UserPicture   string    `json:"user_picture,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type analyteStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
	GetAnalyteByID(id int64) (coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
	DeleteAnalyte(id int64) error
}

type qcTargetStore interface {
	ListQCTargets() ([]coremodel.QCTarget, error)
	GetQCTarget(id int64) (coremodel.QCTarget, error)
	SaveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error)
	DeleteQCTarget(id int64) error
}

type qcRecordStore interface {
	ListQCRecordBundles(runDate string) ([]coremodel.QCRecordBundle, error)
	SaveManualQCRecord(runDate string, analysis coremodel.QCAnalysis, actor string, enteredAt time.Time) error
}

type qcRecordRangeStore interface {
	ListQCRecordBundlesRange(dateFrom, dateTo string) ([]coremodel.QCRecordBundle, error)
}

type qcMetricsStore interface {
	QCPerformance(analyteTag, controlLevel, lotNo, dateFrom, dateTo string, limit int) (map[string]interface{}, error)
}

type dashboardStore interface {
	DashboardSnapshot(limit int) (map[string]interface{}, error)
}

type logStore interface {
	ListLogs(limit int) ([]coremodel.EventLog, error)
}

type orderStore interface {
	ListOrderBundles(roundNo int, orderDate string) ([]coremodel.OrderBundle, error)
	ListRoundNumbers(orderDate string) ([]int, error)
	CreateNextRound(orderDate string) (int, error)
	SetDefaultResult(orderAnalysisID, resultID int64, repeatMode string) error
}

type dailyDetailService interface {
	Definitions() []coremodel.DailyDetailDefinition
	DynamicDefinitionsEnabled() bool
}

type dailyDetailStore interface {
	ListDailyDetailDefinitions() ([]coremodel.DailyDetailDefinition, error)
	GetDailyDetailDefinition(id int64) (coremodel.DailyDetailDefinition, error)
	SaveDailyDetailDefinition(item coremodel.DailyDetailDefinition) (coremodel.DailyDetailDefinition, error)
	DeleteDailyDetailDefinition(id int64) error
	ListDailyDetailValues(scopeDate string, roundNo int) ([]coremodel.DailyDetailValue, error)
	SaveDailyDetailValue(item coremodel.DailyDetailValue) (coremodel.DailyDetailValue, error)
}

type fileImporter interface {
	ImportFileNow(path, orderDate string) (map[string]interface{}, error)
}

type wiseMedAPIService interface {
	Settings() map[string]string
	PublicSettings() map[string]string
	IsConfigured() bool
	SetupComplete() bool
	HasEquipmentID() bool
	SaveSetup(map[string]string) (map[string]string, error)
	Bootstrap() (map[string]interface{}, error)
	Login(wisemedapi.LoginRequest) (wisemedapi.LoginResponse, error)
	EnsureEquipmentOnline(reader map[string]interface{}) (map[string]interface{}, error)
}

type wiseMedWSStatusService interface {
	Connected() bool
}

type resultSyncService interface {
	Status() map[string]interface{}
	SettingsPayload() map[string]interface{}
	RunNow() (map[string]interface{}, error)
	Reset()
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "local-http" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.sessions = map[string]session{}
	m.restartCh = make(chan localHTTPRestart, 1)
	m.language = "ro"
	if lang, _ := rt.ModuleSettings(m.ID())["language"].(string); strings.TrimSpace(lang) != "" {
		m.language = strings.TrimSpace(lang)
	}
	m.cors = firstNonEmpty(asString(rt.ModuleSettings(m.ID())["cors_allowed_origins"]), "https://ldse.wisemed.eu")
	m.repeatMode = normalizeRepeatMode(asString(rt.ModuleSettings(m.ID())["repeat_mode"]))
	m.rt.RegisterService("local-http-control", m)
	m.rt.AddMenu(module.MenuEntry{ID: "overview", Group: "core", Label: "Acasa", Path: "/", Order: 10})
	m.rt.Handle("/", m.withNoCache(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/settings", m.withNoCache(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/orders", m.withNoCache(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/qc", m.withNoCache(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/app.js", m.withNoCache(http.HandlerFunc(m.handleStaticAsset("ui/app.js", "application/javascript; charset=utf-8"))))
	m.rt.Handle("/styles.css", m.withNoCache(http.HandlerFunc(m.handleStaticAsset("ui/styles.css", "text/css; charset=utf-8"))))
	m.rt.Handle("/api/session", m.withNoCache(http.HandlerFunc(m.handleSessionStatus)))
	m.rt.Handle("/api/preferences", m.withNoCache(http.HandlerFunc(m.handlePreferences)))
	m.rt.Handle("/api/preferences/language", m.withNoCache(http.HandlerFunc(m.handleLanguage)))
	m.rt.Handle("/api/reader-settings", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleReaderSettings))))
	m.rt.Handle("/api/wisemed/setup", m.withNoCache(http.HandlerFunc(m.handleWiseMEDSetup)))
	m.rt.Handle("/api/wisemed/bootstrap", m.withNoCache(http.HandlerFunc(m.handleWiseMEDBootstrap)))
	m.rt.Handle("/api/session/login", m.withNoCache(http.HandlerFunc(m.handleLogin)))
	m.rt.Handle("/api/session/logout", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleLogout))))
	m.rt.Handle("/api/app-update/status", m.withNoCache(http.HandlerFunc(m.handleAppUpdateStatus)))
	m.rt.Handle("/api/app-update/settings", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleAppUpdateSettings))))
	m.rt.Handle("/api/status", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleStatus))))
	m.rt.Handle("/api/dashboard", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleDashboard))))
	m.rt.Handle("/api/logs", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleLogs))))
	m.rt.Handle("/api/analytes", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleAnalytes))))
	m.rt.Handle("/api/analytes/", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleAnalyteByID))))
	m.rt.Handle("/api/orders", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleOrders))))
	m.rt.Handle("/api/orders/worklist", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleOrdersWorklist))))
	m.rt.Handle("/api/orders/rounds", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleOrderRounds))))
	m.rt.Handle("/api/orders/import", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleOrdersImport))))
	m.rt.Handle("/api/orders/export", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleOrdersExport))))
	m.rt.Handle("/api/daily-details/definitions", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleDailyDetailDefinitions))))
	m.rt.Handle("/api/daily-details/definitions/", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleDailyDetailDefinitionByID))))
	m.rt.Handle("/api/daily-details", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleDailyDetails))))
	m.rt.Handle("/api/qc-records", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleQCRecords))))
	m.rt.Handle("/api/qc-targets", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleQCTargets))))
	m.rt.Handle("/api/qc-targets/", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleQCTargetByID))))
	m.rt.Handle("/api/qc/metrics", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleQCMetrics))))
	m.rt.Handle("/api/results/default", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleDefaultResult))))
	m.rt.Handle("/api/result-sync/status", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleResultSyncStatus))))
	m.rt.Handle("/api/result-sync/settings", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleResultSyncSettings))))
	m.rt.Handle("/api/result-sync/run", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleResultSyncRun))))
	m.rt.Handle("/api/result-sync/reset", m.withNoCache(m.requireSession(http.HandlerFunc(m.handleResultSyncReset))))
	return nil
}

func (m *Module) ApplyRuntimeSettings(addr, lang string, tls bool) {
	addr = normalizeLocalHTTPAddress(addr)
	lang = strings.TrimSpace(lang)
	currentAddr := m.currentAddress()
	currentTLS := m.currentTLSEnabled()
	m.mu.Lock()
	if lang != "" {
		m.language = lang
	}
	m.mu.Unlock()
	if addr != currentAddr || tls != currentTLS {
		go m.requestRestart(addr, tls)
	}
}

func (m *Module) Start(ctx context.Context) error {
	cfg := m.rt.ModuleSettings(m.ID())
	if enabled, ok := cfg["enabled"].(bool); ok && !enabled {
		<-ctx.Done()
		return nil
	}
	addr, _ := cfg["address"].(string)
	addr = normalizeLocalHTTPAddress(addr)
	useTLS := false
	if raw, ok := cfg["tls"]; ok {
		switch typed := raw.(type) {
		case bool:
			useTLS = typed
		case string:
			useTLS = boolString(typed)
		}
	}
	m.mu.Lock()
	m.address = addr
	m.tlsEnabled = useTLS
	m.mu.Unlock()
	for {
		server := &http.Server{
			Addr:              addr,
			Handler:           m.rt.Mux(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		m.mu.Lock()
		m.server = server
		m.address = addr
		m.tlsEnabled = useTLS
		m.mu.Unlock()
		errCh := make(chan error, 1)
		protocol := "http"
		certFile := ""
		keyFile := ""
		if useTLS {
			var err error
			certFile, keyFile, err = ensureLocalHTTPSMaterial(m.rt.ConfigDir(), addr)
			if err != nil {
				return err
			}
			protocol = "https"
		}
		go func(srv *http.Server, tlsEnabled bool, certPath, keyPath string) {
			if tlsEnabled {
				errCh <- srv.ListenAndServeTLS(certPath, keyPath)
				return
			}
			errCh <- srv.ListenAndServe()
		}(server, useTLS, certFile, keyFile)
		m.rt.Logf("local http listening on %s://%s", protocol, addr)
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := server.Shutdown(shutdownCtx)
			cancel()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			if listenErr := <-errCh; listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
				return listenErr
			}
			return nil
		case next := <-m.restartCh:
			nextAddr := normalizeLocalHTTPAddress(next.address)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := server.Shutdown(shutdownCtx)
			cancel()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			if listenErr := <-errCh; listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
				return listenErr
			}
			addr = nextAddr
			useTLS = next.tls
			if useTLS {
				m.rt.Logf("local http rebinding to https://%s", addr)
			} else {
				m.rt.Logf("local http rebinding to http://%s", addr)
			}
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		}
	}
}

func (m *Module) withNoCache(next http.Handler) http.Handler {
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
	return firstNonEmpty(m.cors, "https://ldse.wisemed.eu")
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

func normalizeLocalHTTPAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:18080"
	}
	return addr
}

func (m *Module) currentAddress() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeLocalHTTPAddress(m.address)
}

func (m *Module) currentTLSEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsEnabled
}

func (m *Module) requestRestart(addr string, tls bool) {
	req := localHTTPRestart{address: normalizeLocalHTTPAddress(addr), tls: tls}
	select {
	case m.restartCh <- req:
	default:
		select {
		case <-m.restartCh:
		default:
		}
		m.restartCh <- req
	}
}

func (m *Module) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := m.currentSession(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Module) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/help" {
		http.Redirect(w, r, "/help/", http.StatusTemporaryRedirect)
		return
	}
	switch r.URL.Path {
	case "/", "/daily-details", "/settings", "/settings/analytes", "/settings/qc", "/settings/daily-details", "/orders", "/qc":
	default:
		http.NotFound(w, r)
		return
	}
	m.handleStaticAsset("ui/index.html", "text/html; charset=utf-8")(w, r)
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

func (m *Module) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	sess, ok := m.currentSession(r)
	wisemedState := m.wiseMEDState()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"authenticated":  ok,
		"setup_required": !wisemedState["setup_complete"].(bool),
		"session":        sess,
		"preferences": map[string]interface{}{
			"language": m.currentLanguage(),
		},
		"permissions": map[string]interface{}{
			"can_view_logs": ok,
		},
		"reader":     m.readerPayload(),
		"wisemed":    wisemedState,
		"app_update": m.appUpdateSettingsPayload(),
	})
}

func (m *Module) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"preferences": map[string]interface{}{
			"language": m.currentLanguage(),
		},
	})
}

func (m *Module) handleLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	lang := strings.ToLower(strings.TrimSpace(req.Language))
	if lang != "ro" && lang != "en" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "unsupported language"})
		return
	}
	m.mu.Lock()
	m.language = lang
	m.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"preferences": map[string]interface{}{
			"language": m.currentLanguage(),
		},
	})
}

func (m *Module) handleReaderSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.readerSettingsPayload()})
	case http.MethodPut:
		var req struct {
			RepeatMode                string `json:"repeat_mode"`
			ReaderID                  string `json:"reader_id"`
			ReaderLabel               string `json:"reader_label"`
			AnalyzerName              string `json:"analyzer_name"`
			AnalyzerCode              string `json:"analyzer_code"`
			DBName                    string `json:"db_name"`
			LocalHTTPAddress          string `json:"local_http_address"`
			LocalHTTPLang             string `json:"local_http_language"`
			LocalHTTPTLS              string `json:"local_http_tls"`
			LocalHTTPCORSAllowed      string `json:"local_http_cors_allowed_origins"`
			AnalyzerCommType          string `json:"analyzer_comm_type"`
			AnalyzerProtocol          string `json:"analyzer_protocol"`
			SQLitePath                string `json:"sqlite_path"`
			AppUpdatesEnabled         string `json:"app_updates_enabled"`
			AppUpdatesAppID           string `json:"app_updates_app_id"`
			AppUpdatesCurrentVersion  string `json:"app_updates_current_version"`
			AppUpdatesChannel         string `json:"app_updates_channel"`
			AppUpdatesBaseURL         string `json:"app_updates_base_url"`
			AppUpdatesAutoDownload    string `json:"app_updates_auto_download"`
			AppUpdatesDownloadDir     string `json:"app_updates_download_dir"`
			ResultSyncEnabled         string `json:"result_sync_enabled"`
			ResultSyncIntervalMinutes string `json:"result_sync_interval_minutes"`
			ResultSyncSamplePrefixes  string `json:"result_sync_sample_prefixes"`
			ResultSyncSampleSuffixes  string `json:"result_sync_sample_suffixes"`
			ResultSyncSeparators      string `json:"result_sync_separators"`
			ResultSyncQCPrefixes      string `json:"result_sync_qc_prefixes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		mode := normalizeRepeatMode(req.RepeatMode)
		if err := m.persistReaderSettings(map[string]interface{}{
			"reader.id":                               strings.TrimSpace(req.ReaderID),
			"reader.label":                            strings.TrimSpace(req.ReaderLabel),
			"reader.analyzer_name":                    strings.TrimSpace(req.AnalyzerName),
			"reader.analyzer_code":                    strings.TrimSpace(req.AnalyzerCode),
			"reader.db_name":                          strings.TrimSpace(req.DBName),
			"local_http.address":                      strings.TrimSpace(req.LocalHTTPAddress),
			"local_http.language":                     strings.TrimSpace(req.LocalHTTPLang),
			"local_http.tls":                          boolString(req.LocalHTTPTLS),
			"local_http.cors_allowed_origins":         strings.TrimSpace(req.LocalHTTPCORSAllowed),
			"modules.local-http.address":              strings.TrimSpace(req.LocalHTTPAddress),
			"modules.local-http.language":             strings.TrimSpace(req.LocalHTTPLang),
			"modules.local-http.tls":                  boolString(req.LocalHTTPTLS),
			"modules.local-http.cors_allowed_origins": strings.TrimSpace(req.LocalHTTPCORSAllowed),
			"analyzer.comm_type":                      strings.TrimSpace(req.AnalyzerCommType),
			"analyzer.protocol":                       strings.TrimSpace(req.AnalyzerProtocol),
			"modules.local-http.repeat_mode":          mode,
			"modules.storage-sqlite.path":             strings.TrimSpace(req.SQLitePath),
			"modules.app-updates.enabled":             boolString(req.AppUpdatesEnabled),
			"modules.app-updates.app_id":              strings.TrimSpace(req.AppUpdatesAppID),
			"modules.app-updates.channel":             strings.TrimSpace(req.AppUpdatesChannel),
			"modules.app-updates.base_url":            strings.TrimSpace(req.AppUpdatesBaseURL),
			"modules.app-updates.auto_download":       boolString(req.AppUpdatesAutoDownload),
			"modules.app-updates.download_dir":        strings.TrimSpace(req.AppUpdatesDownloadDir),
			"modules.result-sync.enabled":             boolString(req.ResultSyncEnabled),
			"modules.result-sync.interval_minutes":    parseIntString(req.ResultSyncIntervalMinutes, "5"),
			"modules.result-sync.sample_prefixes":     splitCSV(req.ResultSyncSamplePrefixes),
			"modules.result-sync.sample_suffixes":     splitCSV(req.ResultSyncSampleSuffixes),
			"modules.result-sync.separators":          splitCSV(req.ResultSyncSeparators),
			"modules.result-sync.qc_prefixes":         parseQCPrefixSettings(req.ResultSyncQCPrefixes),
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		m.mu.Lock()
		m.repeatMode = mode
		if lang := strings.TrimSpace(req.LocalHTTPLang); lang != "" {
			m.language = lang
		}
		m.cors = firstNonEmpty(strings.TrimSpace(req.LocalHTTPCORSAllowed), "https://ldse.wisemed.eu")
		m.updateInfo = nil
		m.mu.Unlock()
		nextAddr := normalizeLocalHTTPAddress(req.LocalHTTPAddress)
		shouldRestart := nextAddr != m.currentAddress()
		nextTLS := boolString(req.LocalHTTPTLS)
		shouldRestart = shouldRestart || nextTLS != m.currentTLSEnabled()
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.readerSettingsPayload()})
		if shouldRestart {
			go m.requestRestart(nextAddr, nextTLS)
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleResultSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	service := m.resultSyncService()
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "result sync service unavailable"})
		return
	}
	status := service.Status()
	status["ok"] = true
	writeJSON(w, http.StatusOK, status)
}

func (m *Module) handleResultSyncSettings(w http.ResponseWriter, r *http.Request) {
	service := m.resultSyncService()
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "result sync service unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": service.SettingsPayload()})
	case http.MethodPut:
		var req struct {
			Enabled         bool   `json:"enabled"`
			IntervalMinutes int    `json:"interval_minutes"`
			SamplePrefixes  string `json:"sample_prefixes"`
			SampleSuffixes  string `json:"sample_suffixes"`
			Separators      string `json:"separators"`
			QCPrefixes      string `json:"qc_prefixes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		if req.IntervalMinutes <= 0 {
			req.IntervalMinutes = 5
		}
		if err := m.persistReaderSettings(map[string]interface{}{
			"modules.result-sync.enabled":          req.Enabled,
			"modules.result-sync.interval_minutes": req.IntervalMinutes,
			"modules.result-sync.sample_prefixes":  splitCSV(req.SamplePrefixes),
			"modules.result-sync.sample_suffixes":  splitCSV(req.SampleSuffixes),
			"modules.result-sync.separators":       splitCSV(req.Separators),
			"modules.result-sync.qc_prefixes":      parseQCPrefixSettings(req.QCPrefixes),
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleResultSyncRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	service := m.resultSyncService()
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "result sync service unavailable"})
		return
	}
	summary, err := service.RunNow()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "summary": summary})
}

func (m *Module) handleResultSyncReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	service := m.resultSyncService()
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "result sync service unavailable"})
		return
	}
	service.Reset()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (m *Module) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		MedicalUnitID string `json:"medical_unit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "username and password are required"})
		return
	}
	wiseMED := m.wiseMEDAPI()
	if wiseMED == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "wisemed api service unavailable"})
		return
	}
	if !wiseMED.SetupComplete() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "WiseMED setup is incomplete"})
		return
	}
	loginResp, err := wiseMED.Login(wisemedapi.LoginRequest{
		Username:      strings.TrimSpace(req.Username),
		Password:      req.Password,
		MedicalUnitID: strings.TrimSpace(req.MedicalUnitID),
		DeviceID:      m.rt.ReaderID(),
		DeviceName:    m.readerSetting("analyzer_name", m.readerSetting("label", m.rt.ReaderID())),
	})
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if _, err := wiseMED.EnsureEquipmentOnline(m.readerPayload()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	sess, err := m.createSessionFromWiseMED(loginResp, wiseMED.Settings())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	token, err := m.encodeSessionToken(sess)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"session": sess,
	})
}

func (m *Module) handleWiseMEDSetup(w http.ResponseWriter, r *http.Request) {
	wiseMED := m.wiseMEDAPI()
	if wiseMED == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "wisemed api service unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":                   true,
			"configured":           wiseMED.IsConfigured(),
			"setup_complete":       wiseMED.SetupComplete(),
			"equipment_registered": wiseMED.HasEquipmentID(),
			"settings":             wiseMED.PublicSettings(),
		})
	case http.MethodPut:
		payload := map[string]interface{}{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		next := map[string]string{}
		for key, value := range payload {
			next[key] = fmt.Sprint(value)
		}
		settings, err := wiseMED.SaveSetup(next)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":                   true,
			"configured":           wiseMED.IsConfigured(),
			"setup_complete":       wiseMED.SetupComplete(),
			"equipment_registered": wiseMED.HasEquipmentID(),
			"settings":             settings,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleWiseMEDBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	wiseMED := m.wiseMEDAPI()
	if wiseMED == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "wisemed api service unavailable"})
		return
	}
	resp, err := wiseMED.Bootstrap()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	resp["ok"] = true
	writeJSON(w, http.StatusOK, resp)
}

func (m *Module) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		m.mu.Lock()
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (m *Module) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	analytes, _ := m.listAnalytes()
	qcTargets, _ := m.listQCTargets()
	barcodeMode := strings.EqualFold(m.analyzerSetting("protocol", ""), "barcodeprinter") || strings.EqualFold(m.readerSetting("analyzer_code", ""), "barcodeprinter")
	stats := map[string]interface{}{
		"analytes":   len(analytes),
		"orders":     0,
		"results":    0,
		"qc_records": 0,
		"qc_results": 0,
		"qc_targets": len(qcTargets),
		"events":     1,
	}
	if barcodeMode {
		stats["analytes"] = 0
		stats["qc_records"] = 0
		stats["qc_results"] = 0
		stats["qc_targets"] = 0
	}
	wsConnected := false
	if wsStatus := m.wiseMEDWSStatus(); wsStatus != nil {
		wsConnected = wsStatus.Connected()
	}
	analyzerConnected := m.analyzerSetting("comm_type", "file") == "file"
	if barcodeMode {
		analyzerConnected = false
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"reader": m.readerPayload(),
		"communication": map[string]interface{}{
			"type": m.analyzerSetting("comm_type", "file"),
		},
		"layout": map[string]interface{}{
			"kind": "simple_list",
		},
		"stats": stats,
		"connections": map[string]interface{}{
			"wisemed_ws_connected": wsConnected,
			"analyzer_connected":   analyzerConnected,
		},
	})
}

func (m *Module) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	service, ok := m.rt.Service("storage")
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true,
			"today": map[string]interface{}{
				"without_result": 0,
				"with_result":    0,
			},
			"qc_today": map[string]interface{}{
				"results":         0,
				"numeric_results": 0,
				"outside_2sd":     0,
				"outside_3sd":     0,
			},
			"series": []map[string]interface{}{},
		})
		return
	}
	store, ok := service.(dashboardStore)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "dashboard service unavailable"})
		return
	}
	snapshot, err := store.DashboardSnapshot(14)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	snapshot["ok"] = true
	writeJSON(w, http.StatusOK, snapshot)
}

func (m *Module) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit <= 0 {
		limit = 40
	}
	service, ok := m.rt.Service("storage")
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "logs": []map[string]interface{}{}})
		return
	}
	store, ok := service.(logStore)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "log service unavailable"})
		return
	}
	items, err := store.ListLogs(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "logs": items})
}

func (m *Module) handleAnalytes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := m.listAnalytes()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analytes": items})
	case http.MethodPost:
		var item coremodel.Analyte
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		saved, err := m.saveAnalyte(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": saved.ID, "tag": saved.Tag})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleAnalyteByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.URL.Path, "/api/analytes/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := m.getAnalyteByID(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "analyte not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analyte": item})
	case http.MethodPut:
		var item coremodel.Analyte
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		saved, err := m.saveAnalyte(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": saved.ID, "tag": saved.Tag})
	case http.MethodDelete:
		if err := m.deleteAnalyte(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
	if orderDate == "" {
		orderDate = time.Now().Format("2006-01-02")
	}
	roundNo, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("round_no")))
	store := m.orderStore()
	if store == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"order_date": orderDate,
			"round_no":   1,
			"rounds":     []int{1},
			"orders":     []coremodel.OrderBundle{},
		})
		return
	}
	rounds, err := store.ListRoundNumbers(orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if len(rounds) == 0 {
		rounds = []int{1}
	}
	if roundNo <= 0 {
		roundNo = rounds[len(rounds)-1]
	}
	items, err := store.ListOrderBundles(roundNo, orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"order_date": orderDate,
		"round_no":   roundNo,
		"rounds":     rounds,
		"orders":     items,
	})
}

func (m *Module) handleOrderRounds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
		if orderDate == "" {
			orderDate = time.Now().Format("2006-01-02")
		}
		store := m.orderStore()
		rounds := []int{1}
		if store != nil {
			var err error
			rounds, err = store.ListRoundNumbers(orderDate)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			if len(rounds) == 0 {
				rounds = []int{1}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_date": orderDate, "round_no": rounds[len(rounds)-1], "rounds": rounds})
	case http.MethodPost:
		var req struct {
			OrderDate string `json:"order_date"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		orderDate := strings.TrimSpace(req.OrderDate)
		if orderDate == "" {
			orderDate = time.Now().Format("2006-01-02")
		}
		store := m.orderStore()
		if store == nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "storage service unavailable"})
			return
		}
		roundNo, err := store.CreateNextRound(orderDate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		rounds, err := store.ListRoundNumbers(orderDate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_date": orderDate, "round_no": roundNo, "rounds": rounds})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleOrdersImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	importer := m.importer()
	if importer == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "file import backend is not available"})
		return
	}
	orderDate := strings.TrimSpace(r.FormValue("order_date"))
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "missing import file"})
		return
	}
	defer file.Close()
	tmpDir := m.rt.ResolvePath(filepath.Join(".tmp-imports"))
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	tmpPath := filepath.Join(tmpDir, filepath.Base(header.Filename))
	out, err := os.Create(tmpPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	_ = out.Close()
	summary, err := importer.ImportFileNow(tmpPath, orderDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (m *Module) handleOrdersExport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "file export backend is not migrated yet"})
}

func (m *Module) handleOrdersWorklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	if !strings.EqualFold(m.analyzerSetting("protocol", ""), "cary60-uvvis") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "worklist is currently available only for Cary60 UV-VIS"})
		return
	}
	orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
	if orderDate == "" {
		orderDate = time.Now().Format("2006-01-02")
	}
	roundNo, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("round_no")))
	store := m.orderStore()
	if store == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "storage service unavailable"})
		return
	}
	if roundNo <= 0 {
		rounds, _ := store.ListRoundNumbers(orderDate)
		if len(rounds) > 0 {
			roundNo = rounds[len(rounds)-1]
		} else {
			roundNo = 1
		}
	}
	bundles, err := store.ListOrderBundles(roundNo, orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	definitions, _ := m.combinedDailyDetailDefinitions()
	values, _ := m.listDailyDetailValues(orderDate, roundNo)
	analyteIndex := map[string]coremodel.Analyte{}
	analytes, _ := m.listAnalytes()
	for _, analyte := range analytes {
		analyteIndex[strings.TrimSpace(analyte.Tag)] = analyte
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(m.renderCaryWorklistHTML(orderDate, roundNo, bundles, analyteIndex, definitions, values)))
}

func (m *Module) handleDailyDetails(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scopeDate := strings.TrimSpace(r.URL.Query().Get("scope_date"))
		if scopeDate == "" {
			scopeDate = time.Now().Format("2006-01-02")
		}
		roundNo, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("round_no")))
		definitions, err := m.combinedDailyDetailDefinitions()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		values, err := m.listDailyDetailValues(scopeDate, roundNo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "scope_date": scopeDate, "round_no": roundNo, "definitions": definitions, "values": values})
	case http.MethodPut:
		var item coremodel.DailyDetailValue
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		saved, err := m.saveDailyDetailValue(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "value": saved})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleDailyDetailDefinitions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		definitions, err := m.combinedDailyDetailDefinitions()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "definitions": definitions, "dynamic_enabled": m.dailyDetailsDynamicEnabled()})
	case http.MethodPost:
		if !m.dailyDetailsDynamicEnabled() {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "dynamic daily detail definitions are disabled"})
			return
		}
		var item coremodel.DailyDetailDefinition
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.Source = "user"
		saved, err := m.saveDailyDetailDefinition(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "definition": saved})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleDailyDetailDefinitionByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.URL.Path, "/api/daily-details/definitions/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !m.dailyDetailsDynamicEnabled() {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "dynamic daily detail definitions are disabled"})
			return
		}
		var item coremodel.DailyDetailDefinition
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		item.Source = "user"
		saved, err := m.saveDailyDetailDefinition(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "definition": saved})
	case http.MethodDelete:
		if err := m.deleteDailyDetailDefinition(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleQCRecords(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runDate := strings.TrimSpace(r.URL.Query().Get("run_date"))
		dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
		dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
		if runDate == "" {
			runDate = time.Now().Format("2006-01-02")
		}
		if dateFrom == "" && dateTo == "" {
			dateFrom, dateTo = runDate, runDate
		}
		items, err := m.listQCRecordBundles(dateFrom, dateTo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"run_date":   runDate,
			"date_from":  dateFrom,
			"date_to":    dateTo,
			"qc_records": items,
		})
	case http.MethodPost:
		var req struct {
			RunDate          string `json:"run_date"`
			AnalyteTag       string `json:"analyte_tag"`
			AnalyteName      string `json:"analyte_name"`
			ControlLevel     string `json:"control_level"`
			LotNo            string `json:"lot_no"`
			ControlLabel     string `json:"control_label"`
			ResultValue      string `json:"result_value"`
			RawValue         string `json:"raw_value"`
			InterpretedValue string `json:"interpreted_value"`
			Unit             string `json:"unit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		sess, _ := m.currentSession(r)
		actor := firstNonEmpty(
			strings.TrimSpace(strings.TrimSpace(sess.FirstName)+" "+strings.TrimSpace(sess.LastName)),
			strings.TrimSpace(sess.Username),
			strings.TrimSpace(sess.UserEmail),
			"unknown",
		)
		analysis := coremodel.QCAnalysis{
			AnalyteTag:  strings.TrimSpace(req.AnalyteTag),
			AnalyteName: strings.TrimSpace(req.AnalyteName),
			ResultValue: strings.TrimSpace(req.ResultValue),
			RawValue:    strings.TrimSpace(req.RawValue),
			Interpreted: strings.TrimSpace(req.InterpretedValue),
			Unit:        strings.TrimSpace(req.Unit),
			Meta: map[string]interface{}{
				"control_label": strings.TrimSpace(req.ControlLabel),
				"control_level": strings.TrimSpace(req.ControlLevel),
				"lot_no":        strings.TrimSpace(req.LotNo),
			},
		}
		if err := m.saveManualQCRecord(strings.TrimSpace(req.RunDate), analysis, actor, time.Now().UTC()); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleQCTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := m.listQCTargets()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "qc_targets": items})
	case http.MethodPost:
		var item coremodel.QCTarget
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		saved, err := m.saveQCTarget(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "qc_target": saved})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleQCTargetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.URL.Path, "/api/qc-targets/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := m.getQCTarget(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "qc target not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "qc_target": item})
	case http.MethodPut:
		var item coremodel.QCTarget
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		saved, err := m.saveQCTarget(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "qc_target": saved})
	case http.MethodDelete:
		if err := m.deleteQCTarget(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleQCMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	service, ok := m.rt.Service("storage")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "storage service unavailable"})
		return
	}
	store, ok := service.(qcMetricsStore)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "qc metrics service unavailable"})
		return
	}
	metrics, err := store.QCPerformance(
		strings.TrimSpace(r.URL.Query().Get("analyte_tag")),
		strings.TrimSpace(r.URL.Query().Get("control_level")),
		strings.TrimSpace(r.URL.Query().Get("lot_no")),
		strings.TrimSpace(r.URL.Query().Get("date_from")),
		strings.TrimSpace(r.URL.Query().Get("date_to")),
		500,
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "metrics": metrics})
}

func (m *Module) handleDefaultResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		OrderAnalysisID int64 `json:"order_analysis_id"`
		ResultID        int64 `json:"result_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	store := m.orderStore()
	if store == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "storage service unavailable"})
		return
	}
	if err := store.SetDefaultResult(req.OrderAnalysisID, req.ResultID, m.currentRepeatMode()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (m *Module) currentLanguage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.language
}

func (m *Module) createSession(username string) (session, error) {
	id, err := randomID()
	if err != nil {
		return session{}, err
	}
	sess := session{
		ID:            id,
		Username:      username,
		MedicalUnitID: 0,
		UserType:      -1,
		FirstName:     username,
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(12 * time.Hour),
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess, nil
}

func (m *Module) createSessionFromWiseMED(info wisemedapi.LoginResponse, settings map[string]string) (session, error) {
	id, err := randomID()
	if err != nil {
		return session{}, err
	}
	medicalUnitID, _ := strconv.Atoi(strings.TrimSpace(settings["unitate_medicala_id"]))
	sess := session{
		ID:            id,
		Username:      firstNonEmpty(info.Login, info.FirstName, id),
		MedicalUnitID: medicalUnitID,
		UserType:      info.UserType,
		FirstName:     info.FirstName,
		LastName:      info.LastName,
		UserEmail:     info.UserEmail,
		UserPicture:   info.UserPicture,
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(12 * time.Hour),
	}
	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess, nil
}

func (m *Module) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return session{}, false
	}
	if sess, ok := m.decodeSessionToken(cookie.Value); ok {
		return sess, true
	}
	m.mu.RLock()
	sess, ok := m.sessions[cookie.Value]
	m.mu.RUnlock()
	if !ok || time.Now().UTC().After(sess.ExpiresAt) {
		if ok {
			m.mu.Lock()
			delete(m.sessions, cookie.Value)
			m.mu.Unlock()
		}
		return session{}, false
	}
	return sess, true
}

func (m *Module) sessionSecret() []byte {
	return []byte("wisemed-local-session:" + m.rt.ReaderID())
}

func (m *Module) encodeSessionToken(sess session) (string, error) {
	payload, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.sessionSecret())
	_, _ = mac.Write([]byte(payloadEncoded))
	signatureEncoded := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadEncoded + "." + signatureEncoded, nil
}

func (m *Module) decodeSessionToken(token string) (session, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return session{}, false
	}
	payloadEncoded := parts[0]
	signatureEncoded := parts[1]
	mac := hmac.New(sha256.New, m.sessionSecret())
	_, _ = mac.Write([]byte(payloadEncoded))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(signatureEncoded)
	if err != nil || !hmac.Equal(got, expected) {
		return session{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return session{}, false
	}
	var sess session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return session{}, false
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		return session{}, false
	}
	return sess, true
}

func (m *Module) readerPayload() map[string]interface{} {
	medicalUnitID := 0
	equipmentID := 0
	equipmentTypeID := 0
	updateMeta := m.appUpdateSettingsPayload()
	if wiseMED := m.wiseMEDAPI(); wiseMED != nil {
		settings := wiseMED.Settings()
		medicalUnitID, _ = strconv.Atoi(strings.TrimSpace(settings["unitate_medicala_id"]))
		equipmentID, _ = strconv.Atoi(strings.TrimSpace(settings["echipament_id"]))
		equipmentTypeID, _ = strconv.Atoi(strings.TrimSpace(settings["tip_de_echipament_id"]))
	}
	return map[string]interface{}{
		"id":                m.rt.ReaderID(),
		"client_id":         m.rt.ReaderID(),
		"label":             m.readerSetting("label", m.rt.ReaderID()),
		"medical_unit_id":   medicalUnitID,
		"equipment_id":      equipmentID,
		"equipment_type_id": equipmentTypeID,
		"analyzer_name":     m.readerSetting("analyzer_name", m.readerSetting("label", m.rt.ReaderID())),
		"analyzer_code":     m.readerSetting("analyzer_code", ""),
		"comm_type":         m.analyzerSetting("comm_type", ""),
		"protocol":          m.analyzerSetting("protocol", ""),
		"repeat_mode":       m.currentRepeatMode(),
		"app_version":       appmeta.CurrentVersion(),
		"app_update_app_id": firstNonEmpty(asString(updateMeta["app_id"]), m.rt.ReaderID()),
	}
}

func (m *Module) handleAppUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	force := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refresh")), "1") ||
		strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refresh")), "true")
	writeJSON(w, http.StatusOK, m.appUpdateStatus(force))
}

func (m *Module) handleAppUpdateSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.appUpdateSettingsPayload()})
	case http.MethodPut:
		var req struct {
			Enabled        string `json:"enabled"`
			AppID          string `json:"app_id"`
			CurrentVersion string `json:"current_version"`
			Channel        string `json:"channel"`
			BaseURL        string `json:"base_url"`
			AutoDownload   string `json:"auto_download"`
			DownloadDir    string `json:"download_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		if err := m.persistReaderSettings(map[string]interface{}{
			"modules.app-updates.enabled":       boolString(req.Enabled),
			"modules.app-updates.app_id":        strings.TrimSpace(req.AppID),
			"modules.app-updates.channel":       strings.TrimSpace(req.Channel),
			"modules.app-updates.base_url":      strings.TrimSpace(req.BaseURL),
			"modules.app-updates.auto_download": boolString(req.AutoDownload),
			"modules.app-updates.download_dir":  strings.TrimSpace(req.DownloadDir),
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		m.mu.Lock()
		m.updateInfo = nil
		m.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.appUpdateSettingsPayload()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) appUpdateSettingsPayload() map[string]interface{} {
	cfg := m.rt.ModuleSettings("app-updates")
	return map[string]interface{}{
		"enabled":         asString(cfg["enabled"]),
		"app_id":          firstNonEmpty(asString(cfg["app_id"]), m.rt.ReaderID()),
		"current_version": appmeta.CurrentVersion(),
		"channel":         firstNonEmpty(asString(cfg["channel"]), "stable"),
		"base_url":        asString(cfg["base_url"]),
		"auto_download":   asString(cfg["auto_download"]),
		"download_dir":    asString(cfg["download_dir"]),
	}
}

func (m *Module) appUpdateStatus(force bool) map[string]interface{} {
	settings := m.appUpdateSettingsPayload()
	if !force {
		m.mu.RLock()
		cached := cloneMap(m.updateInfo)
		m.mu.RUnlock()
		if cached != nil && sameAppUpdateCache(cached, settings) {
			if checkedAt, ok := cached["checked_at"].(string); ok {
				if ts, err := time.Parse(time.RFC3339, checkedAt); err == nil && time.Since(ts) < 30*time.Second {
					return cached
				}
			}
		}
	}
	status := map[string]interface{}{
		"ok":              true,
		"enabled":         boolString(asString(settings["enabled"])),
		"app_id":          settings["app_id"],
		"current_version": settings["current_version"],
		"channel":         settings["channel"],
		"base_url":        settings["base_url"],
		"latest_version":  settings["current_version"],
		"checked_at":      time.Now().UTC().Format(time.RFC3339),
		"state":           "disabled",
		"icon":            "off",
		"message":         "Verificarea update-urilor este dezactivata.",
	}
	if !boolString(asString(settings["enabled"])) {
		return status
	}
	baseURL := appupdates.ResolveBaseURL(asString(settings["base_url"]))
	apiKey := strSettingMap(m.rt.ModuleSettings("wisemed-api"), "cfg_wisemed_key")
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		status["state"] = "error"
		status["icon"] = "alert"
		status["message"] = "Configurarea update-server este incompleta."
		return m.storeAppUpdateStatus(status)
	}
	client := appupdates.NewClient(
		baseURL,
		apiKey,
		firstNonEmpty(asString(settings["app_id"]), m.rt.ReaderID()),
		firstNonEmpty(asString(settings["channel"]), "stable"),
	)
	resp, err := client.Check(appmeta.CurrentVersion(), runtime.GOOS, runtime.GOARCH)
	if err != nil {
		status["state"] = "error"
		status["icon"] = "alert"
		status["message"] = fmt.Sprintf("Nu se poate verifica update-ul: %v", err)
		return m.storeAppUpdateStatus(status)
	}
	status["response"] = resp
	status["latest_version"] = firstNonEmpty(resp.LatestVersion, appmeta.CurrentVersion())
	status["mandatory"] = resp.Mandatory
	status["download_url"] = resp.DownloadURL
	if strings.EqualFold(resp.Status, "update_available") {
		status["state"] = "update_available"
		status["icon"] = "alert"
		status["message"] = fmt.Sprintf("Exista o versiune noua: %s", firstNonEmpty(resp.LatestVersion, "?"))
		if resp.Mandatory {
			status["message"] = fmt.Sprintf("Exista un update obligatoriu: %s", firstNonEmpty(resp.LatestVersion, "?"))
		}
		return m.storeAppUpdateStatus(status)
	}
	if strings.EqualFold(resp.Status, "target_not_published") {
		status["state"] = "error"
		status["icon"] = "alert"
		status["message"] = "Nu exista un pachet publicat pentru aceasta platforma."
		return m.storeAppUpdateStatus(status)
	}
	status["state"] = "up_to_date"
	status["icon"] = "ok"
	status["message"] = fmt.Sprintf("Aplicatia este la zi: %s", firstNonEmpty(resp.CurrentVersion, appmeta.CurrentVersion()))
	return m.storeAppUpdateStatus(status)
}

func (m *Module) storeAppUpdateStatus(status map[string]interface{}) map[string]interface{} {
	copyStatus := cloneMap(status)
	m.mu.Lock()
	m.updateInfo = copyStatus
	m.mu.Unlock()
	return cloneMap(copyStatus)
}

func (m *Module) currentRepeatMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeRepeatMode(m.repeatMode)
}

func normalizeRepeatMode(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "grouped", "grupat", "group", "batch":
		return "grouped"
	default:
		return "individual"
	}
}

func (m *Module) persistModuleSettings(next map[string]interface{}) error {
	path := m.rt.ConfigPath()
	if strings.TrimSpace(path) == "" {
		return errors.New("config path unavailable")
	}
	updates := map[string]interface{}{}
	for key, value := range next {
		updates["modules."+m.ID()+"."+key] = value
	}
	return config.Update(path, updates)
}

func (m *Module) persistReaderSettings(next map[string]interface{}) error {
	path := m.rt.ConfigPath()
	if strings.TrimSpace(path) == "" {
		return errors.New("config path unavailable")
	}
	return config.Update(path, next)
}

func (m *Module) readerSettingsPayload() map[string]interface{} {
	settings := map[string]interface{}{
		"repeat_mode":                     m.currentRepeatMode(),
		"reader_id":                       m.readerSetting("id", m.rt.ReaderID()),
		"reader_label":                    m.readerSetting("label", m.rt.ReaderID()),
		"analyzer_name":                   m.readerSetting("analyzer_name", m.readerSetting("label", m.rt.ReaderID())),
		"analyzer_code":                   m.readerSetting("analyzer_code", ""),
		"db_name":                         m.readerSetting("db_name", ""),
		"local_http_address":              asString(m.rt.ModuleSettings(m.ID())["address"]),
		"local_http_language":             firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["language"]), m.currentLanguage()),
		"local_http_tls":                  asString(m.rt.ModuleSettings(m.ID())["tls"]),
		"local_http_cors_allowed_origins": firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["cors_allowed_origins"]), "https://ldse.wisemed.eu"),
		"analyzer_comm_type":              m.analyzerSetting("comm_type", ""),
		"analyzer_protocol":               m.analyzerSetting("protocol", ""),
		"sqlite_path":                     asString(m.rt.ModuleSettings("storage-sqlite")["path"]),
		"app_updates_enabled":             asString(m.rt.ModuleSettings("app-updates")["enabled"]),
		"app_updates_app_id":              firstNonEmpty(asString(m.rt.ModuleSettings("app-updates")["app_id"]), m.rt.ReaderID()),
		"app_updates_current_version":     appmeta.CurrentVersion(),
		"app_updates_channel":             firstNonEmpty(asString(m.rt.ModuleSettings("app-updates")["channel"]), "stable"),
		"app_updates_base_url":            asString(m.rt.ModuleSettings("app-updates")["base_url"]),
		"app_updates_auto_download":       asString(m.rt.ModuleSettings("app-updates")["auto_download"]),
		"app_updates_download_dir":        asString(m.rt.ModuleSettings("app-updates")["download_dir"]),
		"result_sync_enabled":             boolString(asString(m.rt.ModuleSettings("result-sync")["enabled"])),
		"result_sync_interval_minutes":    firstNonEmpty(asString(m.rt.ModuleSettings("result-sync")["interval_minutes"]), "5"),
		"result_sync_sample_prefixes":     joinStringList(m.rt.ModuleSettings("result-sync")["sample_prefixes"]),
		"result_sync_sample_suffixes":     joinStringList(m.rt.ModuleSettings("result-sync")["sample_suffixes"]),
		"result_sync_separators":          joinStringList(m.rt.ModuleSettings("result-sync")["separators"]),
		"result_sync_qc_prefixes":         formatQCPrefixSettings(m.rt.ModuleSettings("result-sync")["qc_prefixes"]),
	}

	cfg, err := config.Load(m.rt.ConfigPath())
	if err != nil || cfg == nil {
		return settings
	}

	localHTTP := cfg.ModuleSettings(m.ID())
	appUpdates := cfg.ModuleSettings("app-updates")
	resultSync := cfg.ModuleSettings("result-sync")
	storageSQLite := cfg.ModuleSettings("storage-sqlite")

	settings["repeat_mode"] = firstNonEmpty(asString(localHTTP["repeat_mode"]), asString(settings["repeat_mode"]))
	settings["reader_id"] = firstNonEmpty(strings.TrimSpace(cfg.Reader.ID), asString(settings["reader_id"]))
	settings["reader_label"] = firstNonEmpty(strings.TrimSpace(cfg.Reader.Label), asString(settings["reader_label"]))
	settings["analyzer_name"] = firstNonEmpty(strings.TrimSpace(cfg.Reader.AnalyzerName), asString(settings["analyzer_name"]))
	settings["analyzer_code"] = firstNonEmpty(strings.TrimSpace(cfg.Reader.AnalyzerCode), asString(settings["analyzer_code"]))
	settings["db_name"] = firstNonEmpty(strings.TrimSpace(cfg.Reader.DBName), asString(settings["db_name"]))
	settings["local_http_address"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.Address), asString(settings["local_http_address"]))
	settings["local_http_language"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.Language), asString(settings["local_http_language"]))
	settings["local_http_tls"] = firstNonEmpty(asString(cfg.LocalHTTP.TLS), asString(settings["local_http_tls"]))
	settings["local_http_cors_allowed_origins"] = firstNonEmpty(strings.TrimSpace(cfg.LocalHTTP.CORS), asString(localHTTP["cors_allowed_origins"]), asString(settings["local_http_cors_allowed_origins"]), "https://ldse.wisemed.eu")
	settings["analyzer_comm_type"] = firstNonEmpty(strings.TrimSpace(cfg.Analyzer.CommType), asString(settings["analyzer_comm_type"]))
	settings["analyzer_protocol"] = firstNonEmpty(strings.TrimSpace(cfg.Analyzer.Protocol), asString(settings["analyzer_protocol"]))
	settings["sqlite_path"] = firstNonEmpty(asString(storageSQLite["path"]), asString(settings["sqlite_path"]))
	settings["app_updates_enabled"] = firstNonEmpty(asString(appUpdates["enabled"]), asString(settings["app_updates_enabled"]))
	settings["app_updates_app_id"] = firstNonEmpty(asString(appUpdates["app_id"]), asString(settings["app_updates_app_id"]))
	settings["app_updates_channel"] = firstNonEmpty(asString(appUpdates["channel"]), asString(settings["app_updates_channel"]))
	settings["app_updates_base_url"] = firstNonEmpty(asString(appUpdates["base_url"]), asString(settings["app_updates_base_url"]))
	settings["app_updates_auto_download"] = firstNonEmpty(asString(appUpdates["auto_download"]), asString(settings["app_updates_auto_download"]))
	settings["app_updates_download_dir"] = firstNonEmpty(asString(appUpdates["download_dir"]), asString(settings["app_updates_download_dir"]))
	settings["result_sync_enabled"] = firstNonEmpty(asString(resultSync["enabled"]), asString(settings["result_sync_enabled"]))
	settings["result_sync_interval_minutes"] = firstNonEmpty(asString(resultSync["interval_minutes"]), asString(settings["result_sync_interval_minutes"]))
	settings["result_sync_sample_prefixes"] = firstNonEmpty(joinStringList(resultSync["sample_prefixes"]), asString(settings["result_sync_sample_prefixes"]))
	settings["result_sync_sample_suffixes"] = firstNonEmpty(joinStringList(resultSync["sample_suffixes"]), asString(settings["result_sync_sample_suffixes"]))
	settings["result_sync_separators"] = firstNonEmpty(joinStringList(resultSync["separators"]), asString(settings["result_sync_separators"]))
	settings["result_sync_qc_prefixes"] = firstNonEmpty(formatQCPrefixSettings(resultSync["qc_prefixes"]), asString(settings["result_sync_qc_prefixes"]))
	return settings
}

func (m *Module) readerSetting(key, fallback string) string {
	if service, ok := m.rt.Service("reader-config"); ok {
		if cfg, ok := service.(map[string]interface{}); ok {
			if value, ok := cfg[key].(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return fallback
}

func (m *Module) analyzerSetting(key, fallback string) string {
	if service, ok := m.rt.Service("analyzer-config"); ok {
		if cfg, ok := service.(map[string]interface{}); ok {
			if value, ok := cfg[key].(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return fallback
}

func (m *Module) wiseMEDAPI() wiseMedAPIService {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	api, _ := service.(wiseMedAPIService)
	return api
}

func (m *Module) resultSyncService() resultSyncService {
	service, ok := m.rt.Service("result-sync")
	if !ok {
		return nil
	}
	item, _ := service.(resultSyncService)
	return item
}

func (m *Module) wiseMEDState() map[string]interface{} {
	api := m.wiseMEDAPI()
	if api == nil {
		return map[string]interface{}{
			"configured":           false,
			"setup_complete":       false,
			"equipment_registered": false,
			"settings":             map[string]string{},
		}
	}
	return map[string]interface{}{
		"configured":           api.IsConfigured(),
		"setup_complete":       api.SetupComplete(),
		"equipment_registered": api.HasEquipmentID(),
		"settings":             api.PublicSettings(),
	}
}

func (m *Module) wiseMEDWSStatus() wiseMedWSStatusService {
	service, ok := m.rt.Service("wisemed-ws-status")
	if !ok {
		return nil
	}
	status, _ := service.(wiseMedWSStatusService)
	return status
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func asString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func boolString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "da":
		return true
	default:
		return false
	}
}

func parseIntString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		if text := strings.TrimSpace(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func joinStringList(raw interface{}) string {
	values := []string{}
	switch typed := raw.(type) {
	case []interface{}:
		for _, item := range typed {
			if text := strings.TrimSpace(asString(item)); text != "" {
				values = append(values, text)
			}
		}
	case []string:
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				values = append(values, text)
			}
		}
	}
	return strings.Join(values, ", ")
}

func parseQCPrefixSettings(raw string) []map[string]interface{} {
	parts := strings.Split(raw, ",")
	out := make([]map[string]interface{}, 0, len(parts))
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		row := map[string]interface{}{"prefix": item, "keep_as_lot": false}
		if strings.Contains(item, ":") {
			chunks := strings.SplitN(item, ":", 2)
			row["prefix"] = strings.TrimSpace(chunks[0])
			row["keep_as_lot"] = boolString(chunks[1])
		}
		if strings.TrimSpace(asString(row["prefix"])) != "" {
			out = append(out, row)
		}
	}
	return out
}

func formatQCPrefixSettings(raw interface{}) string {
	switch typed := raw.(type) {
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			row, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			prefix := strings.TrimSpace(asString(row["prefix"]))
			if prefix == "" {
				continue
			}
			values = append(values, fmt.Sprintf("%s:%t", prefix, boolString(asString(row["keep_as_lot"]))))
		}
		return strings.Join(values, ", ")
	default:
		return ""
	}
}

func strSettingMap(raw map[string]interface{}, key string) string {
	if raw == nil {
		return ""
	}
	return asString(raw[key])
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func sameAppUpdateCache(left, right map[string]interface{}) bool {
	keys := []string{"enabled", "app_id", "channel", "base_url", "download_dir", "auto_download"}
	for _, key := range keys {
		if asString(left[key]) != asString(right[key]) {
			return false
		}
	}
	return true
}

func (m *Module) analyteStore() analyteStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(analyteStore)
	return store
}

func (m *Module) targetStore() qcTargetStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(qcTargetStore)
	return store
}

func (m *Module) recordStore() qcRecordStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(qcRecordStore)
	return store
}

func (m *Module) orderStore() orderStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(orderStore)
	return store
}

func (m *Module) dailyDetailConfig() dailyDetailService {
	service, ok := m.rt.Service("daily-details")
	if !ok {
		return nil
	}
	details, _ := service.(dailyDetailService)
	return details
}

func (m *Module) dailyDetailStore() dailyDetailStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(dailyDetailStore)
	return store
}

func (m *Module) importer() fileImporter {
	service, ok := m.rt.Service("file-importer")
	if !ok {
		return nil
	}
	importer, _ := service.(fileImporter)
	return importer
}

func (m *Module) dailyDetailsDynamicEnabled() bool {
	service := m.dailyDetailConfig()
	if service == nil {
		return false
	}
	return service.DynamicDefinitionsEnabled()
}

func (m *Module) combinedDailyDetailDefinitions() ([]coremodel.DailyDetailDefinition, error) {
	out := []coremodel.DailyDetailDefinition{}
	index := map[string]int{}
	if service := m.dailyDetailConfig(); service != nil {
		for _, item := range service.Definitions() {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			index[key] = len(out)
			out = append(out, item)
		}
	}
	if store := m.dailyDetailStore(); store != nil {
		items, err := store.ListDailyDetailDefinitions()
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			if pos, ok := index[key]; ok {
				out[pos] = item
				continue
			}
			index[key] = len(out)
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func (m *Module) listDailyDetailValues(scopeDate string, roundNo int) ([]coremodel.DailyDetailValue, error) {
	store := m.dailyDetailStore()
	if store == nil {
		return []coremodel.DailyDetailValue{}, nil
	}
	return store.ListDailyDetailValues(scopeDate, roundNo)
}

func (m *Module) saveDailyDetailDefinition(item coremodel.DailyDetailDefinition) (coremodel.DailyDetailDefinition, error) {
	store := m.dailyDetailStore()
	if store == nil {
		return coremodel.DailyDetailDefinition{}, errors.New("storage service unavailable")
	}
	return store.SaveDailyDetailDefinition(item)
}

func (m *Module) deleteDailyDetailDefinition(id int64) error {
	store := m.dailyDetailStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	return store.DeleteDailyDetailDefinition(id)
}

func (m *Module) saveDailyDetailValue(item coremodel.DailyDetailValue) (coremodel.DailyDetailValue, error) {
	store := m.dailyDetailStore()
	if store == nil {
		return coremodel.DailyDetailValue{}, errors.New("storage service unavailable")
	}
	return store.SaveDailyDetailValue(item)
}

func (m *Module) listAnalytes() ([]coremodel.Analyte, error) {
	store := m.analyteStore()
	if store == nil {
		return append([]coremodel.Analyte(nil), m.analytes...), nil
	}
	return store.ListAnalytes()
}

func (m *Module) getAnalyteByID(id int64) (coremodel.Analyte, error) {
	store := m.analyteStore()
	if store == nil {
		for _, item := range m.analytes {
			if item.ID == id {
				return item, nil
			}
		}
		return coremodel.Analyte{}, errors.New("analyte not found")
	}
	return store.GetAnalyteByID(id)
}

func (m *Module) saveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error) {
	store := m.analyteStore()
	if store == nil {
		return coremodel.Analyte{}, errors.New("storage service unavailable")
	}
	return store.SaveAnalyte(item)
}

func (m *Module) deleteAnalyte(id int64) error {
	store := m.analyteStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	return store.DeleteAnalyte(id)
}

func (m *Module) listQCTargets() ([]coremodel.QCTarget, error) {
	store := m.targetStore()
	if store == nil {
		return append([]coremodel.QCTarget(nil), m.qcTargets...), nil
	}
	return store.ListQCTargets()
}

func (m *Module) getQCTarget(id int64) (coremodel.QCTarget, error) {
	store := m.targetStore()
	if store == nil {
		for _, item := range m.qcTargets {
			if item.ID == id {
				return item, nil
			}
		}
		return coremodel.QCTarget{}, errors.New("qc target not found")
	}
	return store.GetQCTarget(id)
}

func (m *Module) saveQCTarget(item coremodel.QCTarget) (coremodel.QCTarget, error) {
	store := m.targetStore()
	if store == nil {
		return coremodel.QCTarget{}, errors.New("storage service unavailable")
	}
	return store.SaveQCTarget(item)
}

func (m *Module) deleteQCTarget(id int64) error {
	store := m.targetStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	return store.DeleteQCTarget(id)
}

func (m *Module) listQCRecordBundles(dateFrom, dateTo string) ([]coremodel.QCRecordBundle, error) {
	store := m.recordStore()
	if store == nil {
		return []coremodel.QCRecordBundle{}, nil
	}
	if rangeStore, ok := store.(qcRecordRangeStore); ok {
		return rangeStore.ListQCRecordBundlesRange(dateFrom, dateTo)
	}
	return store.ListQCRecordBundles(dateFrom)
}

func (m *Module) saveManualQCRecord(runDate string, analysis coremodel.QCAnalysis, actor string, enteredAt time.Time) error {
	store := m.recordStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	if strings.TrimSpace(runDate) == "" {
		runDate = time.Now().Format("2006-01-02")
	}
	return store.SaveManualQCRecord(runDate, analysis, actor, enteredAt)
}

func (m *Module) renderCaryWorklistHTML(orderDate string, roundNo int, bundles []coremodel.OrderBundle, analyteIndex map[string]coremodel.Analyte, definitions []coremodel.DailyDetailDefinition, values []coremodel.DailyDetailValue) string {
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
	rows := make([]string, 0)
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
			rows = append(rows, `<tr>`+
				`<td>`+html.EscapeString(bundle.Order.SampleID)+`</td>`+
				`<td>`+html.EscapeString(bundle.Order.OrderDate)+`</td>`+
				`<td>`+html.EscapeString(worklistLabel)+`</td>`+
				`<td>`+html.EscapeString(firstNonEmpty(asString(flags["measured_concentration"]), item.Analysis.RawValue))+`</td>`+
				`<td>`+html.EscapeString(firstNonEmpty(asString(flags["dilution_factor"]), "-"))+`</td>`+
				`<td>`+html.EscapeString(firstNonEmpty(asString(flags["final_concentration"]), item.Analysis.ResultValue))+`</td>`+
				`<td class="sign"></td><td class="sign"></td>`+
				`</tr>`)
		}
	}
	headerRows := make([]string, 0, len(headers))
	for _, item := range headers {
		headerRows = append(headerRows, `<tr><td>`+html.EscapeString(item.Name)+`</td><td>`+html.EscapeString(item.AMartor)+`</td></tr>`)
	}
	return `<!doctype html><html lang="ro"><head><meta charset="utf-8"><title>Lista de lucru ` + html.EscapeString(orderDate) + `</title><style>
body{font-family:Arial,sans-serif;margin:24px;color:#111}
h1,h2{margin:0 0 12px}
.meta{margin-bottom:20px;color:#444}
table{width:100%;border-collapse:collapse;margin:0 0 20px}
th,td{border:1px solid #222;padding:8px 10px;font-size:12px;vertical-align:top}
th{background:#f2f2f2}
.head-grid{display:grid;grid-template-columns:340px 1fr;gap:24px;align-items:start;margin-bottom:20px}
.sign{height:32px;min-width:120px}
@media print{body{margin:8mm}.print-btn{display:none}}
</style></head><body>
<button class="print-btn" onclick="window.print()">Print</button>
<h1>Lista de lucru</h1>
<div class="meta">Data analizei: ` + html.EscapeString(orderDate) + ` · Runda: ` + html.EscapeString(strconv.Itoa(roundNo)) + `</div>
<div class="head-grid">
<div><h2>A martor</h2><table><thead><tr><th>Analiza</th><th>Valoare</th></tr></thead><tbody>` + strings.Join(headerRows, "") + `</tbody></table></div>
</div>
<table><thead><tr><th>Cod proba</th><th>Data analizei</th><th>Domeniu de lucru / UM</th><th>Concentratie masurata</th><th>Dilutie</th><th>Concentratie finala</th><th>Executant</th><th>Responsabil</th></tr></thead><tbody>` + strings.Join(rows, "") + `</tbody></table>
</body></html>`
}

func parsePathInt64(path, prefix string) (int64, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(path, prefix))
	if raw == "" || raw == path {
		return 0, errors.New("resource id is required")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("resource id must be a positive integer")
	}
	return v, nil
}

func randomID() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (m *Module) String() string {
	return fmt.Sprintf("local-http(%s)", m.rt.ReaderID())
}
