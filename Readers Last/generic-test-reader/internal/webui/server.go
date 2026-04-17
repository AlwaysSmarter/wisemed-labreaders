package webui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readerslast/generic-test-reader/internal/config"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/model"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/reader"
	"wisemed-labreaders/readerslast/generic-test-reader/internal/wisemedapi"
)

//go:embed ui/*
var uiAssets embed.FS

const sessionCookieName = "wmr_local_session"

type Server struct {
	cfg      *config.Config
	app      *reader.App
	httpSrv  *http.Server
	sessions map[string]session
	mu       sync.RWMutex
	helpDir  string
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

func New(cfg *config.Config, app *reader.App) (*Server, error) {
	helpDir := cfg.HelpDirPath()
	if err := ensureHelpContent(helpDir, cfg); err != nil {
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		app:      app,
		sessions: map[string]session{},
		helpDir:  helpDir,
	}
	mux := http.NewServeMux()
	mux.Handle("/", s.withNoCache(http.HandlerFunc(s.handleIndex)))
	mux.Handle("/app.js", s.withNoCache(http.HandlerFunc(s.handleStaticAsset("ui/app.js", "application/javascript; charset=utf-8"))))
	mux.Handle("/styles.css", s.withNoCache(http.HandlerFunc(s.handleStaticAsset("ui/styles.css", "text/css; charset=utf-8"))))
	mux.Handle("/api/session", s.withNoCache(http.HandlerFunc(s.handleSessionStatus)))
	mux.Handle("/api/preferences", s.withNoCache(http.HandlerFunc(s.handlePreferences)))
	mux.Handle("/api/preferences/language", s.withNoCache(http.HandlerFunc(s.handleLanguage)))
	mux.Handle("/api/session/login", s.withNoCache(http.HandlerFunc(s.handleLogin)))
	mux.Handle("/api/session/logout", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleLogout))))
	mux.Handle("/api/config", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleConfig))))
	mux.Handle("/api/config/", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleConfigSection))))
	mux.Handle("/api/status", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleStatus))))
	mux.Handle("/api/stats", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleStats))))
	mux.Handle("/api/stats/series", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleStatsSeries))))
	mux.Handle("/api/dashboard", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleDashboard))))
	mux.Handle("/api/logs", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleLogs))))
	mux.Handle("/api/orders", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrders))))
	mux.Handle("/api/order-analysis", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrderAnalysis))))
	mux.Handle("/api/order-analysis/", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrderAnalysisByID))))
	mux.Handle("/api/orders/rounds", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrderRounds))))
	mux.Handle("/api/orders/import", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrdersImport))))
	mux.Handle("/api/orders/export", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleOrdersExport))))
	mux.Handle("/api/results/default", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleDefaultResult))))
	mux.Handle("/api/analytes", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleAnalytes))))
	mux.Handle("/api/analytes/", s.withNoCache(s.requireSession(http.HandlerFunc(s.handleAnalyteByTag))))
	mux.Handle("/help/", s.withNoCache(s.requireSession(http.StripPrefix("/help/", http.FileServer(http.Dir(helpDir))))))
	s.httpSrv = &http.Server{
		Addr:              cfg.LocalHTTP.Address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func (s *Server) withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := s.currentSession(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.handleStaticAsset("ui/index.html", "text/html; charset=utf-8")(w, r)
}

func (s *Server) handleStaticAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blob, err := fs.ReadFile(uiAssets, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(blob)
	}
}

func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	sess, ok := s.currentSession(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"authenticated": ok,
		"session":       sess,
		"preferences": map[string]interface{}{
			"language": s.cfg.LocalHTTP.Language,
		},
		"permissions": map[string]interface{}{
			"can_view_logs": ok && canViewLogs(sess),
		},
		"reader": map[string]interface{}{
			"id":            s.cfg.Reader.ID,
			"label":         s.cfg.Reader.Label,
			"analyzer_name": s.cfg.Reader.AnalyzerName,
		},
	})
}

func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"preferences": map[string]interface{}{
			"language": s.cfg.LocalHTTP.Language,
		},
	})
}

func (s *Server) handleLanguage(w http.ResponseWriter, r *http.Request) {
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
	switch lang {
	case "ro", "en":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "unsupported language"})
		return
	}
	s.cfg.LocalHTTP.Language = lang
	if err := s.cfg.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"preferences": map[string]interface{}{
			"language": s.cfg.LocalHTTP.Language,
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		MedicalUnitID int    `json:"medical_unit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "username and password are required"})
		return
	}
	if req.MedicalUnitID == 0 {
		req.MedicalUnitID = 1
	}
	client := wisemedapi.New(s.cfg)
	loginResp, err := client.AdministrativeLogin(req.Username, req.Password, req.MedicalUnitID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	sess, err := s.createSession(loginResp, req.MedicalUnitID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
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

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": s.app.StatusSnapshot()})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.app.ConfigSnapshot()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "config": cfg})
	case http.MethodPut:
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		if err := s.app.UpdateConfig(patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		cfg, err := s.app.ConfigSnapshot()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "config": cfg})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleConfigSection(w http.ResponseWriter, r *http.Request) {
	section := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/config/"))
	if section == "" || section == r.URL.Path {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "config section is required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.app.ConfigSection(section)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "section": section, "config": value})
	case http.MethodPut:
		var payload interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		if err := s.app.UpdateConfigSection(section, payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		value, err := s.app.ConfigSection(section)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "section": section, "config": value})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	sess, ok := s.currentSession(r)
	if !ok || !canViewLogs(sess) {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "log access denied"})
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := s.app.ListLogs(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "logs": items})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	data, err := s.app.DashboardSnapshot(14)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "today": data["today"], "series": data["series"]})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
	data, err := s.app.StatsForDate(orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": data})
}

func (s *Server) handleStatsSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	limit := 14
	if raw := strings.TrimSpace(r.URL.Query().Get("series_limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	data, err := s.app.StatsSeries(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": data})
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		requestedRoundNo := 0
		orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
		includeAnalysis := parseBoolQuery(r.URL.Query().Get("include_analysis"))
		if raw := strings.TrimSpace(r.URL.Query().Get("round_no")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				requestedRoundNo = n
			}
		}
		if orderDate == "" {
			orderDate = time.Now().Format("2006-01-02")
		}
		rounds, err := s.app.ListRoundNumbers(orderDate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		effectiveRoundNo := requestedRoundNo
		if effectiveRoundNo <= 0 && len(rounds) > 0 {
			effectiveRoundNo = rounds[len(rounds)-1]
		}
		if len(rounds) == 0 {
			rounds = []int{1}
		}
		if includeAnalysis {
			items, err := s.app.ListOrderBundles(effectiveRoundNo, orderDate)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":               true,
				"orders":           items,
				"rounds":           rounds,
				"order_date":       orderDate,
				"round_no":         effectiveRoundNo,
				"include_analysis": true,
			})
			return
		}
		items, err := s.app.ListOrders(effectiveRoundNo, orderDate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":               true,
			"orders":           items,
			"rounds":           rounds,
			"order_date":       orderDate,
			"round_no":         effectiveRoundNo,
			"include_analysis": false,
		})
	case http.MethodPost:
		var order model.Order
		if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item, err := s.app.UpsertOrder(order)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order": item})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleOrderAnalysis(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		orderID, err := parseRequiredInt64Query(r, "order_id")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		items, err := s.app.ListOrderAnalyses(orderID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_analyses": items})
	case http.MethodPost:
		var item model.OrderAnalysis
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		saved, err := s.app.SaveOrderAnalysis(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_analysis": saved})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleOrderAnalysisByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.URL.Path, "/api/order-analysis/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := s.app.GetOrderAnalysis(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_analysis": item})
	case http.MethodPut:
		var item model.OrderAnalysis
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		saved, err := s.app.SaveOrderAnalysis(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "order_analysis": saved})
	case http.MethodDelete:
		if err := s.app.DeleteOrderAnalysis(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleOrderRounds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		orderDate := strings.TrimSpace(r.URL.Query().Get("order_date"))
		if orderDate == "" {
			orderDate = time.Now().Format("2006-01-02")
		}
		rounds, err := s.app.ListRoundNumbers(orderDate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		currentRoundNo := 1
		if len(rounds) > 0 {
			currentRoundNo = rounds[len(rounds)-1]
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"order_date": orderDate,
			"round_no":   currentRoundNo,
			"rounds":     rounds,
		})
		return
	case http.MethodPost:
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		OrderDate string `json:"order_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	orderDate := strings.TrimSpace(req.OrderDate)
	if orderDate == "" {
		orderDate = time.Now().Format("2006-01-02")
	}
	roundNo, err := s.app.CreateNextRound(orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	rounds, err := s.app.ListRoundNumbers(orderDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"order_date": orderDate,
		"round_no":   roundNo,
		"rounds":     rounds,
	})
}

func (s *Server) handleOrdersImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	if s.cfg.Comm.Type != config.CommTypeFile {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "manual file import is available only for file communication"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid multipart upload"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "file is required"})
		return
	}
	defer file.Close()
	orderDate := strings.TrimSpace(r.FormValue("order_date"))
	if err := os.MkdirAll(s.cfg.Comm.File.ImportDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	expectedExt := configuredPatternExt(s.cfg.Comm.File.Pattern)
	originalName := filepath.Base(header.Filename)
	uploadedExt := strings.ToLower(filepath.Ext(originalName))
	if uploadedExt != "" && uploadedExt != expectedExt {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": fmt.Sprintf("file extension %s is not compatible with %s", uploadedExt, s.cfg.Comm.File.Pattern)})
		return
	}
	baseName := strings.TrimSuffix(originalName, filepath.Ext(originalName))
	name := fmt.Sprintf("manual-%d-%s%s", time.Now().UnixNano(), sanitizeUploadBase(baseName), expectedExt)
	path := filepath.Join(s.cfg.Comm.File.ImportDir, name)
	out, err := os.Create(path)
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
	summary, err := s.app.ImportFileNow(path, orderDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"path":       path,
		"file_name":  name,
		"imported":   summary.Imported,
		"warnings":   summary.Warnings,
		"protocol":   summary.Protocol,
		"order_date": summary.OrderDate,
	})
}

func (s *Server) handleOrdersExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	if s.cfg.Comm.Type != config.CommTypeFile {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "manual file export is available only for file communication"})
		return
	}
	var req struct {
		OrderIDs  []int64 `json:"order_ids"`
		OrderDate string  `json:"order_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
		return
	}
	path, rows, err := s.app.ExportOrdersCSV(req.OrderIDs, strings.TrimSpace(req.OrderDate))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "path": path, "rows": rows})
}

func configuredPatternExt(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ".csv"
	}
	ext := filepath.Ext(strings.ReplaceAll(pattern, "*", "x"))
	if ext == "" || ext == "." {
		return ".csv"
	}
	return strings.ToLower(ext)
}

func sanitizeUploadBase(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "import"
	}
	name = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(name)
	return name
}

func parseBoolQuery(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw == "1" || raw == "true" || raw == "yes"
}

func parseRequiredInt64Query(r *http.Request, key string) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return v, nil
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

func (s *Server) handleDefaultResult(w http.ResponseWriter, r *http.Request) {
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
	if req.OrderAnalysisID <= 0 || req.ResultID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "order_analysis_id and result_id are required"})
		return
	}
	if err := s.app.SetDefaultResult(req.OrderAnalysisID, req.ResultID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleAnalytes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.app.ListAnalytes()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analytes": items})
	case http.MethodPost:
		var item model.Analyte
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		id, err := s.app.SaveAnalyte(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id, "tag": item.Tag})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) handleAnalyteByTag(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/analytes/"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "missing analyte id"})
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid analyte id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := s.app.GetAnalyteByID(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "analyte": item})
	case http.MethodPut:
		var item model.Analyte
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		item.ID = id
		savedID, err := s.app.SaveAnalyte(item)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": savedID, "tag": item.Tag})
	case http.MethodDelete:
		if err := s.app.DeleteAnalyte(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (s *Server) createSession(loginResp *wisemedapi.LoginResponse, medicalUnitID int) (session, error) {
	id, err := randomID()
	if err != nil {
		return session{}, err
	}
	sess := session{
		ID:            id,
		Username:      loginResp.Login,
		MedicalUnitID: medicalUnitID,
		UserType:      loginResp.UserType,
		FirstName:     strings.TrimSpace(loginResp.FirstName),
		LastName:      strings.TrimSpace(loginResp.LastName),
		UserEmail:     strings.TrimSpace(loginResp.UserEmail),
		UserPicture:   strings.TrimSpace(loginResp.UserPicture),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(12 * time.Hour),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func canViewLogs(sess session) bool {
	return sess.UserType == -1 || sess.UserType == 0
}

func (s *Server) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return session{}, false
	}
	s.mu.RLock()
	sess, ok := s.sessions[cookie.Value]
	s.mu.RUnlock()
	if !ok || time.Now().UTC().After(sess.ExpiresAt) {
		if ok {
			s.mu.Lock()
			delete(s.sessions, cookie.Value)
			s.mu.Unlock()
		}
		return session{}, false
	}
	return sess, true
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

func ensureHelpContent(helpDir string, cfg *config.Config) error {
	assetsDir := filepath.Join(helpDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return err
	}
	indexPath := filepath.Join(helpDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := fmt.Sprintf(defaultHelpHTML, cfg.Reader.AnalyzerName, cfg.Reader.Label, cfg.Reader.AnalyzerCode)
	return os.WriteFile(indexPath, []byte(content), 0o644)
}

const defaultHelpHTML = `<!doctype html>
<html lang="ro">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Help %s</title>
  <style>
    body{font-family:Georgia,serif;background:#f4efe6;color:#231f18;margin:0;padding:32px}
    .sheet{max-width:960px;margin:0 auto;background:#fffdf9;border:1px solid #d9cfbf;border-radius:24px;padding:32px;box-shadow:0 20px 60px rgba(64,41,10,.08)}
    h1,h2{margin:0 0 12px;color:#2f2418}
    p,li{line-height:1.6}
    .callout{background:#f7f0e2;border-left:4px solid #a4682a;padding:16px 18px;border-radius:12px;margin:20px 0}
    .media{margin-top:24px;padding:24px;border:1px dashed #c6b292;border-radius:18px;background:#fbf7f0}
    code{background:#f2eadc;padding:2px 6px;border-radius:6px}
  </style>
</head>
<body>
  <div class="sheet">
    <h1>%s</h1>
    <p>Pagina locală de help pentru readerul <strong>%s</strong>.</p>
    <div class="callout">
      Completează aici instrucțiunile de configurare ale analizorului, exemple de fișiere, pași de troubleshooting și capturi de ecran.
    </div>
    <h2>Structură recomandată</h2>
    <ul>
      <li>Descrierea modului de lucru al analizorului</li>
      <li>Setările de comunicare necesare</li>
      <li>Formatul fișierelor importate/exportate</li>
      <li>Erori frecvente și rezolvare</li>
    </ul>
    <div class="media">
      <p>Poți pune aici imagini, PDF-uri sau video în directorul <code>assets/</code> din help și să le referențiezi din acest HTML.</p>
      <p>Exemplu: <code>&lt;img src="assets/setup.png"&gt;</code> sau <code>&lt;video controls src="assets/demo.mp4"&gt;&lt;/video&gt;</code></p>
    </div>
  </div>
</body>
</html>
`
