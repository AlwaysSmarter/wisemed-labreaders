package labnovationld560

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

type storageService interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type statusSnapshot struct {
	ConnectedClients int       `json:"connected_clients"`
	LastProtocol     string    `json:"last_protocol"`
	LastMessageAt    time.Time `json:"last_message_at"`
	LastError        string    `json:"last_error"`
	LastImportCount  int       `json:"last_import_count"`
}

type Module struct {
	rt module.Runtime

	mu        sync.Mutex
	clients   int
	status    statusSnapshot
	restartCh chan struct{}
}

func New() module.Module { return &Module{} }

func (m *Module) ID() string { return "protocol-labnovation-ld560" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	m.restartCh = make(chan struct{}, 1)
	rt.AddMenu(module.MenuEntry{ID: "protocol-labnovation-ld560", Group: "admin", Label: "Protocol Labnovation LD-560", Path: "/settings/protocol/labnovation-ld560", Order: 45})
	rt.Handle("/settings/protocol/labnovation-ld560", http.HandlerFunc(m.handleSettingsPage))
	rt.Handle("/api/protocol/labnovation-ld560/settings", http.HandlerFunc(m.handleSettingsAPI))
	rt.Handle("/api/protocol/labnovation-ld560/status", http.HandlerFunc(m.handleStatusAPI))
	rt.Handle("/api/protocol/meta", http.HandlerFunc(m.handleMeta))
	rt.RegisterService("labnovation-ld560-status", m)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	for {
		cfg := m.communicationConfig()
		if !strings.EqualFold(cfg.CommType, "tcpip") {
			m.rt.Logf("labnovation-ld560 protocol idle: comm_type=%q", cfg.CommType)
			select {
			case <-ctx.Done():
				return nil
			case <-m.restartCh:
				m.rt.Logf("labnovation-ld560 communication reconfiguration requested while idle")
				continue
			}
		}

		runCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func(current commConfig) {
			if strings.EqualFold(current.TCPMode, "client") {
				errCh <- m.runTCPClient(runCtx, current)
				return
			}
			errCh <- m.runTCPServer(runCtx, current)
		}(cfg)

		select {
		case <-ctx.Done():
			cancel()
			<-errCh
			return nil
		case <-m.restartCh:
			m.rt.Logf("labnovation-ld560 reinitializing communication: comm_type=%s protocol=%s mode=%s", cfg.CommType, cfg.ProtocolMode, cfg.TCPMode)
			cancel()
			<-errCh
			continue
		case err := <-errCh:
			cancel()
			if err == nil {
				select {
				case <-ctx.Done():
					return nil
				case <-m.restartCh:
					m.rt.Logf("labnovation-ld560 communication loop restarted after graceful stop")
					continue
				case <-time.After(1 * time.Second):
					continue
				}
			}
			m.setError(err)
			m.rt.Logf("labnovation-ld560 communication error: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-m.restartCh:
				m.rt.Logf("labnovation-ld560 communication restart requested after error")
				continue
			case <-time.After(3 * time.Second):
				m.rt.Logf("labnovation-ld560 retrying communication after error")
				continue
			}
		}
	}
}

func (m *Module) runTCPServer(ctx context.Context, cfg commConfig) error {
	addr := net.JoinHostPort(cfg.ListenHost, cfg.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		m.rt.Logf("labnovation-ld560 failed to listen on %s: %v", addr, err)
		return err
	}
	defer ln.Close()
	m.rt.Logf("labnovation-ld560 listening as tcp server on %s using protocol=%s", addr, cfg.ProtocolMode)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			m.rt.Logf("labnovation-ld560 accept failed on %s: %v", addr, err)
			return err
		}
		m.rt.Logf("labnovation-ld560 client connected from %s", conn.RemoteAddr())
		go m.handleConn(ctx, conn, cfg.ProtocolMode, "server")
	}
}

func (m *Module) runTCPClient(ctx context.Context, cfg commConfig) error {
	target := net.JoinHostPort(cfg.RemoteHost, cfg.RemotePort)
	m.rt.Logf("labnovation-ld560 connecting as tcp client to %s using protocol=%s", target, cfg.ProtocolMode)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		conn, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			m.setError(err)
			m.rt.Logf("labnovation-ld560 tcp client connection to %s failed: %v", target, err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}
		m.rt.Logf("labnovation-ld560 tcp client connected to %s", target)
		m.handleConn(ctx, conn, cfg.ProtocolMode, "client")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
			m.rt.Logf("labnovation-ld560 tcp client reconnect loop scheduled for %s", target)
		}
	}
}

func (m *Module) ConnectedClients() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clients
}

func (m *Module) snapshot() statusSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Module) handleMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                true,
		"protocol":          "labnovation-ld560",
		"active_mode":       m.activeProtocol(),
		"supported_modes":   []string{"hl7", "simple"},
		"communication":     "tcpip",
		"connected_clients": m.ConnectedClients(),
	})
}

func (m *Module) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"status": m.snapshot(),
		"tcpip": map[string]string{
			"mode":        m.tcpMode(),
			"host":        m.listenHost(),
			"port":        m.listenPort(),
			"remote_host": m.remoteHost(),
			"remote_port": m.remotePort(),
		},
		"protocol": m.activeProtocol(),
	})
}

func (m *Module) handleSettingsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "settings": m.settingsPayload()})
	case http.MethodPut:
		before := m.communicationConfig()
		var req struct {
			ProtocolMode  string                 `json:"protocol_mode"`
			TCPMode       string                 `json:"tcp_mode"`
			TCPHost       string                 `json:"tcp_host"`
			TCPPort       string                 `json:"tcp_port"`
			TCPRemoteHost string                 `json:"tcp_remote_host"`
			TCPRemotePort string                 `json:"tcp_remote_port"`
			HL7           map[string]interface{} `json:"hl7"`
			Simple        map[string]interface{} `json:"simple"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid json body"})
			return
		}
		mode := normalizeProtocolMode(req.ProtocolMode)
		if mode == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "protocol_mode must be hl7 or simple"})
			return
		}
		tcpMode := normalizeTCPMode(req.TCPMode)
		if tcpMode == "" {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "tcp_mode must be server or client"})
			return
		}
		host := strings.TrimSpace(req.TCPHost)
		if host == "" {
			host = "0.0.0.0"
		}
		port := strings.TrimSpace(req.TCPPort)
		if _, err := strconv.Atoi(port); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "tcp_port must be numeric"})
			return
		}
		remoteHost := strings.TrimSpace(req.TCPRemoteHost)
		if remoteHost == "" {
			remoteHost = "127.0.0.1"
		}
		remotePort := strings.TrimSpace(req.TCPRemotePort)
		if remotePort == "" {
			remotePort = port
		}
		if _, err := strconv.Atoi(remotePort); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "tcp_remote_port must be numeric"})
			return
		}
		hl7 := normalizeMap(req.HL7)
		simple := normalizeMap(req.Simple)
		if err := config.Update(m.rt.ConfigPath(), map[string]interface{}{
			"analyzer.protocol":                         mode,
			"modules.transport-tcpip.mode":              tcpMode,
			"modules.transport-tcpip.host":              host,
			"modules.transport-tcpip.port":              port,
			"modules.transport-tcpip.remote_host":       remoteHost,
			"modules.transport-tcpip.remote_port":       remotePort,
			"modules.protocol-labnovation-ld560.hl7":    hl7,
			"modules.protocol-labnovation-ld560.simple": simple,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		after := m.communicationConfig()
		restartNeeded := before != after
		message := "Setarile au fost salvate."
		if restartNeeded {
			message = "Setarile au fost salvate. Comunicarea Labnovation LD-560 a fost reinitializata."
		}
		m.rt.Logf("labnovation-ld560 settings saved: protocol=%s mode=%s listen=%s:%s remote=%s:%s restart=%t", after.ProtocolMode, after.TCPMode, after.ListenHost, after.ListenPort, after.RemoteHost, after.RemotePort, restartNeeded)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"settings": m.settingsPayload(),
			"message":  message,
		})
		if restartNeeded {
			m.requestRestart()
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
	}
}

func (m *Module) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	payload := m.settingsPayload()
	blob, _ := json.MarshalIndent(payload, "", "  ")
	page := `<!doctype html>
<html lang="ro">
<head>
  <meta charset="utf-8">
  <title>Protocol Labnovation LD-560</title>
  <style>
    :root { color-scheme: light; --bg:#f5f1e8; --panel:#fffdfa; --ink:#1e2a2f; --muted:#617079; --line:#d8cdbd; --accent:#0d6b63; --danger:#a53333; }
    body { margin:0; font-family: Georgia, "Times New Roman", serif; background:linear-gradient(180deg,#efe5d5 0%,#f8f5ef 100%); color:var(--ink); }
    main { max-width: 1100px; margin: 32px auto; padding: 0 20px 40px; }
    .card { background:var(--panel); border:1px solid var(--line); border-radius:16px; padding:20px; box-shadow:0 12px 30px rgba(46,54,64,.08); }
    h1 { margin:0 0 8px; font-size:32px; }
    p { color:var(--muted); }
    label { display:block; font-weight:700; margin:16px 0 8px; }
    input, select, textarea, button { font:inherit; }
    input, select, textarea { width:100%; box-sizing:border-box; border:1px solid var(--line); border-radius:10px; padding:10px 12px; background:#fff; color:var(--ink); }
    textarea { min-height:220px; resize:vertical; font-family: "SFMono-Regular", Menlo, monospace; font-size:13px; }
    .grid { display:grid; gap:16px; grid-template-columns:repeat(auto-fit,minmax(220px,1fr)); }
    button { margin-top:18px; background:var(--accent); color:#fff; border:none; border-radius:999px; padding:12px 18px; cursor:pointer; }
    .status { margin-top:16px; padding:12px 14px; border-radius:10px; background:#eef7f5; color:#114942; display:none; }
    .status.error { background:#fdecec; color:var(--danger); }
    code { background:#f1ece3; padding:2px 6px; border-radius:6px; }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <h1>Protocol Labnovation LD-560</h1>
      <p>Selectorul dintre <code>hl7</code> si <code>simple</code> este salvat si in <code>config.yaml</code>. JSON-urile de mai jos sunt editabile complet din interfata pentru configurari avansate.</p>
      <div class="grid">
        <div>
          <label for="protocol_mode">Protocol activ</label>
          <select id="protocol_mode">
            <option value="hl7">HL7</option>
            <option value="simple">Simple</option>
          </select>
        </div>
        <div>
          <label for="tcp_mode">TCP mode</label>
          <select id="tcp_mode">
            <option value="server">Server</option>
            <option value="client">Client</option>
          </select>
        </div>
        <div>
          <label for="tcp_host">TCP host</label>
          <input id="tcp_host" />
        </div>
        <div>
          <label for="tcp_port">TCP port</label>
          <input id="tcp_port" />
        </div>
        <div>
          <label for="tcp_remote_host">TCP remote host</label>
          <input id="tcp_remote_host" />
        </div>
        <div>
          <label for="tcp_remote_port">TCP remote port</label>
          <input id="tcp_remote_port" />
        </div>
      </div>
      <label for="hl7_json">Configuratie HL7</label>
      <textarea id="hl7_json"></textarea>
      <label for="simple_json">Configuratie Simple</label>
      <textarea id="simple_json"></textarea>
      <button id="save">Salveaza</button>
      <div id="status" class="status"></div>
    </div>
    <script>
      const settings = ` + "`" + string(blob) + "`" + `;
      const data = JSON.parse(settings);
      document.getElementById('protocol_mode').value = data.protocol_mode || 'simple';
      document.getElementById('tcp_mode').value = data.tcp_mode || 'server';
      document.getElementById('tcp_host').value = data.tcp_host || '0.0.0.0';
      document.getElementById('tcp_port').value = data.tcp_port || '8000';
      document.getElementById('tcp_remote_host').value = data.tcp_remote_host || '127.0.0.1';
      document.getElementById('tcp_remote_port').value = data.tcp_remote_port || data.tcp_port || '8000';
      document.getElementById('hl7_json').value = JSON.stringify(data.hl7 || {}, null, 2);
      document.getElementById('simple_json').value = JSON.stringify(data.simple || {}, null, 2);
      document.getElementById('save').addEventListener('click', async () => {
        const status = document.getElementById('status');
        status.className = 'status';
        try {
          const payload = {
            protocol_mode: document.getElementById('protocol_mode').value,
            tcp_mode: document.getElementById('tcp_mode').value,
            tcp_host: document.getElementById('tcp_host').value,
            tcp_port: document.getElementById('tcp_port').value,
            tcp_remote_host: document.getElementById('tcp_remote_host').value,
            tcp_remote_port: document.getElementById('tcp_remote_port').value,
            hl7: JSON.parse(document.getElementById('hl7_json').value || '{}'),
            simple: JSON.parse(document.getElementById('simple_json').value || '{}')
          };
          const resp = await fetch('/api/protocol/labnovation-ld560/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
          const body = await resp.json();
          if (!resp.ok || !body.ok) throw new Error(body.error || 'save failed');
          status.style.display = 'block';
          status.textContent = body.message || 'Setari salvate.';
        } catch (err) {
          status.style.display = 'block';
          status.className = 'status error';
          status.textContent = err.message || String(err);
        }
      });
    </script>
  </main>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

func (m *Module) settingsPayload() map[string]interface{} {
	settings := m.rt.ModuleSettings(m.ID())
	payload := map[string]interface{}{
		"protocol_mode":   m.activeProtocol(),
		"tcp_mode":        m.tcpMode(),
		"tcp_host":        m.listenHost(),
		"tcp_port":        m.listenPort(),
		"tcp_remote_host": m.remoteHost(),
		"tcp_remote_port": m.remotePort(),
		"hl7":             defaultHL7Settings(),
		"simple":          defaultSimpleSettings(),
	}
	if raw, ok := settings["hl7"].(map[string]interface{}); ok && raw != nil {
		payload["hl7"] = mergeSettings(defaultHL7Settings(), raw)
	}
	if raw, ok := settings["simple"].(map[string]interface{}); ok && raw != nil {
		payload["simple"] = mergeSettings(defaultSimpleSettings(), raw)
	}
	return payload
}

func (m *Module) handleConn(ctx context.Context, conn net.Conn, protocol, role string) {
	defer conn.Close()
	m.changeClients(1)
	defer m.changeClients(-1)
	remoteAddr := conn.RemoteAddr().String()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer m.rt.Logf("labnovation-ld560 %s connection closed: remote=%s", role, remoteAddr)

	_ = conn.SetReadDeadline(time.Time{})
	reader := bufio.NewReader(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var raw []byte
		var err error
		switch protocol {
		case "hl7":
			raw, err = readHL7Message(reader, hl7SettingsFromMap(m.settingsPayload()["hl7"].(map[string]interface{})))
		default:
			raw, err = readSimpleMessage(reader)
		}
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed) {
				return
			}
			if err.Error() == "EOF" {
				m.rt.Logf("labnovation-ld560 %s connection closed by peer: remote=%s", role, remoteAddr)
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "closed") {
				return
			}
			m.setError(err)
			m.rt.Logf("labnovation-ld560 %s read error: remote=%s protocol=%s err=%v", role, remoteAddr, protocol, err)
			return
		}
		imported, parseErr := m.importMessage(protocol, raw)
		if parseErr != nil {
			m.setError(parseErr)
			m.rt.Logf("labnovation-ld560 import failed: remote=%s protocol=%s err=%v", remoteAddr, protocol, parseErr)
			continue
		}
		m.setImported(protocol, imported)
		m.rt.Logf("labnovation-ld560 imported %d result(s): remote=%s protocol=%s", imported, remoteAddr, protocol)
	}
}

func (m *Module) importMessage(protocol string, raw []byte) (int, error) {
	store := m.storage()
	if store == nil {
		return 0, errors.New("storage service unavailable")
	}
	switch protocol {
	case "hl7":
		msgs, err := parseHL7Results(raw, hl7SettingsFromMap(m.settingsPayload()["hl7"].(map[string]interface{})))
		if err != nil {
			return 0, err
		}
		return m.persistMessages(store, msgs, "hl7")
	default:
		msgs, err := parseSimpleResults(raw, simpleSettingsFromMap(m.settingsPayload()["simple"].(map[string]interface{})))
		if err != nil {
			return 0, err
		}
		return m.persistMessages(store, msgs, "simple")
	}
}

func (m *Module) persistMessages(store storageService, messages []parsedMessage, source string) (int, error) {
	imported := 0
	roundCache := map[string]int{}
	for _, item := range messages {
		for _, result := range item.Results {
			analyteTag := strings.TrimSpace(result.AnalyteTag)
			if analyteTag == "" {
				continue
			}
			if err := m.ensureAnalyte(store, result); err != nil {
				return imported, err
			}
			if item.IsQC {
				record, err := store.UpsertQCRecord(coremodel.QCRecord{
					RoundNo:      1,
					RunDate:      item.RunDate,
					ControlLabel: firstNonEmpty(item.SampleID, item.SampleNo, "QC"),
					ControlLevel: firstNonEmpty(item.ControlLevel, "QC"),
					LotNo:        firstNonEmpty(item.SampleID, item.SampleNo, "-"),
					FileID:       item.FileID,
					Status:       "completed",
					SourceFile:   "tcp:" + source,
				})
				if err != nil {
					return imported, err
				}
				if _, err := store.UpsertQCAnalysis(coremodel.QCAnalysis{
					QCRecordID:  record.ID,
					AnalyteTag:  analyteTag,
					AnalyteName: result.AnalyteName,
					Status:      "completed",
					ResultValue: result.ResultValue,
					RawValue:    result.RawValue,
					Interpreted: result.Interpreted,
					Unit:        result.Unit,
					LotNo:       firstNonEmpty(item.SampleID, item.SampleNo, "-"),
					SourceFile:  "tcp:" + source,
					Flags:       result.Flags,
				}); err != nil {
					return imported, err
				}
				imported++
				continue
			}
			roundNo := roundCache[item.RunDate]
			if roundNo == 0 {
				var err error
				roundNo, err = store.CurrentRoundNo(item.RunDate)
				if err != nil {
					return imported, err
				}
				roundCache[item.RunDate] = roundNo
			}
			_, _, _, err := store.RecordImportedResult(item.RunDate, roundNo, coremodel.ImportedRecord{
				SampleID:     item.SampleID,
				FileID:       item.FileID,
				PatientID:    item.PatientID,
				PatientName:  item.PatientName,
				AnalyteTag:   analyteTag,
				AnalyteName:  result.AnalyteName,
				ResultValue:  result.ResultValue,
				RawValue:     result.RawValue,
				Interpreted:  result.Interpreted,
				Flags:        result.Flags,
				Unit:         result.Unit,
				RackNo:       atoi(item.RackNo),
				RackPosition: atoi(item.RackPosition),
				SampleNo:     atoi(item.SampleNo),
				Meta: map[string]interface{}{
					"protocol": source,
				},
			}, "tcp:"+source)
			if err != nil {
				return imported, err
			}
			imported++
		}
	}
	return imported, nil
}

func (m *Module) ensureAnalyte(store storageService, result parsedResult) error {
	items, err := store.ListAnalytes()
	if err == nil {
		for _, item := range items {
			if strings.EqualFold(strings.TrimSpace(item.Tag), strings.TrimSpace(result.AnalyteTag)) {
				return nil
			}
		}
	}
	_, err = store.SaveAnalyte(coremodel.Analyte{
		Active:            true,
		Tag:               result.AnalyteTag,
		Code:              result.AnalyteTag,
		Name:              firstNonEmpty(result.AnalyteName, result.AnalyteTag),
		Description:       "Auto-generated from Labnovation LD-560 imports",
		ResultType:        "numeric",
		ResultFormatting:  "raw",
		ResultWeighting:   1,
		ResultMeasureUnit: result.Unit,
		ProtocolOptions:   normalizeMap(result.ProtocolOptions),
	})
	return err
}

func (m *Module) storage() storageService {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(storageService)
	return store
}

func (m *Module) activeProtocol() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		if mode := normalizeProtocolMode(cfg.Analyzer.Protocol); mode != "" {
			return mode
		}
	}
	if service, ok := m.rt.Service("analyzer-config"); ok {
		if raw, ok := service.(map[string]interface{}); ok {
			if mode := normalizeProtocolMode(asString(raw["protocol"])); mode != "" {
				return mode
			}
		}
	}
	return "simple"
}

func (m *Module) commType() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		if strings.TrimSpace(cfg.Analyzer.CommType) != "" {
			return strings.TrimSpace(cfg.Analyzer.CommType)
		}
	}
	if service, ok := m.rt.Service("analyzer-config"); ok {
		if raw, ok := service.(map[string]interface{}); ok {
			return strings.TrimSpace(asString(raw["comm_type"]))
		}
	}
	return ""
}

func (m *Module) listenHost() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		host := strings.TrimSpace(asString(cfg.ModuleSettings("transport-tcpip")["host"]))
		if host != "" {
			return host
		}
	}
	host := strings.TrimSpace(asString(m.rt.ModuleSettings("transport-tcpip")["host"]))
	if host == "" {
		return "0.0.0.0"
	}
	return host
}

func (m *Module) listenPort() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		port := strings.TrimSpace(asString(cfg.ModuleSettings("transport-tcpip")["port"]))
		if port != "" {
			return port
		}
	}
	port := strings.TrimSpace(asString(m.rt.ModuleSettings("transport-tcpip")["port"]))
	if port == "" {
		return "8000"
	}
	return port
}

func (m *Module) tcpMode() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		mode := strings.TrimSpace(asString(cfg.ModuleSettings("transport-tcpip")["mode"]))
		if mode != "" {
			return mode
		}
	}
	mode := strings.TrimSpace(asString(m.rt.ModuleSettings("transport-tcpip")["mode"]))
	if mode == "" {
		return "server"
	}
	return mode
}

func (m *Module) remoteHost() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		host := strings.TrimSpace(asString(cfg.ModuleSettings("transport-tcpip")["remote_host"]))
		if host != "" {
			return host
		}
	}
	host := strings.TrimSpace(asString(m.rt.ModuleSettings("transport-tcpip")["remote_host"]))
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

func (m *Module) remotePort() string {
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		port := strings.TrimSpace(asString(cfg.ModuleSettings("transport-tcpip")["remote_port"]))
		if port != "" {
			return port
		}
	}
	port := strings.TrimSpace(asString(m.rt.ModuleSettings("transport-tcpip")["remote_port"]))
	if port == "" {
		return m.listenPort()
	}
	return port
}

func (m *Module) changeClients(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients += delta
	if m.clients < 0 {
		m.clients = 0
	}
	m.status.ConnectedClients = m.clients
}

func (m *Module) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.LastError = err.Error()
}

func (m *Module) requestRestart() {
	select {
	case m.restartCh <- struct{}{}:
	default:
		select {
		case <-m.restartCh:
		default:
		}
		m.restartCh <- struct{}{}
	}
}

type commConfig struct {
	CommType     string
	ProtocolMode string
	TCPMode      string
	ListenHost   string
	ListenPort   string
	RemoteHost   string
	RemotePort   string
}

func (m *Module) communicationConfig() commConfig {
	return commConfig{
		CommType:     firstNonEmpty(strings.TrimSpace(m.commType()), ""),
		ProtocolMode: firstNonEmpty(normalizeProtocolMode(m.activeProtocol()), "simple"),
		TCPMode:      firstNonEmpty(normalizeTCPMode(m.tcpMode()), "server"),
		ListenHost:   firstNonEmpty(strings.TrimSpace(m.listenHost()), "0.0.0.0"),
		ListenPort:   firstNonEmpty(strings.TrimSpace(m.listenPort()), "8000"),
		RemoteHost:   firstNonEmpty(strings.TrimSpace(m.remoteHost()), "127.0.0.1"),
		RemotePort:   firstNonEmpty(strings.TrimSpace(m.remotePort()), "8000"),
	}
}

func (m *Module) setImported(protocol string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.LastProtocol = protocol
	m.status.LastImportCount = count
	m.status.LastMessageAt = time.Now()
	m.status.LastError = ""
}

func normalizeMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mergeSettings(base, override map[string]interface{}) map[string]interface{} {
	out := normalizeMap(base)
	keys := make([]string, 0, len(override))
	for key := range override {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := override[key]
		if current, ok := out[key].(map[string]interface{}); ok {
			if next, ok := value.(map[string]interface{}); ok {
				out[key] = mergeSettings(current, next)
				continue
			}
		}
		out[key] = value
	}
	return out
}

func normalizeProtocolMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hl7":
		return "hl7"
	case "simple", "labnovation-simple", "ld560-simple":
		return "simple"
	default:
		return ""
	}
}

func normalizeTCPMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "client":
		return "client"
	case "server", "":
		return "server"
	default:
		return ""
	}
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
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
	switch t := value.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
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

func escapeHTMLText(value string) string {
	return html.EscapeString(value)
}
