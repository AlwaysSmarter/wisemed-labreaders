package signingpad

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/module"
	"wisemed-labreaders/readersv3/shared/bindguard"
)

//go:embed ui/*
var uiAssets embed.FS

type Module struct {
	rt      module.Runtime
	server  *http.Server
	mu      sync.RWMutex
	started time.Time
}

type captureRequest struct {
	RequestID    string `json:"request_id"`
	PatientID    string `json:"patient_id"`
	FileID       string `json:"file_id"`
	SampleCode   string `json:"sample_code"`
	SpecimenCode string `json:"specimen_code"`
	Message      string `json:"message"`
	Reason       string `json:"reason"`
	DeviceType   string `json:"device_type"`
	SDKMode      string `json:"sdk_mode"`
	DeviceHost   string `json:"device_host"`
	DevicePort   string `json:"device_port"`
}

type helperResponse struct {
	OK          bool   `json:"ok"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	ImageBase64 string `json:"imageBase64"`
	MimeType    string `json:"mimeType"`
	ImagePath   string `json:"imagePath"`
	Timestamp   string `json:"timestamp"`
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "signing-pad" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.started = time.Now()
	m.rt.RegisterService("signing-pad", m)
	m.rt.Handle("/", m.withCORS(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/health", m.withCORS(http.HandlerFunc(m.handleIndex)))
	m.rt.Handle("/signing-pad/app.js", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/app.js", "application/javascript; charset=utf-8"))))
	m.rt.Handle("/signing-pad/styles.css", m.withCORS(http.HandlerFunc(m.handleStaticAsset("ui/styles.css", "text/css; charset=utf-8"))))
	m.rt.Handle("/api/signing-pad/health", m.withCORS(http.HandlerFunc(m.handleHealth)))
	m.rt.Handle("/api/signing-pad/capture", m.withCORS(http.HandlerFunc(m.handleCapture)))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	addr := strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["address"]))
	if addr == "" {
		addr = "127.0.0.1:19110"
	}
	for {
		m.server = &http.Server{
			Addr:              addr,
			Handler:           m.rt.Mux(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if m.server != nil {
				_ = m.server.Shutdown(shutdownCtx)
			}
		}()
		useTLS := parseBool(asString(m.rt.ModuleSettings(m.ID())["tls"]))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if bindguard.IsAddressInUse(err) {
				nextAddr, _, handleErr := bindguard.HandleAddressInUse(m.rt.ConfigPath(), addr, map[string]interface{}{
					"modules.signing-pad.address": addr,
				}, m.rt.Logf)
				if handleErr != nil {
					return err
				}
				msg := fmt.Sprintf("signing-pad utility nu se poate binda la %s: %v. Propun %s si scriu aceasta valoare in config.yaml", addr, err, nextAddr)
				fmt.Println(msg)
				m.rt.Logf(msg)
				addr = nextAddr
				continue
			}
			return err
		}
		if useTLS {
			certFile, keyFile, err := ensureLocalHTTPSMaterial(m.rt.ConfigDir(), addr)
			if err != nil {
				_ = listener.Close()
				return err
			}
			m.rt.Logf("signing-pad utility listening on https://%s", addr)
			if err := m.server.ServeTLS(listener, certFile, keyFile); err != nil && err != http.ErrServerClosed {
				return err
			}
			return nil
		}
		m.rt.Logf("signing-pad utility listening on http://%s", addr)
		if err := m.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func (m *Module) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/health" {
		http.NotFound(w, r)
		return
	}
	m.handleStaticAsset("ui/index.html", "text/html; charset=utf-8")(w, r)
}

func (m *Module) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := m.invokeHelper(r.Context(), map[string]string{
		"action":     "health",
		"deviceType": strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["device_type"])),
		"sdkMode":    strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["sdk_mode"])),
	})
	if err != nil {
		m.writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"status":  "degraded",
			"message": err.Error(),
		})
		return
	}
	m.writeJSON(w, http.StatusOK, result)
}

func (m *Module) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req captureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	settings := m.rt.ModuleSettings(m.ID())
	payload := map[string]string{
		"action":       "capture",
		"requestId":    strings.TrimSpace(req.RequestID),
		"patientId":    strings.TrimSpace(req.PatientID),
		"fileId":       strings.TrimSpace(req.FileID),
		"sampleCode":   strings.TrimSpace(req.SampleCode),
		"specimenCode": strings.TrimSpace(req.SpecimenCode),
		"message":      firstNonEmpty(strings.TrimSpace(req.Message), strings.TrimSpace(req.Reason), "Please sign"),
		"deviceType":   firstNonEmpty(strings.TrimSpace(req.DeviceType), strings.TrimSpace(asString(settings["device_type"]))),
		"sdkMode":      firstNonEmpty(strings.TrimSpace(req.SDKMode), strings.TrimSpace(asString(settings["sdk_mode"]))),
		"deviceHost":   firstNonEmpty(strings.TrimSpace(req.DeviceHost), strings.TrimSpace(asString(settings["device_host"]))),
		"devicePort":   firstNonEmpty(strings.TrimSpace(req.DevicePort), strings.TrimSpace(asString(settings["device_port"]))),
	}
	result, err := m.invokeHelper(r.Context(), payload)
	if err != nil {
		m.writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"ok":      false,
			"status":  "helper_error",
			"message": err.Error(),
		})
		return
	}
	resp := helperResponse{}
	blob, _ := json.Marshal(result)
	_ = json.Unmarshal(blob, &resp)
	if resp.OK && strings.TrimSpace(resp.ImageBase64) != "" {
		savedPath, err := m.persistSignature(req, resp)
		if err != nil {
			m.rt.Logf("signing-pad image save failed: %v", err)
			m.writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"ok":      false,
				"status":  "image_save_error",
				"message": err.Error(),
			})
			return
		}
		resp.ImagePath = savedPath
		m.rt.Logf("signing-pad image saved path=%s request_id=%s file_id=%s sample_code=%s specimen_code=%s", savedPath, req.RequestID, req.FileID, req.SampleCode, req.SpecimenCode)
	}
	m.writeJSON(w, http.StatusOK, resp)
}

func (m *Module) persistSignature(req captureRequest, resp helperResponse) (string, error) {
	raw := strings.TrimSpace(resp.ImageBase64)
	if raw == "" {
		return "", errors.New("empty image payload")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("decode image base64: %w", err)
	}
	root := m.rt.ResolvePath(firstNonEmpty(strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["storage_dir"])), "./signatures"))
	dirName := buildFolderName(req.FileID, req.SampleCode, req.SpecimenCode)
	targetDir := filepath.Join(root, dirName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create signature dir: %w", err)
	}
	fileName := fmt.Sprintf("signature-%s.png", time.Now().Format("20060102-150405"))
	targetPath := filepath.Join(targetDir, fileName)
	if err := os.WriteFile(targetPath, decoded, 0o644); err != nil {
		return "", fmt.Errorf("write signature image: %w", err)
	}
	return targetPath, nil
}

func buildFolderName(fileID, sampleCode, specimenCode string) string {
	parts := []string{
		sanitizePathPart(fileID),
		sanitizePathPart(sampleCode),
		sanitizePathPart(specimenCode),
	}
	return strings.Join(parts, "__")
}

func sanitizePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "missing"
	}
	value = strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(value)
	return value
}

func (m *Module) invokeHelper(ctx context.Context, payload map[string]string) (map[string]interface{}, error) {
	settings := m.rt.ModuleSettings(m.ID())
	command := strings.TrimSpace(asString(settings["helper_command"]))
	if command == "" {
		return nil, errors.New("helper_command is not configured")
	}
	args := stringList(settings["helper_args"])
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = m.rt.ConfigDir()
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(requestBody)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("helper failed: %s", msg)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		return nil, errors.New("helper returned empty response")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("invalid helper response: %w", err)
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		m.rt.Logf("signing-pad helper stderr: %s", msg)
	}
	return result, nil
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
		allowed := strings.TrimSpace(asString(m.rt.ModuleSettings(m.ID())["cors_allowed_origins"]))
		if allowed == "" {
			allowed = "*"
		}
		origin := ""
		if r != nil {
			origin = strings.TrimSpace(r.Header.Get("Origin"))
		}
		if allowed == "*" {
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
		} else {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
		}
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Cache-Control", "no-store")
		if r != nil && r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Module) writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func asString(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func stringList(v interface{}) []string {
	switch raw := v.(type) {
	case []string:
		return append([]string(nil), raw...)
	case []interface{}:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			s := strings.TrimSpace(asString(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		s := strings.TrimSpace(asString(v))
		if s == "" {
			return nil
		}
		return []string{s}
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

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func intSetting(settings map[string]interface{}, key string, fallback int) int {
	raw := strings.TrimSpace(asString(settings[key]))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
