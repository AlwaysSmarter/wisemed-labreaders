package appupdateserver

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/modules/wisemedapi"
	"wisemed-labreaders/readersv3/shared/appupdates"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

//go:embed ui/*
var uiAssets embed.FS

const sessionCookieName = "wmr_update_server_session"

type Module struct {
	rt module.Runtime

	mu             sync.RWMutex
	db             *sql.DB
	server         *http.Server
	tlsEnabled     bool
	corsAllowed    string
	sessions       map[string]session
	downloadTokens map[string]downloadToken
}

type session struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	UserType  int       `json:"user_type"`
	FirstName string    `json:"first_name,omitempty"`
	LastName  string    `json:"last_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type application struct {
	ID          int64     `json:"id"`
	AppID       string    `json:"app_id"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type versionRecord struct {
	ID                      int64     `json:"id"`
	ApplicationID           int64     `json:"application_id"`
	Version                 string    `json:"version"`
	Channel                 string    `json:"channel"`
	TargetOS                string    `json:"target_os"`
	TargetArch              string    `json:"target_arch"`
	Mandatory               bool      `json:"mandatory"`
	DownloadURL             string    `json:"download_url"`
	FileName                string    `json:"file_name"`
	FilePath                string    `json:"file_path"`
	ChecksumSHA256          string    `json:"checksum_sha256"`
	FileSize                int64     `json:"file_size"`
	InstallerFileName       string    `json:"installer_file_name"`
	InstallerFilePath       string    `json:"installer_file_path"`
	InstallerChecksumSHA256 string    `json:"installer_checksum_sha256"`
	InstallerFileSize       int64     `json:"installer_file_size"`
	ReleaseNotes            string    `json:"release_notes"`
	UploadedBy              string    `json:"uploaded_by"`
	CreatedAt               time.Time `json:"created_at"`
	Active                  bool      `json:"active"`
}

type managedReaderRelease struct {
	SourceAppID string
	UpdateAppID string
}

var managedReaderUpdateAppIDAliases = map[string]string{
	"labnovation-ld-560-reader-v3": "labnovation-ld-560",
	"labnovatiob-ld-560-reader-v3": "labnovation-ld-560",
}

type releaseRequest struct {
	Channel      string `json:"channel"`
	TargetOS     string `json:"target_os"`
	TargetArch   string `json:"target_arch"`
	Mandatory    bool   `json:"mandatory"`
	ReleaseNotes string `json:"release_notes"`
}

type downloadToken struct {
	Token     string
	VersionID int64
	AppID     string
	ExpiresAt time.Time
}

type wiseMedAPIService interface {
	Settings() map[string]string
	IsConfigured() bool
	SaveSetup(map[string]string) (map[string]string, error)
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "app-update-server" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.sessions = map[string]session{}
	m.downloadTokens = map[string]downloadToken{}
	m.corsAllowed = firstNonEmpty(asString(rt.ModuleSettings("local-http")["cors_allowed_origins"]), "https://ldse.wisemed.eu")
	if err := m.openDB(); err != nil {
		return err
	}
	m.rt.Handle("/", m.withCORS(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/settings", m.withCORS(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/help-ui", m.withCORS(http.HandlerFunc(m.handleHelpRedirect)))
	m.rt.Handle("/app-updates/app.js", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/app.js", "application/javascript; charset=utf-8"))))
	m.rt.Handle("/app-updates/styles.css", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/styles.css", "text/css; charset=utf-8"))))
	m.rt.Handle("/api/session", m.withCORS(http.HandlerFunc(m.handleSession)))
	m.rt.Handle("/api/session/login", m.withCORS(http.HandlerFunc(m.handleLogin)))
	m.rt.Handle("/api/session/logout", m.withCORS(m.requireSession(http.HandlerFunc(m.handleLogout))))
	m.rt.Handle("/api/update-server/meta", m.withCORS(http.HandlerFunc(m.handleMeta)))
	m.rt.Handle("/api/update-server/settings", m.withCORS(m.requireSession(http.HandlerFunc(m.handleSettings))))
	m.rt.Handle("/api/update-server/apps", m.withCORS(m.requireSession(http.HandlerFunc(m.handleApps))))
	m.rt.Handle("/api/update-server/apps/", m.withCORS(m.requireSession(http.HandlerFunc(m.handleAppSubroutes))))
	m.rt.Handle("/api/update-server/versions/", m.withCORS(m.requireSession(http.HandlerFunc(m.handleVersionByID))))
	m.rt.Handle("/api/update-server/package-download/", m.withCORS(m.requireSession(http.HandlerFunc(m.handlePackageDownload))))
	m.rt.Handle("/api/update-server/installer-download/", m.withCORS(m.requireSession(http.HandlerFunc(m.handleInstallerDownload))))
	m.rt.Handle("/api/update-server/download-link/", m.withCORS(m.requireSession(http.HandlerFunc(m.handleAdminDownloadLink))))
	m.rt.Handle("/api/public/check-update", m.withCORS(http.HandlerFunc(m.handlePublicCheckUpdate)))
	m.rt.Handle("/api/public/download/", m.withCORS(http.HandlerFunc(m.handlePublicDownload)))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	addr := strings.TrimSpace(asString(m.rt.ModuleSettings("local-http")["address"]))
	if addr == "" {
		addr = "127.0.0.1:19090"
	}
	useTLS := parseBoolString(asString(m.rt.ModuleSettings("local-http")["tls"]))
	m.server = &http.Server{
		Addr:              addr,
		Handler:           m.rt.Mux(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.mu.Lock()
	m.tlsEnabled = useTLS
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if m.server != nil {
			_ = m.server.Shutdown(shutdownCtx)
		}
		if m.db != nil {
			_ = m.db.Close()
		}
	}()
	if useTLS {
		certFile, keyFile, err := ensureLocalHTTPSMaterial(m.rt.ConfigDir(), addr)
		if err != nil {
			return err
		}
		m.rt.Logf("app update server listening on https://%s", addr)
		if err := m.server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	m.rt.Logf("app update server listening on http://%s", addr)
	if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (m *Module) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/settings" {
		http.NotFound(w, r)
		return
	}
	m.handleStaticAsset("ui/index.html", "text/html; charset=utf-8")(w, r)
}

func (m *Module) handleHelpRedirect(w http.ResponseWriter, r *http.Request) {
	m.handleStaticAsset("ui/help-redirect.html", "text/html; charset=utf-8")(w, r)
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
	return firstNonEmpty(m.corsAllowed, "https://ldse.wisemed.eu")
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

func (m *Module) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := m.currentSession(r); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Module) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	sess, ok := m.currentSession(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"authenticated":  ok,
		"session":        sess,
		"wisemed_ready":  m.wiseMEDReady(),
		"service_config": m.publicSettings(),
	})
}

func (m *Module) handleMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"settings": m.publicSettings(),
	})
}

func (m *Module) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	info, err := m.loginWiseMED(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if !m.isAdminUserType(info.UserType) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": fmt.Sprintf("user type %d is not allowed on update server", info.UserType)})
		return
	}
	sess := session{
		ID:        randomToken(),
		Username:  firstNonEmpty(info.Login, req.Username),
		UserType:  info.UserType,
		FirstName: info.FirstName,
		LastName:  info.LastName,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(12 * time.Hour),
	}
	m.mu.Lock()
	m.sessions[sess.ID] = sess
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "session": sess})
}

func (m *Module) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
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

func (m *Module) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.publicSettings()})
	case http.MethodPut:
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		next := map[string]interface{}{
			"files_dir":          strings.TrimSpace(asString(req["files_dir"])),
			"public_base_url":    strings.TrimSpace(asString(req["public_base_url"])),
			"allowed_user_types": strings.TrimSpace(asString(req["allowed_user_types"])),
		}
		if raw := strings.TrimSpace(asString(req["cfg_wisemed_protocol"])); raw != "" {
			next["cfg_wisemed_protocol"] = raw
		}
		if raw := strings.TrimSpace(asString(req["cfg_wisemed_ip"])); raw != "" {
			next["cfg_wisemed_ip"] = raw
		}
		if raw := strings.TrimSpace(asString(req["cfg_wisemed_port"])); raw != "" {
			next["cfg_wisemed_port"] = raw
		}
		if raw := strings.TrimSpace(asString(req["cfg_wisemed_path"])); raw != "" {
			next["cfg_wisemed_path"] = raw
		}
		if raw := strings.TrimSpace(asString(req["cfg_wisemed_key"])); raw != "" {
			next["cfg_wisemed_key"] = raw
		}
		if err := m.persistSettings(next); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		if wise := m.wiseMED(); wise != nil {
			save := map[string]string{}
			for _, key := range []string{"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key"} {
				if value := strings.TrimSpace(asString(next[key])); value != "" {
					save[key] = value
				}
			}
			if len(save) > 0 {
				_, _ = wise.SaveSetup(save)
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.publicSettings()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleApps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := m.listApplications()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "apps": items})
	case http.MethodPost:
		var item application
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		saved, err := m.saveApplication(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "app": saved})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleAppSubroutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/update-server/apps/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	appID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid application id"})
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var item application
			if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
				return
			}
			item.ID = appID
			saved, err := m.saveApplication(item)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "app": saved})
		case http.MethodDelete:
			if err := m.deleteApplication(appID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		}
		return
	}
	if len(parts) == 2 && parts[1] == "versions" {
		switch r.Method {
		case http.MethodGet:
			items, err := m.listVersions(appID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "versions": items})
		case http.MethodPost:
			sess, _ := m.currentSession(r)
			saved, err := m.createVersionFromRequest(r, appID, sess.Username)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "version": saved})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		}
		return
	}
	if len(parts) == 2 && parts[1] == "make-release" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
			return
		}
		sess, _ := m.currentSession(r)
		saved, err := m.makeReleaseFromRequest(r, appID, sess.Username)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "version": saved})
		return
	}
	http.NotFound(w, r)
}

func (m *Module) handleVersionByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/update-server/versions/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid version id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := m.getVersion(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "version": item})
	case http.MethodPut:
		var item versionRecord
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		saved, err := m.saveVersion(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "version": saved})
	case http.MethodDelete:
		if err := m.deleteVersion(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleInstallerDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/update-server/installer-download/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid version id"})
		return
	}
	item, err := m.getVersion(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(item.InstallerFilePath) == "" {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "installer is unavailable for this version"})
		return
	}
	target := m.resolveFilesPath(item.InstallerFilePath)
	f, err := os.Open(target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapeHeader(item.InstallerFileName)))
	http.ServeContent(w, r, item.InstallerFileName, item.CreatedAt, f)
}

func (m *Module) handlePackageDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/update-server/package-download/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid version id"})
		return
	}
	item, err := m.getVersion(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	artifact := m.preferredPublicArtifact(item)
	if strings.TrimSpace(artifact.FilePath) == "" {
		target := strings.TrimSpace(artifact.DownloadURL)
		if target == "" {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "download is unavailable"})
			return
		}
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	target := m.resolveFilesPath(artifact.FilePath)
	f, err := os.Open(target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapeHeader(artifact.FileName)))
	http.ServeContent(w, r, artifact.FileName, item.CreatedAt, f)
}

func (m *Module) handlePublicCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "message": "method not allowed"})
		return
	}
	claims, err := m.verifyPublicJWT(r, "check")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	var req appupdates.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "invalid json body"})
		return
	}
	req.AppID = firstNonEmpty(req.AppID, claims.AppID)
	if req.AppID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "app_id is required"})
		return
	}
	appItem, err := m.findApplicationByAppID(req.AppID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "message": "application not found"})
		return
	}
	versions, err := m.listVersions(appItem.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	if countActiveVersions(versions) == 0 {
		current := appupdates.NormalizeVersion(req.CurrentVersion)
		writeJSON(w, http.StatusOK, appupdates.CheckResponse{
			OK:             true,
			Status:         "up_to_date",
			AppID:          req.AppID,
			CurrentVersion: current,
			LatestVersion:  current,
			Message:        "No published versions exist for this application",
		})
		return
	}
	best, ok := pickBestVersion(versions, req.Channel, req.OS, req.Arch)
	if !ok {
		writeJSON(w, http.StatusOK, appupdates.CheckResponse{
			OK:             true,
			Status:         "target_not_published",
			AppID:          req.AppID,
			CurrentVersion: req.CurrentVersion,
			Message:        "No published version matched this target",
		})
		return
	}
	current := appupdates.NormalizeVersion(req.CurrentVersion)
	latest := appupdates.NormalizeVersion(best.Version)
	if current != "" && appupdates.CompareVersions(current, latest) >= 0 {
		writeJSON(w, http.StatusOK, appupdates.CheckResponse{
			OK:             true,
			Status:         "up_to_date",
			AppID:          req.AppID,
			CurrentVersion: current,
			LatestVersion:  latest,
			Message:        "Application is already at the latest version",
		})
		return
	}
	writeJSON(w, http.StatusOK, appupdates.CheckResponse{
		OK:              true,
		Status:          "update_available",
		AppID:           req.AppID,
		CurrentVersion:  current,
		LatestVersion:   latest,
		Mandatory:       best.Mandatory,
		DownloadURL:     m.publicDownloadURL(r, best, m.issueDownloadToken(best.ID, req.AppID), req.OS),
		ChecksumSHA256:  m.publicDownloadChecksum(best, req.OS),
		FileName:        m.publicDownloadFileName(best, req.OS),
		ReleaseNotes:    best.ReleaseNotes,
		VersionRecordID: best.ID,
		Message:         "Update available",
	})
}

func (m *Module) handlePublicDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "message": "method not allowed"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/public/download/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "invalid version id"})
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "message": "missing token"})
		return
	}
	if _, err := m.consumeDownloadToken(token, id); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	artifactKind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("artifact")))
	item, err := m.getVersion(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	if artifactKind == "installer" && strings.TrimSpace(item.InstallerFilePath) != "" {
		target := m.resolveFilesPath(item.InstallerFilePath)
		f, err := os.Open(target)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "message": err.Error()})
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapeHeader(item.InstallerFileName)))
		http.ServeContent(w, r, item.InstallerFileName, item.CreatedAt, f)
		return
	}
	artifact := m.preferredPublicArtifact(item)
	if strings.TrimSpace(artifact.FilePath) == "" {
		target := strings.TrimSpace(artifact.DownloadURL)
		if target == "" {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "message": "download is unavailable"})
			return
		}
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	target := m.resolveFilesPath(artifact.FilePath)
	f, err := os.Open(target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "message": err.Error()})
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, escapeHeader(artifact.FileName)))
	http.ServeContent(w, r, artifact.FileName, item.CreatedAt, f)
}

func (m *Module) handleAdminDownloadLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/update-server/download-link/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid version id"})
		return
	}
	item, err := m.getVersion(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	appItem, err := m.findApplicationByID(item.ApplicationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	artifactKind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("artifact")))
	token := m.issueDownloadToken(item.ID, appItem.AppID)
	downloadURL := m.publicDownloadURL(r, item, token, "")
	if artifactKind == "installer" && strings.TrimSpace(item.InstallerFilePath) != "" {
		downloadURL = m.publicDownloadURL(r, item, token, "installer")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"download_url": downloadURL,
	})
}

func (m *Module) openDB() error {
	path := resolvePath(m.rt.ConfigDir(), firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["db_path"]), "./app-update-server.db"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	m.db = db
	for _, pragma := range []string{
		`pragma busy_timeout = 5000`,
		`pragma journal_mode = wal`,
		`pragma synchronous = normal`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	schema := []string{
		`create table if not exists applications (
			id integer primary key autoincrement,
			app_id text not null unique,
			display_name text not null,
			description text not null default '',
			active integer not null default 1,
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists app_versions (
			id integer primary key autoincrement,
			application_id integer not null,
			version text not null,
			channel text not null default '',
			target_os text not null default '',
			target_arch text not null default '',
			mandatory integer not null default 0,
			download_url text not null default '',
			file_name text not null default '',
			file_path text not null default '',
			checksum_sha256 text not null default '',
			file_size integer not null default 0,
			installer_file_name text not null default '',
			installer_file_path text not null default '',
			installer_checksum_sha256 text not null default '',
			installer_file_size integer not null default 0,
			release_notes text not null default '',
			uploaded_by text not null default '',
			created_at text not null,
			active integer not null default 1,
			foreign key(application_id) references applications(id)
		)`,
	}
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	for column, ddl := range map[string]string{
		"installer_file_name":       `alter table app_versions add column installer_file_name text not null default ''`,
		"installer_file_path":       `alter table app_versions add column installer_file_path text not null default ''`,
		"installer_checksum_sha256": `alter table app_versions add column installer_checksum_sha256 text not null default ''`,
		"installer_file_size":       `alter table app_versions add column installer_file_size integer not null default 0`,
	} {
		if err := ensureColumnExists(db, "app_versions", column, ddl); err != nil {
			return err
		}
	}
	if err := m.repairWindowsManagedArtifacts(); err != nil {
		return err
	}
	return nil
}

func (m *Module) repairWindowsManagedArtifacts() error {
	rows, err := m.db.Query(`select id, file_name, file_path, checksum_sha256, file_size, installer_file_name, installer_file_path from app_versions where lower(target_os)='windows' and installer_file_path<>''`)
	if err != nil {
		return err
	}

	type repairCandidate struct {
		id                int64
		filePath          string
		installerFilePath string
	}
	candidates := make([]repairCandidate, 0)
	repaired := 0
	for rows.Next() {
		var (
			id                int64
			fileName          string
			filePath          string
			checksumSHA256    string
			fileSize          int64
			installerFileName string
			installerFilePath string
		)
		if err := rows.Scan(&id, &fileName, &filePath, &checksumSHA256, &fileSize, &installerFileName, &installerFilePath); err != nil {
			return err
		}
		if !needsWindowsArtifactRepair(filePath, installerFilePath) {
			continue
		}
		candidates = append(candidates, repairCandidate{
			id:                id,
			filePath:          filePath,
			installerFilePath: installerFilePath,
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, candidate := range candidates {
		updateName, updatePath, updateChecksum, updateSize, ok := m.findManagedUpdateArchive(candidate.installerFilePath)
		if !ok {
			continue
		}
		if _, err := m.db.Exec(`update app_versions set file_name=?, file_path=?, checksum_sha256=?, file_size=? where id=?`,
			updateName, updatePath, updateChecksum, updateSize, candidate.id); err != nil {
			return err
		}
		repaired++
	}
	if repaired > 0 {
		m.rt.Logf("app-update-server: repaired %d windows update artifact record(s)", repaired)
	}
	return nil
}

func needsWindowsArtifactRepair(filePath, installerFilePath string) bool {
	filePath = strings.TrimSpace(filePath)
	installerFilePath = strings.TrimSpace(installerFilePath)
	if installerFilePath == "" {
		return false
	}
	if filePath == "" {
		return true
	}
	if !strings.EqualFold(filepath.Ext(filePath), ".zip") {
		return true
	}
	return strings.EqualFold(filePath, installerFilePath)
}

func (m *Module) findManagedUpdateArchive(installerFilePath string) (string, string, string, int64, bool) {
	versionDir := filepath.Dir(strings.TrimSpace(installerFilePath))
	if strings.EqualFold(filepath.Base(versionDir), "installers") {
		versionDir = filepath.Dir(versionDir)
	}
	absVersionDir := m.resolveFilesPath(versionDir)
	entries, err := os.ReadDir(absVersionDir)
	if err != nil {
		return "", "", "", 0, false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			continue
		}
		relPath := filepath.Join(versionDir, entry.Name())
		absPath := filepath.Join(absVersionDir, entry.Name())
		checksum, size, err := fileChecksumAndSize(absPath)
		if err != nil {
			return "", "", "", 0, false
		}
		return entry.Name(), relPath, checksum, size, true
	}
	return "", "", "", 0, false
}

func fileChecksumAndSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func ensureColumnExists(db *sql.DB, tableName, columnName, ddl string) error {
	rows, err := db.Query(`pragma table_info(` + tableName + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	_, err = db.Exec(ddl)
	return err
}

func (m *Module) listApplications() ([]application, error) {
	rows, err := m.db.Query(`select id, app_id, display_name, description, active, created_at, updated_at from applications order by lower(display_name), lower(app_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []application{}
	for rows.Next() {
		var item application
		var active int
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&item.ID, &item.AppID, &item.DisplayName, &item.Description, &active, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.Active = active != 0
		item.CreatedAt = parseTime(createdAt)
		item.UpdatedAt = parseTime(updatedAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (m *Module) saveApplication(item application) (application, error) {
	item.AppID = strings.TrimSpace(item.AppID)
	item.DisplayName = strings.TrimSpace(item.DisplayName)
	if item.AppID == "" || item.DisplayName == "" {
		return application{}, errors.New("app_id and display_name are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if item.ID > 0 {
		_, err := m.db.Exec(`update applications set app_id=?, display_name=?, description=?, active=?, updated_at=? where id=?`,
			item.AppID, item.DisplayName, strings.TrimSpace(item.Description), boolToInt(item.Active), now, item.ID)
		if err != nil {
			return application{}, err
		}
	} else {
		res, err := m.db.Exec(`insert into applications(app_id, display_name, description, active, created_at, updated_at) values(?,?,?,?,?,?)`,
			item.AppID, item.DisplayName, strings.TrimSpace(item.Description), boolToInt(item.Active), now, now)
		if err != nil {
			return application{}, err
		}
		item.ID, _ = res.LastInsertId()
	}
	return m.findApplicationByID(item.ID)
}

func (m *Module) findApplicationByID(id int64) (application, error) {
	var item application
	var active int
	var createdAt string
	var updatedAt string
	err := m.db.QueryRow(`select id, app_id, display_name, description, active, created_at, updated_at from applications where id=?`, id).
		Scan(&item.ID, &item.AppID, &item.DisplayName, &item.Description, &active, &createdAt, &updatedAt)
	if err != nil {
		return application{}, err
	}
	item.Active = active != 0
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func (m *Module) findApplicationByAppID(appID string) (application, error) {
	var item application
	var active int
	var createdAt string
	var updatedAt string
	err := m.db.QueryRow(`select id, app_id, display_name, description, active, created_at, updated_at from applications where app_id=?`, strings.TrimSpace(appID)).
		Scan(&item.ID, &item.AppID, &item.DisplayName, &item.Description, &active, &createdAt, &updatedAt)
	if err != nil {
		return application{}, err
	}
	item.Active = active != 0
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return item, nil
}

func (m *Module) deleteApplication(id int64) error {
	if _, err := m.findApplicationByID(id); err != nil {
		return err
	}
	versions, err := m.listVersions(id)
	if err != nil {
		return err
	}
	for _, item := range versions {
		if err := m.deleteVersion(item.ID); err != nil {
			return err
		}
	}
	if _, err := m.db.Exec(`delete from applications where id=?`, id); err != nil {
		return err
	}
	return nil
}

func (m *Module) listVersions(appID int64) ([]versionRecord, error) {
	rows, err := m.db.Query(`select id, application_id, version, channel, target_os, target_arch, mandatory, download_url, file_name, file_path, checksum_sha256, file_size, installer_file_name, installer_file_path, installer_checksum_sha256, installer_file_size, release_notes, uploaded_by, created_at, active from app_versions where application_id=? order by created_at desc, id desc`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []versionRecord{}
	for rows.Next() {
		var item versionRecord
		var mandatory int
		var active int
		var createdAt string
		if err := rows.Scan(&item.ID, &item.ApplicationID, &item.Version, &item.Channel, &item.TargetOS, &item.TargetArch, &mandatory, &item.DownloadURL, &item.FileName, &item.FilePath, &item.ChecksumSHA256, &item.FileSize, &item.InstallerFileName, &item.InstallerFilePath, &item.InstallerChecksumSHA256, &item.InstallerFileSize, &item.ReleaseNotes, &item.UploadedBy, &createdAt, &active); err != nil {
			return nil, err
		}
		item.Mandatory = mandatory != 0
		item.Active = active != 0
		item.CreatedAt = parseTime(createdAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (m *Module) getVersion(id int64) (versionRecord, error) {
	var item versionRecord
	var mandatory int
	var active int
	var createdAt string
	err := m.db.QueryRow(`select id, application_id, version, channel, target_os, target_arch, mandatory, download_url, file_name, file_path, checksum_sha256, file_size, installer_file_name, installer_file_path, installer_checksum_sha256, installer_file_size, release_notes, uploaded_by, created_at, active from app_versions where id=?`, id).
		Scan(&item.ID, &item.ApplicationID, &item.Version, &item.Channel, &item.TargetOS, &item.TargetArch, &mandatory, &item.DownloadURL, &item.FileName, &item.FilePath, &item.ChecksumSHA256, &item.FileSize, &item.InstallerFileName, &item.InstallerFilePath, &item.InstallerChecksumSHA256, &item.InstallerFileSize, &item.ReleaseNotes, &item.UploadedBy, &createdAt, &active)
	if err != nil {
		return versionRecord{}, err
	}
	item.Mandatory = mandatory != 0
	item.Active = active != 0
	item.CreatedAt = parseTime(createdAt)
	return item, nil
}

func (m *Module) deleteVersion(id int64) error {
	item, err := m.getVersion(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.FilePath) != "" {
		target := m.resolveFilesPath(item.FilePath)
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_ = removeEmptyParents(filepath.Dir(target), m.resolveFilesPath(""))
	}
	if strings.TrimSpace(item.InstallerFilePath) != "" {
		target := m.resolveFilesPath(item.InstallerFilePath)
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_ = removeEmptyParents(filepath.Dir(target), m.resolveFilesPath(""))
	}
	if _, err := m.db.Exec(`delete from app_versions where id=?`, id); err != nil {
		return err
	}
	return nil
}

func (m *Module) saveVersion(item versionRecord) (versionRecord, error) {
	item.Version = appupdates.NormalizeVersion(item.Version)
	if item.ApplicationID <= 0 || item.Version == "" {
		return versionRecord{}, errors.New("application_id and version are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if item.ID > 0 {
		_, err := m.db.Exec(`update app_versions set version=?, channel=?, target_os=?, target_arch=?, mandatory=?, download_url=?, file_name=?, file_path=?, checksum_sha256=?, file_size=?, installer_file_name=?, installer_file_path=?, installer_checksum_sha256=?, installer_file_size=?, release_notes=?, active=? where id=?`,
			item.Version, strings.TrimSpace(item.Channel), strings.TrimSpace(item.TargetOS), strings.TrimSpace(item.TargetArch), boolToInt(item.Mandatory), strings.TrimSpace(item.DownloadURL), strings.TrimSpace(item.FileName), strings.TrimSpace(item.FilePath), strings.TrimSpace(item.ChecksumSHA256), item.FileSize, strings.TrimSpace(item.InstallerFileName), strings.TrimSpace(item.InstallerFilePath), strings.TrimSpace(item.InstallerChecksumSHA256), item.InstallerFileSize, strings.TrimSpace(item.ReleaseNotes), boolToInt(item.Active), item.ID)
		if err != nil {
			return versionRecord{}, err
		}
	} else {
		res, err := m.db.Exec(`insert into app_versions(application_id, version, channel, target_os, target_arch, mandatory, download_url, file_name, file_path, checksum_sha256, file_size, installer_file_name, installer_file_path, installer_checksum_sha256, installer_file_size, release_notes, uploaded_by, created_at, active) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			item.ApplicationID, item.Version, strings.TrimSpace(item.Channel), strings.TrimSpace(item.TargetOS), strings.TrimSpace(item.TargetArch), boolToInt(item.Mandatory), strings.TrimSpace(item.DownloadURL), strings.TrimSpace(item.FileName), strings.TrimSpace(item.FilePath), strings.TrimSpace(item.ChecksumSHA256), item.FileSize, strings.TrimSpace(item.InstallerFileName), strings.TrimSpace(item.InstallerFilePath), strings.TrimSpace(item.InstallerChecksumSHA256), item.InstallerFileSize, strings.TrimSpace(item.ReleaseNotes), strings.TrimSpace(item.UploadedBy), now, boolToInt(item.Active))
		if err != nil {
			return versionRecord{}, err
		}
		item.ID, _ = res.LastInsertId()
	}
	return m.getVersion(item.ID)
}

func (m *Module) createVersionFromRequest(r *http.Request, appID int64, actor string) (versionRecord, error) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		return versionRecord{}, err
	}
	item := versionRecord{
		ApplicationID: appID,
		Version:       appupdates.NormalizeVersion(r.FormValue("version")),
		Channel:       strings.TrimSpace(r.FormValue("channel")),
		TargetOS:      strings.TrimSpace(r.FormValue("target_os")),
		TargetArch:    strings.TrimSpace(r.FormValue("target_arch")),
		Mandatory:     parseBoolString(r.FormValue("mandatory")),
		ReleaseNotes:  strings.TrimSpace(r.FormValue("release_notes")),
		UploadedBy:    strings.TrimSpace(actor),
		Active:        true,
	}
	if item.Version == "" {
		return versionRecord{}, errors.New("version is required")
	}
	if raw := strings.TrimSpace(r.FormValue("download_url")); raw != "" {
		item.DownloadURL = raw
		item.FileName = filepath.Base(raw)
	}
	file, header, err := r.FormFile("package")
	if err == nil {
		defer file.Close()
		meta, path, hash, size, err := m.saveUpload(file, header, appID, item.Version)
		if err != nil {
			return versionRecord{}, err
		}
		item.FileName = meta
		item.FilePath = path
		item.ChecksumSHA256 = hash
		item.FileSize = size
	}
	if item.DownloadURL == "" && item.FilePath == "" {
		return versionRecord{}, errors.New("upload a package or provide an external download_url")
	}
	return m.saveVersion(item)
}

func (m *Module) makeReleaseFromRequest(r *http.Request, appID int64, actor string) (versionRecord, error) {
	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return versionRecord{}, errors.New("invalid json body")
	}
	req.Channel = firstNonEmpty(strings.TrimSpace(req.Channel), "stable")
	req.TargetOS = strings.TrimSpace(strings.ToLower(req.TargetOS))
	req.TargetArch = strings.TrimSpace(strings.ToLower(req.TargetArch))
	if req.TargetOS == "" || req.TargetArch == "" {
		return versionRecord{}, errors.New("target_os and target_arch are required")
	}
	if !isSupportedReleaseTarget(req.TargetOS, req.TargetArch) {
		return versionRecord{}, fmt.Errorf("unsupported target %s/%s", req.TargetOS, req.TargetArch)
	}

	appItem, err := m.findApplicationByID(appID)
	if err != nil {
		return versionRecord{}, err
	}
	repoRoot, err := m.readersRepoRoot()
	if err != nil {
		return versionRecord{}, err
	}
	managed, err := discoverManagedReader(repoRoot, appItem.AppID)
	if err != nil {
		return versionRecord{}, err
	}
	versions, err := m.listVersions(appID)
	if err != nil {
		return versionRecord{}, err
	}
	nextVersion := nextReleaseVersion(versions)
	target := req.TargetOS + "-" + req.TargetArch

	m.rt.Logf("update-server: pregatesc release-ul app_id=%s source_app=%s version=%s target=%s actor=%s", appItem.AppID, managed.SourceAppID, nextVersion, target, strings.TrimSpace(actor))

	releaseResult, err := m.runReleaseCtlRelease(repoRoot, managed.SourceAppID, target, nextVersion)
	if err != nil {
		m.rt.Logf("update-server: release esuat app_id=%s version=%s target=%s err=%v", appItem.AppID, nextVersion, target, err)
		return versionRecord{}, err
	}
	m.rt.Logf("update-server: compilarea s-a incheiat app_id=%s version=%s target=%s update_artifact=%s", appItem.AppID, nextVersion, target, strings.TrimSpace(releaseResult.Update.Path))
	fileName := sanitizeFileName(releaseResult.Update.FileName)
	if fileName == "" {
		return versionRecord{}, errors.New("release did not produce a valid update artifact")
	}
	relFilePath := filepath.Join(appItem.AppID, nextVersion, fileName)
	absFilePath := m.resolveFilesPath(relFilePath)
	if err := copyManagedArtifact(releaseResult.Update.Path, absFilePath); err != nil {
		m.rt.Logf("update-server: copiere update artifact esuata src=%s dst=%s err=%v", strings.TrimSpace(releaseResult.Update.Path), absFilePath, err)
		return versionRecord{}, err
	}
	m.rt.Logf("update-server: update artifact copiat la %s", absFilePath)

	item := versionRecord{
		ApplicationID:  appID,
		Version:        nextVersion,
		Channel:        req.Channel,
		TargetOS:       req.TargetOS,
		TargetArch:     req.TargetArch,
		Mandatory:      req.Mandatory,
		FileName:       fileName,
		FilePath:       relFilePath,
		ChecksumSHA256: strings.TrimSpace(releaseResult.Update.ChecksumSHA256),
		FileSize:       releaseResult.Update.Size,
		ReleaseNotes:   strings.TrimSpace(req.ReleaseNotes),
		UploadedBy:     strings.TrimSpace(actor),
		Active:         true,
	}
	if releaseResult.Installer != nil && strings.TrimSpace(releaseResult.Installer.Path) != "" {
		installerName := sanitizeFileName(releaseResult.Installer.FileName)
		if installerName != "" {
			installerRelPath := filepath.Join(appItem.AppID, nextVersion, "installers", installerName)
			installerAbsPath := m.resolveFilesPath(installerRelPath)
			if err := copyManagedArtifact(releaseResult.Installer.Path, installerAbsPath); err != nil {
				m.rt.Logf("update-server: copiere installer esuata src=%s dst=%s err=%v", strings.TrimSpace(releaseResult.Installer.Path), installerAbsPath, err)
				return versionRecord{}, err
			}
			item.InstallerFileName = installerName
			item.InstallerFilePath = installerRelPath
			item.InstallerChecksumSHA256 = strings.TrimSpace(releaseResult.Installer.ChecksumSHA256)
			item.InstallerFileSize = releaseResult.Installer.Size
			m.rt.Logf("update-server: installer copiat la %s", installerAbsPath)
		}
	}
	saved, err := m.saveVersion(item)
	if err != nil {
		_ = os.Remove(absFilePath)
		if strings.TrimSpace(item.InstallerFilePath) != "" {
			_ = os.Remove(m.resolveFilesPath(item.InstallerFilePath))
		}
		return versionRecord{}, err
	}
	m.rt.Logf("update-server: release finalizat app_id=%s version=%s target=%s saved_version_id=%d", appItem.AppID, saved.Version, target, saved.ID)
	return saved, nil
}

func copyManagedArtifact(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return copyFileWithMode(src, dst, info.Mode())
}

func (m *Module) saveUpload(file multipart.File, header *multipart.FileHeader, appID int64, version string) (string, string, string, int64, error) {
	appItem, err := m.findApplicationByID(appID)
	if err != nil {
		return "", "", "", 0, err
	}
	fileName := sanitizeFileName(header.Filename)
	if fileName == "" {
		return "", "", "", 0, errors.New("invalid file name")
	}
	relPath := filepath.Join(appItem.AppID, version, fileName)
	absPath := m.resolveFilesPath(relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", "", "", 0, err
	}
	tmpPath := absPath + ".part"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", "", "", 0, err
	}
	defer out.Close()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hasher), file)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", "", "", 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", "", 0, err
	}
	if err := os.Rename(tmpPath, absPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", "", 0, err
	}
	return fileName, relPath, hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func (m *Module) publicSettings() map[string]interface{} {
	settings := m.rt.ModuleSettings(m.ID())
	wise := map[string]string{}
	if svc := m.wiseMED(); svc != nil {
		wise = svc.Settings()
	}
	return map[string]interface{}{
		"files_dir":            firstNonEmpty(asString(settings["files_dir"]), "./files"),
		"db_path":              firstNonEmpty(asString(settings["db_path"]), "./app-update-server.db"),
		"public_base_url":      strings.TrimSpace(asString(settings["public_base_url"])),
		"public_protocol":      firstNonEmpty(asString(settings["public_protocol"]), "http"),
		"public_host":          firstNonEmpty(asString(settings["public_host"]), "127.0.0.1"),
		"public_port":          firstNonEmpty(asString(settings["public_port"]), "19090"),
		"allowed_user_types":   firstNonEmpty(asString(settings["allowed_user_types"]), "1,9,10"),
		"cfg_wisemed_protocol": strings.TrimSpace(wise["cfg_wisemed_protocol"]),
		"cfg_wisemed_ip":       strings.TrimSpace(wise["cfg_wisemed_ip"]),
		"cfg_wisemed_port":     strings.TrimSpace(wise["cfg_wisemed_port"]),
		"cfg_wisemed_path":     strings.TrimSpace(wise["cfg_wisemed_path"]),
		"cfg_wisemed_key":      strings.TrimSpace(wise["cfg_wisemed_key"]),
	}
}

func (m *Module) effectivePublicBaseURL() string {
	settings := m.rt.ModuleSettings(m.ID())
	if base := strings.TrimSpace(asString(settings["public_base_url"])); base != "" {
		return strings.TrimRight(base, "/")
	}
	protocol := firstNonEmpty(asString(settings["public_protocol"]), "http")
	host := firstNonEmpty(asString(settings["public_host"]), "127.0.0.1")
	port := firstNonEmpty(asString(settings["public_port"]), "19090")
	return fmt.Sprintf("%s://%s:%s", protocol, host, port)
}

func (m *Module) readersRepoRoot() (string, error) {
	candidates := []string{
		strings.TrimSpace(m.rt.ConfigDir()),
		filepath.Dir(strings.TrimSpace(m.rt.ConfigPath())),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exePath))
	}
	for _, candidate := range candidates {
		root, ok := locateReadersRepoRoot(candidate)
		if ok {
			return root, nil
		}
	}
	return "", errors.New("could not infer readersv3 repository root from update-server config path, working directory, or executable location")
}

func isReadersRepoRoot(dir string) bool {
	for _, rel := range []string{"apps", "output", filepath.Join("tools", "releasectl", "main.go")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return false
		}
	}
	return true
}

func locateReadersRepoRoot(start string) (string, bool) {
	start = strings.TrimSpace(start)
	if start == "" {
		return "", false
	}
	if !filepath.IsAbs(start) {
		abs, err := filepath.Abs(start)
		if err != nil {
			return "", false
		}
		start = abs
	}
	for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
		if isReadersRepoRoot(dir) {
			return dir, true
		}
		nested := filepath.Join(dir, "readersv3")
		if isReadersRepoRoot(nested) {
			return nested, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func discoverManagedReader(repoRoot, updateAppID string) (managedReaderRelease, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "apps"))
	if err != nil {
		return managedReaderRelease{}, err
	}
	updateAppID = canonicalManagedReaderUpdateAppID(updateAppID)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configPath := filepath.Join(repoRoot, "apps", entry.Name(), "deployments", "config.install.yaml")
		blob, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var raw map[string]interface{}
		if err := yaml.Unmarshal(blob, &raw); err != nil {
			continue
		}
		modules, _ := raw["modules"].(map[string]interface{})
		appUpdates, _ := modules["app-updates"].(map[string]interface{})
		if appUpdates == nil {
			continue
		}
		appID := canonicalManagedReaderUpdateAppID(asString(appUpdates["app_id"]))
		if appID != updateAppID {
			continue
		}
		return managedReaderRelease{
			SourceAppID: entry.Name(),
			UpdateAppID: appID,
		}, nil
	}
	return managedReaderRelease{}, fmt.Errorf("no readersv3 source mapping found for update app_id %q", updateAppID)
}

func canonicalManagedReaderUpdateAppID(appID string) string {
	appID = strings.TrimSpace(appID)
	if canonical, ok := managedReaderUpdateAppIDAliases[appID]; ok {
		return canonical
	}
	return appID
}

func nextReleaseVersion(versions []versionRecord) string {
	best := ""
	for _, item := range versions {
		candidate := appupdates.NormalizeVersion(item.Version)
		if appupdates.CompareVersions(candidate, best) > 0 {
			best = candidate
		}
	}
	return bumpPatchVersion(best)
}

func bumpPatchVersion(version string) string {
	version = appupdates.NormalizeVersion(version)
	if version == "" {
		return "1.0.0"
	}
	parts := strings.Split(version, ".")
	last := len(parts) - 1
	if last < 0 {
		return "1.0.0"
	}
	n, err := strconv.Atoi(parts[last])
	if err != nil {
		return version + ".1"
	}
	parts[last] = strconv.Itoa(n + 1)
	return strings.Join(parts, ".")
}

func isSupportedReleaseTarget(osName, arch string) bool {
	switch osName {
	case "windows", "linux", "darwin":
	default:
		return false
	}
	switch arch {
	case "amd64", "arm64":
		return true
	default:
		return false
	}
}

func (m *Module) runReleaseCtlBuild(repoRoot, sourceAppID, target, version string) error {
	goVersion, err := goToolVersion(repoRoot)
	if err != nil {
		return err
	}
	cacheSuffix := sanitizeFileName(goVersion)
	if cacheSuffix == "" {
		cacheSuffix = "unknown-go-version"
	}
	tmpCache := filepath.Join(os.TempDir(), "wisemed-releasectl-gocache-"+cacheSuffix)
	if err := os.MkdirAll(tmpCache, 0o755); err != nil {
		return err
	}
	output, err := runReleaseCtlBuildCommand(repoRoot, sourceAppID, target, version, tmpCache)
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(output)
	if strings.Contains(text, "does not match go tool version") {
		_ = os.RemoveAll(tmpCache)
		if err := os.MkdirAll(tmpCache, 0o755); err != nil {
			return err
		}
		output, err = runReleaseCtlBuildCommand(repoRoot, sourceAppID, target, version, tmpCache)
		if err == nil {
			return nil
		}
		text = strings.TrimSpace(output)
	}
	return fmt.Errorf("releasectl build failed: %s", text)
}

func runReleaseCtlBuildCommand(repoRoot, sourceAppID, target, version, goCache string) (string, error) {
	cmd := exec.Command("go", "run", "-a", "./tools/releasectl", "build", "--app", sourceAppID, "--target", target, "--version", version)
	cmd.Dir = repoRoot
	cmd.Env = append(cleanGoCommandEnv(), "GOCACHE="+goCache)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

type releaseCtlArtifact struct {
	Kind           string `json:"kind"`
	FileName       string `json:"fileName"`
	Path           string `json:"path"`
	ChecksumSHA256 string `json:"checksumSHA256"`
	Size           int64  `json:"size"`
}

type releaseCtlResult struct {
	Update    releaseCtlArtifact  `json:"update"`
	Installer *releaseCtlArtifact `json:"installer,omitempty"`
}

func (m *Module) runReleaseCtlRelease(repoRoot, sourceAppID, target, version string) (releaseCtlResult, error) {
	goVersion, err := goToolVersion(repoRoot)
	if err != nil {
		return releaseCtlResult{}, err
	}
	cacheSuffix := sanitizeFileName(goVersion)
	if cacheSuffix == "" {
		cacheSuffix = "unknown-go-version"
	}
	tmpCache := filepath.Join(os.TempDir(), "wisemed-releasectl-gocache-"+cacheSuffix)
	if err := os.MkdirAll(tmpCache, 0o755); err != nil {
		return releaseCtlResult{}, err
	}
	m.rt.Logf("update-server: compilez release source_app=%s version=%s target=%s repo=%s gocache=%s", sourceAppID, version, target, repoRoot, tmpCache)
	output, err := runReleaseCtlReleaseCommand(repoRoot, sourceAppID, target, version, tmpCache, m.effectivePublicBaseURL())
	if err == nil {
		m.rt.Logf("update-server: releasectl a terminat source_app=%s version=%s target=%s", sourceAppID, version, target)
		return output, nil
	}
	if strings.Contains(err.Error(), "does not match go tool version") {
		m.rt.Logf("update-server: curat cache-ul Go si reiau compilarea pentru source_app=%s version=%s target=%s", sourceAppID, version, target)
		_ = os.RemoveAll(tmpCache)
		if err := os.MkdirAll(tmpCache, 0o755); err != nil {
			return releaseCtlResult{}, err
		}
		output, retryErr := runReleaseCtlReleaseCommand(repoRoot, sourceAppID, target, version, tmpCache, m.effectivePublicBaseURL())
		if retryErr == nil {
			m.rt.Logf("update-server: releasectl a terminat dupa retry source_app=%s version=%s target=%s", sourceAppID, version, target)
			return output, nil
		}
		return releaseCtlResult{}, retryErr
	}
	return releaseCtlResult{}, err
}

func runReleaseCtlReleaseCommand(repoRoot, sourceAppID, target, version, goCache, appUpdatesBaseURL string) (releaseCtlResult, error) {
	args := []string{"run", "-a", "./tools/releasectl", "release", "--json", "--app", sourceAppID, "--target", target, "--version", version}
	if strings.TrimSpace(appUpdatesBaseURL) != "" {
		args = append(args, "--app-updates-base-url", strings.TrimSpace(appUpdatesBaseURL))
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(cleanGoCommandEnv(), "GOCACHE="+goCache)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		return releaseCtlResult{}, fmt.Errorf("releasectl release failed: %s", text)
	}
	var payload struct {
		Update    releaseCtlArtifact  `json:"update"`
		Installer *releaseCtlArtifact `json:"installer"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return releaseCtlResult{}, fmt.Errorf("invalid releasectl release json: %w", err)
	}
	return releaseCtlResult{
		Update:    payload.Update,
		Installer: payload.Installer,
	}, nil
}

func goToolVersion(repoRoot string) (string, error) {
	cmd := exec.Command("go", "env", "GOVERSION")
	cmd.Dir = repoRoot
	cmd.Env = cleanGoCommandEnv()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go env GOVERSION failed: %s", strings.TrimSpace(output.String()))
	}
	return strings.TrimSpace(output.String()), nil
}

func copyFileWithMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func cleanGoCommandEnv() []string {
	keep := map[string]bool{
		"PATH":     true,
		"HOME":     true,
		"TMPDIR":   true,
		"TMP":      true,
		"TEMP":     true,
		"USER":     true,
		"LOGNAME":  true,
		"SHELL":    true,
		"LANG":     true,
		"LC_ALL":   true,
		"LC_CTYPE": true,
		"TERM":     true,
	}
	out := make([]string, 0, len(keep))
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if keep[parts[0]] {
			out = append(out, item)
		}
	}
	return out
}

func zipRuntimePayload(runtimeDir, archivePath string) (string, int64, error) {
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return "", 0, err
	}
	tmpPath := archivePath + ".part"
	file, err := os.Create(tmpPath)
	if err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	writer := zip.NewWriter(io.MultiWriter(file, hasher))
	var walkErr error
	for _, entry := range entries {
		name := entry.Name()
		if name == "manifest.json" {
			continue
		}
		fullPath := filepath.Join(runtimeDir, name)
		if entry.IsDir() {
			walkErr = filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(runtimeDir, path)
				if err != nil {
					return err
				}
				return addFileToZip(writer, path, rel)
			})
		} else {
			walkErr = addFileToZip(writer, fullPath, name)
		}
		if walkErr != nil {
			break
		}
	}
	if err := writer.Close(); err != nil && walkErr == nil {
		walkErr = err
	}
	if err := file.Close(); err != nil && walkErr == nil {
		walkErr = err
	}
	if walkErr != nil {
		_ = os.Remove(tmpPath)
		return "", 0, walkErr
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

func addFileToZip(writer *zip.Writer, path, rel string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(rel)
	header.Method = zip.Deflate
	entryWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(entryWriter, fh)
	return err
}

func (m *Module) persistSettings(next map[string]interface{}) error {
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
	section, _ := modules[m.ID()].(map[string]interface{})
	if section == nil {
		section = map[string]interface{}{}
	}
	for key, value := range next {
		if strings.HasPrefix(key, "cfg_wisemed_") {
			continue
		}
		section[key] = value
	}
	modules[m.ID()] = section
	if wise, ok := modules["wisemed-api"].(map[string]interface{}); ok {
		for _, key := range []string{"cfg_wisemed_protocol", "cfg_wisemed_ip", "cfg_wisemed_port", "cfg_wisemed_path", "cfg_wisemed_key"} {
			if value := strings.TrimSpace(asString(next[key])); value != "" {
				wise[key] = value
			}
		}
		modules["wisemed-api"] = wise
	}
	updated, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, updated, 0o644)
}

func (m *Module) wiseMED() wiseMedAPIService {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	svc, _ := service.(wiseMedAPIService)
	return svc
}

func (m *Module) wiseMEDReady() bool {
	wise := m.wiseMED()
	return wise != nil && wise.IsConfigured()
}

func (m *Module) loginWiseMED(username, password string) (wisemedapi.LoginResponse, error) {
	wise := m.wiseMED()
	if wise == nil {
		return wisemedapi.LoginResponse{}, errors.New("wisemed api service unavailable")
	}
	settings := wise.Settings()
	if strings.TrimSpace(settings["cfg_wisemed_key"]) == "" {
		return wisemedapi.LoginResponse{}, errors.New("cfg_wisemed_key is not configured")
	}
	body := map[string]interface{}{
		"username": username,
		"password": password,
	}
	raw := map[string]interface{}{}
	if err := doJSONWithSettings(settings, http.MethodPut, "/administrative/login", body, &raw); err != nil {
		return wisemedapi.LoginResponse{}, err
	}
	blob, _ := json.Marshal(raw)
	var out wisemedapi.LoginResponse
	if err := json.Unmarshal(blob, &out); err == nil && strings.TrimSpace(out.LoginToken) != "" {
		return out, nil
	}
	return parseLoginResponse(raw)
}

func (m *Module) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return session{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.sessions[cookie.Value]
	if !ok || item.ExpiresAt.Before(time.Now()) {
		return session{}, false
	}
	return item, true
}

func (m *Module) isAdminUserType(userType int) bool {
	raw := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["allowed_user_types"]), "-1")
	for _, token := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(token))
		if err == nil && n == userType {
			return true
		}
	}
	return false
}

func (m *Module) verifyPublicJWT(r *http.Request, scope string) (appupdates.Claims, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-WiseMED-Update-JWT"))
	}
	wise := m.wiseMED()
	if wise == nil {
		return appupdates.Claims{}, errors.New("wisemed api service unavailable")
	}
	secret := strings.TrimSpace(wise.Settings()["cfg_wisemed_key"])
	if secret == "" {
		return appupdates.Claims{}, errors.New("cfg_wisemed_key is not configured")
	}
	claims, err := appupdates.VerifyJWT(secret, token)
	if err != nil {
		return appupdates.Claims{}, err
	}
	if scope != "" && !containsScope(claims.Scopes, scope) {
		return appupdates.Claims{}, fmt.Errorf("token scope %q is missing", scope)
	}
	return claims, nil
}

func (m *Module) publicDownloadURL(r *http.Request, item versionRecord, token string, artifactKind string) string {
	if strings.EqualFold(strings.TrimSpace(artifactKind), "installer") && strings.TrimSpace(item.InstallerFilePath) != "" {
		querySuffix := "?token=" + url.QueryEscape(token) + "&artifact=installer"
		base := strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["public_base_url"]))
		if base != "" {
			return strings.TrimRight(base, "/") + "/api/public/download/" + strconv.FormatInt(item.ID, 10) + querySuffix
		}
		protocol := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_protocol"]), "http")
		host := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_host"]), "127.0.0.1")
		port := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_port"]), "19090")
		if strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["public_host"])) == "" && r != nil {
			host = strings.TrimSpace(strings.Split(r.Host, ":")[0])
		}
		return fmt.Sprintf("%s://%s:%s/api/public/download/%d%s", protocol, host, port, item.ID, querySuffix)
	}
	artifact := m.preferredPublicArtifact(item)
	if strings.TrimSpace(artifact.DownloadURL) != "" {
		return artifact.DownloadURL
	}
	querySuffix := "?token=" + url.QueryEscape(token)
	if strings.EqualFold(strings.TrimSpace(artifactKind), "installer") && strings.TrimSpace(item.InstallerFilePath) != "" {
		querySuffix += "&artifact=installer"
	}
	base := strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["public_base_url"]))
	if base != "" {
		return strings.TrimRight(base, "/") + "/api/public/download/" + strconv.FormatInt(item.ID, 10) + querySuffix
	}
	protocol := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_protocol"]), "http")
	host := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_host"]), "127.0.0.1")
	port := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["public_port"]), "19090")
	if strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["public_host"])) == "" && r != nil {
		host = strings.TrimSpace(strings.Split(r.Host, ":")[0])
	}
	return fmt.Sprintf("%s://%s:%s/api/public/download/%d%s", protocol, host, port, item.ID, querySuffix)
}

func (m *Module) publicDownloadFileName(item versionRecord, _ string) string {
	return m.preferredPublicArtifact(item).FileName
}

func (m *Module) publicDownloadChecksum(item versionRecord, _ string) string {
	return m.preferredPublicArtifact(item).ChecksumSHA256
}

func (m *Module) preferredPublicArtifact(item versionRecord) versionRecord {
	if !strings.EqualFold(strings.TrimSpace(item.TargetOS), "windows") {
		return item
	}
	if looksLikeZipArtifact(item.FileName, item.FilePath, item.DownloadURL) {
		return item
	}
	updateName, updatePath, updateChecksum, updateSize, ok := m.findManagedUpdateArchive(item.InstallerFilePath)
	if !ok {
		return item
	}
	item.DownloadURL = ""
	item.FileName = updateName
	item.FilePath = updatePath
	item.ChecksumSHA256 = updateChecksum
	item.FileSize = updateSize
	return item
}

func looksLikeZipArtifact(fileName, filePath, downloadURL string) bool {
	for _, value := range []string{filePath, fileName, downloadURL} {
		if strings.EqualFold(filepath.Ext(strings.TrimSpace(value)), ".zip") {
			return true
		}
	}
	return false
}

func (m *Module) resolveFilesPath(value string) string {
	base := firstNonEmpty(asString(m.rt.ModuleSettings(m.ID())["files_dir"]), "./files")
	base = resolvePath(m.rt.ConfigDir(), base)
	return resolvePath(base, value)
}

func resolvePath(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return base
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(base, value)
}

func pickBestVersion(items []versionRecord, channel, osName, arch string) (versionRecord, bool) {
	filtered := make([]versionRecord, 0, len(items))
	for _, item := range items {
		if !item.Active {
			continue
		}
		if !matchesTarget(item.Channel, channel) || !matchesTarget(item.TargetOS, osName) || !matchesTarget(item.TargetArch, arch) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return versionRecord{}, false
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return appupdates.CompareVersions(filtered[i].Version, filtered[j].Version) > 0
	})
	return filtered[0], true
}

func countActiveVersions(items []versionRecord) int {
	total := 0
	for _, item := range items {
		if item.Active {
			total++
		}
	}
	return total
}

func matchesTarget(rule, value string) bool {
	rule = strings.TrimSpace(strings.ToLower(rule))
	value = strings.TrimSpace(strings.ToLower(value))
	return rule == "" || value == "" || rule == value || rule == "*" || rule == "any" || rule == "all"
}

func removeEmptyParents(startDir, stopDir string) error {
	stopDir = filepath.Clean(stopDir)
	for dir := filepath.Clean(startDir); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if dir == stopDir {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	return nil
}

func (m *Module) issueDownloadToken(versionID int64, appID string) string {
	token := randomToken()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredDownloadTokensLocked()
	m.downloadTokens[token] = downloadToken{
		Token:     token,
		VersionID: versionID,
		AppID:     strings.TrimSpace(appID),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	return token
}

func (m *Module) consumeDownloadToken(token string, versionID int64) (downloadToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredDownloadTokensLocked()
	item, ok := m.downloadTokens[token]
	if !ok {
		return downloadToken{}, errors.New("invalid token")
	}
	if item.VersionID != versionID {
		delete(m.downloadTokens, token)
		return downloadToken{}, errors.New("token does not match requested version")
	}
	delete(m.downloadTokens, token)
	return item, nil
}

func (m *Module) cleanupExpiredDownloadTokensLocked() {
	now := time.Now()
	for key, item := range m.downloadTokens {
		if item.ExpiresAt.Before(now) {
			delete(m.downloadTokens, key)
		}
	}
}

func doJSONWithSettings(settings map[string]string, method, apiPath string, payload interface{}, out interface{}) error {
	target, err := makeURL(settings, apiPath)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		blob, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(blob))
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	token, err := createWiseMEDJWT(settings, "AppUpdateServer")
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", token)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	blob, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if text := strings.TrimSpace(string(blob)); text != "" {
			return errors.New(text)
		}
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}
	if out == nil || len(blob) == 0 {
		return nil
	}
	return json.Unmarshal(blob, out)
}

func makeURL(settings map[string]string, apiPath string) (string, error) {
	protocol := strings.TrimSpace(settings["cfg_wisemed_protocol"])
	host := strings.TrimSpace(settings["cfg_wisemed_ip"])
	port := strings.TrimSpace(settings["cfg_wisemed_port"])
	basePath := strings.TrimSpace(settings["cfg_wisemed_path"])
	if protocol == "" || host == "" || port == "" || basePath == "" {
		return "", errors.New("WiseMED API configuration is incomplete")
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return fmt.Sprintf("%s://%s:%s%s%s", protocol, host, port, strings.TrimRight(basePath, "/"), apiPath), nil
}

func createWiseMEDJWT(settings map[string]string, callerType string) (string, error) {
	secret := strings.TrimSpace(settings["cfg_wisemed_key"])
	if secret == "" {
		return "", errors.New("WiseMED API key is missing")
	}
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"caller_id":   "WM-Lab-Reader",
		"caller_type": callerType,
		"exp":         time.Now().Add(5 * time.Minute).Unix(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func parseLoginResponse(raw map[string]interface{}) (wisemedapi.LoginResponse, error) {
	raw = unwrapLoginEnvelope(raw)
	loginToken := firstNonEmpty(asString(raw["login_token"]), asString(raw["token"]), asString(raw["lt"]))
	out := wisemedapi.LoginResponse{
		Login:       strings.TrimSpace(asString(raw["login"])),
		FirstName:   strings.TrimSpace(asString(raw["first_name"])),
		LastName:    strings.TrimSpace(asString(raw["last_name"])),
		UserType:    intValue(raw["user_type"]),
		UserEmail:   strings.TrimSpace(asString(raw["user_email"])),
		LoginToken:  strings.TrimSpace(loginToken),
		UserPicture: strings.TrimSpace(asString(raw["user_picture"])),
	}
	if out.Login == "" && out.LoginToken == "" {
		return wisemedapi.LoginResponse{}, errors.New("unexpected login response")
	}
	return out, nil
}

func unwrapLoginEnvelope(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	for _, key := range []string{"data", "result", "item", "user", "payload"} {
		if nested, ok := raw[key].(map[string]interface{}); ok && len(nested) > 0 {
			if asString(nested["login"]) != "" || asString(nested["login_token"]) != "" || nested["user_type"] != nil {
				return nested
			}
		}
	}
	return raw
}

func randomToken() string {
	blob := make([]byte, 24)
	if _, err := rand.Read(blob); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(blob)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseBoolString(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	return false
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "..", "")
	if name == "." || name == "/" || name == string(filepath.Separator) {
		return ""
	}
	return name
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, raw)
	return t
}

func containsScope(items []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	for _, item := range items {
		if strings.TrimSpace(strings.ToLower(item)) == want {
			return true
		}
	}
	return false
}

func intValue(raw interface{}) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(typed))
		return n
	}
	return 0
}

func asString(raw interface{}) string {
	switch typed := raw.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		if raw == nil {
			return ""
		}
		return fmt.Sprint(raw)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func escapeHeader(value string) string {
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
