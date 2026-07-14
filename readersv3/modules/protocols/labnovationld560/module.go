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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/modules/protocols/fileimportbase"
	"wisemed-labreaders/readersv3/shared/bindguard"
	"wisemed-labreaders/readersv3/shared/commtrace"
	"wisemed-labreaders/readersv3/shared/debugreplay"
)

type storageService interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ReapplyOrderTransformations(orderIDs []int64) error
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
	ReapplyQCTransformations(recordIDs []int64) error
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type wiseMedSyncService interface {
	SetupComplete() bool
	EnsureEquipmentOnline(reader map[string]interface{}) (map[string]interface{}, error)
}

type auditLogger interface {
	AppendAuditLog(level, actor, eventType, message string, meta map[string]interface{}) error
}

type analyzerActivityTracker interface {
	Connected(delta int)
	Packet(direction, transport string)
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
	rt.RegisterService("debug-replay-runner", m)
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
		if bindguard.IsAddressInUse(err) {
			nextAddr, _, handleErr := bindguard.HandleAddressInUse(m.rt.ConfigPath(), addr, map[string]interface{}{
				"modules.transport-tcpip.host": cfg.ListenHost,
				"modules.transport-tcpip.port": cfg.ListenPort,
			}, m.rt.Logf)
			if handleErr == nil {
				msg := fmt.Sprintf("labnovation-ld560 nu se poate binda la %s: %v. Propun %s si scriu aceasta valoare in config.yaml", addr, err, nextAddr)
				fmt.Println(msg)
				m.rt.Logf(msg)
				host, port, splitErr := net.SplitHostPort(nextAddr)
				if splitErr == nil {
					cfg.ListenHost = host
					cfg.ListenPort = port
					return m.runTCPServer(ctx, cfg)
				}
			}
		}
		m.rt.Logf("labnovation-ld560 failed to listen on %s: %v", addr, err)
		m.appendAuditLog("error", "transport-tcpip", fmt.Sprintf("labnovation-ld560 server failed to start on %s", addr), map[string]interface{}{
			"mode":     "server",
			"address":  addr,
			"host":     cfg.ListenHost,
			"port":     cfg.ListenPort,
			"protocol": cfg.ProtocolMode,
			"error":    err.Error(),
		})
		return err
	}
	defer ln.Close()
	m.rt.Logf("labnovation-ld560 listening as tcp server on %s using protocol=%s", addr, cfg.ProtocolMode)
	m.appendAuditLog("info", "transport-tcpip", fmt.Sprintf("labnovation-ld560 tcp server started on %s", addr), map[string]interface{}{
		"mode":     "server",
		"address":  addr,
		"host":     cfg.ListenHost,
		"port":     cfg.ListenPort,
		"protocol": cfg.ProtocolMode,
	})
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
	m.appendAuditLog("info", "transport-tcpip", fmt.Sprintf("labnovation-ld560 tcp client connecting to %s", target), map[string]interface{}{
		"mode":        "client",
		"address":     target,
		"remote_host": cfg.RemoteHost,
		"remote_port": cfg.RemotePort,
		"protocol":    cfg.ProtocolMode,
	})
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

func (m *Module) LastMessageAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status.LastMessageAt
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
		var req struct {
			ProtocolMode  string                 `json:"protocol_mode"`
			TCPMode       string                 `json:"tcp_mode"`
			TCPHost       string                 `json:"tcp_host"`
			TCPPort       string                 `json:"tcp_port"`
			TCPRemoteHost string                 `json:"tcp_remote_host"`
			TCPRemotePort string                 `json:"tcp_remote_port"`
			ImageMode     string                 `json:"image_mode"`
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
		simple["image_mode"] = normalizeImageMode(req.ImageMode)
		if err := config.Update(m.rt.ConfigPath(), map[string]interface{}{
			"analyzer.protocol":                             mode,
			"modules.transport-tcpip.mode":                  tcpMode,
			"modules.transport-tcpip.host":                  host,
			"modules.transport-tcpip.port":                  port,
			"modules.transport-tcpip.remote_host":           remoteHost,
			"modules.transport-tcpip.remote_port":           remotePort,
			"modules.protocol-labnovation-ld560.image_mode": normalizeImageMode(req.ImageMode),
			"modules.protocol-labnovation-ld560.hl7":        hl7,
			"modules.protocol-labnovation-ld560.simple":     simple,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		after := m.communicationConfig()
		restartNeeded := true
		message := "Setarile au fost salvate. Comunicarea Labnovation LD-560 a fost reinitializata."
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
      <label for="image_mode">Image mode</label>
      <select id="image_mode">
        <option value="no_image">1 - Fara imagine</option>
        <option value="bitmap">2 - Format bitmap</option>
        <option value="base64">3 - Format base64</option>
      </select>
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
      document.getElementById('image_mode').value = data.image_mode || ((data.simple || {}).image_mode) || 'no_image';
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
            image_mode: document.getElementById('image_mode').value,
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

func (m *Module) RunDebugReplay(ctx context.Context, scriptName string, steps []debugreplay.Step) (debugreplay.Result, error) {
	cfg := m.communicationConfig()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return debugreplay.Result{}, err
	}
	defer ln.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			serverErrCh <- acceptErr
			return
		}
		m.handleConn(runCtx, conn, cfg.ProtocolMode, "debug")
		serverErrCh <- nil
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return debugreplay.Result{}, err
	}
	defer client.Close()

	tcpClient, _ := client.(*net.TCPConn)
	result := debugreplay.Result{
		Runner:     m.ID(),
		ScriptName: scriptName,
		Passed:     true,
		StartedAt:  time.Now().UTC(),
		Steps:      make([]debugreplay.StepResult, 0, len(steps)),
	}

	for _, step := range steps {
		stepStarted := time.Now()
		stepResult := debugreplay.StepResult{
			Index:          step.Index,
			Line:           step.Line,
			Terminator:     step.Terminator,
			InputPreview:   debugreplay.DescribePayload(step.Input),
			ExpectedOutput: debugreplay.DescribePayload(step.ExpectedOutput),
			Passed:         true,
		}
		if step.Terminator == debugreplay.TerminatorEOF {
			if _, err := m.importMessage(cfg.ProtocolMode, step.Input); err != nil {
				stepResult.Passed = false
				stepResult.Error = err.Error()
			}
		} else if len(step.Input) > 0 {
			if _, err := client.Write(step.Input); err != nil {
				stepResult.Passed = false
				stepResult.Error = err.Error()
			}
		}
		if stepResult.Passed {
			switch step.Terminator {
			case debugreplay.TerminatorOUT:
				actual, readErr := readDebugReplayOutput(client, 2*time.Second, 250*time.Millisecond)
				stepResult.ActualOutput = debugreplay.DescribePayload(actual)
				if readErr != nil {
					stepResult.Passed = false
					stepResult.Error = readErr.Error()
				} else if string(actual) != string(step.ExpectedOutput) {
					stepResult.Passed = false
					stepResult.Error = "output mismatch"
				}
			case debugreplay.TerminatorEOT:
				actual, _ := readDebugReplayOutput(client, 250*time.Millisecond, 150*time.Millisecond)
				stepResult.ActualOutput = debugreplay.DescribePayload(actual)
				if len(actual) > 0 {
					stepResult.Passed = false
					stepResult.Error = "unexpected output"
				}
			case debugreplay.TerminatorEOF:
				if tcpClient != nil {
					_ = tcpClient.CloseWrite()
				}
			}
		}
		stepResult.DurationMS = time.Since(stepStarted).Milliseconds()
		if !stepResult.Passed {
			result.Passed = false
		}
		result.Steps = append(result.Steps, stepResult)
		if !stepResult.Passed {
			break
		}
	}

	cancel()
	_ = client.Close()
	select {
	case serverErr := <-serverErrCh:
		if serverErr != nil && !errors.Is(serverErr, net.ErrClosed) && !strings.Contains(strings.ToLower(serverErr.Error()), "closed") {
			return result, serverErr
		}
	case <-time.After(500 * time.Millisecond):
	}
	result.FinishedAt = time.Now().UTC()
	return result, nil
}

func readDebugReplayOutput(conn net.Conn, startTimeout, quietWindow time.Duration) ([]byte, error) {
	buf := make([]byte, 256)
	payload := []byte{}
	deadline := time.Now().Add(startTimeout)
	for {
		_ = conn.SetReadDeadline(deadline)
		n, err := conn.Read(buf)
		if n > 0 {
			payload = append(payload, buf[:n]...)
			deadline = time.Now().Add(quietWindow)
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if len(payload) == 0 {
					return nil, errors.New("timeout waiting for output")
				}
				return payload, nil
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				if len(payload) == 0 {
					return nil, errors.New("timeout waiting for output")
				}
				return payload, nil
			}
			return payload, err
		}
	}
}

func (m *Module) settingsPayload() map[string]interface{} {
	settings := m.rt.ModuleSettings(m.ID())
	nestedImageModeConfigured := false
	payload := map[string]interface{}{
		"protocol_mode":   m.activeProtocol(),
		"tcp_mode":        m.tcpMode(),
		"tcp_host":        m.listenHost(),
		"tcp_port":        m.listenPort(),
		"tcp_remote_host": m.remoteHost(),
		"tcp_remote_port": m.remotePort(),
		"hl7":             defaultHL7Settings(),
		"simple":          defaultSimpleSettings(),
		"image_mode":      "no_image",
	}
	if cfg, err := config.Load(m.rt.ConfigPath()); err == nil && cfg != nil {
		settings = cfg.ModuleSettings(m.ID())
	}
	if raw, ok := settings["hl7"].(map[string]interface{}); ok && raw != nil {
		payload["hl7"] = mergeSettings(defaultHL7Settings(), raw)
	}
	if raw, ok := settings["simple"].(map[string]interface{}); ok && raw != nil {
		_, nestedImageModeConfigured = raw["image_mode"]
		payload["simple"] = mergeSettings(defaultSimpleSettings(), raw)
	}
	if simple, ok := payload["simple"].(map[string]interface{}); ok {
		legacy := normalizeImageMode(asString(settings["image_mode"]))
		if !nestedImageModeConfigured && legacy != "" {
			simple["image_mode"] = legacy
		}
		payload["image_mode"] = normalizeImageMode(asString(simple["image_mode"]))
	}
	return payload
}

func (m *Module) handleConn(ctx context.Context, conn net.Conn, protocol, role string) {
	defer conn.Close()
	m.changeClients(1)
	m.markAnalyzerConnected(1)
	defer m.changeClients(-1)
	defer m.markAnalyzerConnected(-1)
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
		m.logWire("in", "tcp", fmt.Sprintf("remote=%s protocol=%s role=%s bytes=%d", remoteAddr, protocol, role, len(raw)), raw)
		m.markAnalyzerPacket("tcp")
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
		msgs, err := parseHL7Results(raw, hl7SettingsFromMap(m.settingsPayload()["hl7"].(map[string]interface{})), m.logIgnored)
		if err != nil {
			return 0, err
		}
		m.logParsedMessages(protocol, msgs)
		return m.persistMessages(store, msgs, "hl7")
	default:
		msgs, err := parseSimpleResults(raw, simpleSettingsFromMap(m.settingsPayload()["simple"].(map[string]interface{})), m.logIgnored)
		if err != nil {
			return 0, err
		}
		m.logParsedMessages(protocol, msgs)
		return m.persistMessages(store, msgs, "simple")
	}
}

func (m *Module) persistMessages(store storageService, messages []parsedMessage, source string) (int, error) {
	imported := 0
	roundCache := map[string]int{}
	autoSaveTargets := map[string]*fileimportbase.AutoSaveTarget{}
	qcRecordIDs := []int64{}
	imageMode := m.simpleImageMode()
	items, err := store.ListAnalytes()
	if err != nil {
		return imported, err
	}
	knownAnalytes := make(map[string]coremodel.Analyte, len(items))
	for _, item := range items {
		knownAnalytes[strings.TrimSpace(item.Tag)] = item
	}
	analytesChanged := false
	m.logf(4, "labnovation-ld560 persist start source=%s messages=%d image_mode=%s", source, len(messages), imageMode)
	for _, item := range messages {
		imagePath := ""
		imageFormat := ""
		imageEncoding := ""
		if item.image == nil && imageMode != "no_image" {
			m.rt.Logf("labnovation-ld560 image not parsed file_id=%s image_mode=%s", firstNonEmpty(item.FileID, "-"), imageMode)
		}
		if item.image != nil {
			imageFormat = firstNonEmpty(item.image.format, "bin")
			imageEncoding = firstNonEmpty(item.image.encoding, "bitmap")
			m.rt.Logf("labnovation-ld560 image received file_id=%s image_name=%s encoding=%s bytes=%d", firstNonEmpty(item.FileID, "-"), firstNonEmpty(item.image.name, "-"), imageEncoding, len(item.image.data))
			if savedPath, err := m.saveMessageImage(item, *item.image); err != nil {
				m.rt.Logf("labnovation-ld560 image save failed file_id=%s image_name=%s err=%v", firstNonEmpty(item.FileID, "-"), firstNonEmpty(item.image.name, "-"), err)
			} else {
				imagePath = savedPath
				m.rt.Logf("labnovation-ld560 image saved file_id=%s path=%s", firstNonEmpty(item.FileID, "-"), savedPath)
			}
		}
		for _, result := range item.Results {
			analyteTag := strings.TrimSpace(result.AnalyteTag)
			if analyteTag == "" {
				m.logIgnored("result", "empty analyte tag after parsing", map[string]interface{}{
					"source":      source,
					"sample_id":   item.SampleID,
					"file_id":     item.FileID,
					"analyteName": result.AnalyteName,
					"raw_value":   result.RawValue,
				})
				continue
			}
			changed, err := m.ensureAnalyte(store, knownAnalytes, result)
			if err != nil {
				return imported, err
			}
			analytesChanged = analytesChanged || changed
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
				qcRecordIDs = append(qcRecordIDs, record.ID)
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
			order, _, _, err := store.RecordImportedResult(item.RunDate, roundNo, coremodel.ImportedRecord{
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
					"protocol":                   source,
					"labnovation_image_path":     imagePath,
					"labnovation_image_format":   imageFormat,
					"labnovation_image_encoding": imageEncoding,
				},
			}, "tcp:"+source)
			if err != nil {
				return imported, err
			}
			fileimportbase.CollectAutoSaveTarget(autoSaveTargets, item.RunDate, roundNo, order.ID)
			imported++
		}
	}
	orderIDs := []int64{}
	for _, target := range fileimportbase.FlattenAutoSaveTargets(autoSaveTargets) {
		orderIDs = append(orderIDs, target.OrderIDs...)
	}
	if err := store.ReapplyOrderTransformations(orderIDs); err != nil {
		m.rt.Logf("labnovation-ld560 transformation warning: %v", err)
	}
	if err := store.ReapplyQCTransformations(qcRecordIDs); err != nil {
		m.rt.Logf("labnovation-ld560 qc transformation warning: %v", err)
	}
	if analytesChanged {
		if err := m.syncAnalytesToWiseMED(); err != nil {
			m.rt.Logf("labnovation-ld560 analyte sync warning: %v", err)
		}
	}
	if err := fileimportbase.AutoSaveResultsToWiseMED(m.rt, fileimportbase.FlattenAutoSaveTargets(autoSaveTargets)); err != nil {
		m.rt.Logf("labnovation-ld560 result autosave warning: %v", err)
	}
	m.logf(4, "labnovation-ld560 persist done source=%s imported=%d analytes_changed=%t", source, imported, analytesChanged)
	return imported, nil
}

func (m *Module) ensureAnalyte(store storageService, known map[string]coremodel.Analyte, result parsedResult) (bool, error) {
	target := coremodel.Analyte{
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
	}
	existing, ok := known[strings.TrimSpace(target.Tag)]
	if ok && strings.EqualFold(strings.TrimSpace(existing.Tag), strings.TrimSpace(target.Tag)) {
		return false, nil
	}
	saved, err := store.SaveAnalyte(target)
	if err != nil {
		return false, err
	}
	known[strings.TrimSpace(saved.Tag)] = saved
	return true, nil
}

func (m *Module) storage() storageService {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(storageService)
	return store
}

func (m *Module) syncAnalytesToWiseMED() error {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	api, ok := service.(wiseMedSyncService)
	if !ok || !api.SetupComplete() {
		return nil
	}
	_, err := api.EnsureEquipmentOnline(nil)
	return err
}

func (m *Module) verboseLevel() int {
	raw := strings.TrimSpace(fmt.Sprint(m.rt.ModuleSettings("logging")["verbose_level"]))
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 1
	}
	if value > 5 {
		return 5
	}
	return value
}

func (m *Module) logf(level int, format string, args ...interface{}) {
	if m.verboseLevel() >= level {
		m.rt.Logf(format, args...)
	}
}

func (m *Module) logParsedMessages(protocol string, messages []parsedMessage) {
	m.logf(4, "labnovation-ld560 parse ok protocol=%s messages=%d", protocol, len(messages))
	if m.verboseLevel() < 5 {
		return
	}
	preview := map[string]interface{}{}
	if len(messages) > 0 {
		preview["first_message"] = messages[0]
	}
	if blob, err := json.Marshal(preview); err == nil {
		m.rt.Logf("labnovation-ld560 parse preview protocol=%s %s", protocol, string(blob))
	}
}

func (m *Module) logIgnored(kind, reason string, payload map[string]interface{}) {
	if m.verboseLevel() < 5 {
		return
	}
	entry := map[string]interface{}{
		"kind":   kind,
		"reason": reason,
	}
	for key, value := range payload {
		entry[key] = value
	}
	if blob, err := json.Marshal(entry); err == nil {
		m.rt.Logf("labnovation-ld560 ignored %s", string(blob))
		return
	}
	m.rt.Logf("labnovation-ld560 ignored kind=%s reason=%s", kind, reason)
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

func (m *Module) appendAuditLog(level, eventType, message string, meta map[string]interface{}) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return
	}
	logger, ok := service.(auditLogger)
	if !ok {
		return
	}
	if err := logger.AppendAuditLog(level, "system", eventType, strings.TrimSpace(message), meta); err != nil {
		m.rt.Logf("labnovation-ld560 audit log failed: %v", err)
	}
}

func (m *Module) logWire(direction, transport, details string, payload []byte) {
	if m.verboseLevel() < 4 {
		return
	}
	m.rt.Logf("labnovation-ld560 %s", commtrace.Format(direction, transport, details, payload, m.verboseLevel()))
}

func (m *Module) markAnalyzerConnected(delta int) {
	service, ok := m.rt.Service("analyzer-activity")
	if !ok {
		return
	}
	tracker, ok := service.(analyzerActivityTracker)
	if !ok {
		return
	}
	tracker.Connected(delta)
}

func (m *Module) markAnalyzerPacket(transport string) {
	service, ok := m.rt.Service("analyzer-activity")
	if !ok {
		return
	}
	tracker, ok := service.(analyzerActivityTracker)
	if !ok {
		return
	}
	tracker.Packet("in", transport)
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

func (m *Module) simpleImageMode() string {
	settings := m.settingsPayload()
	simple, _ := settings["simple"].(map[string]interface{})
	mode := normalizeImageMode(asString(simple["image_mode"]))
	if mode != "" {
		return mode
	}
	return normalizeImageMode(asString(settings["image_mode"]))
}

func (m *Module) saveMessageImage(item parsedMessage, image parsedImage) (string, error) {
	if len(image.data) == 0 {
		return "", nil
	}
	root := m.rt.ResolvePath(filepath.Join("images", "labnovation-ld-560"))
	tuple := strings.Join([]string{
		safePathToken(item.FileID, "missing-fileid"),
		safePathToken("", "missing-samplecode"),
		safePathToken("", "missing-specimen"),
	}, "__")
	targetDir := filepath.Join(root, tuple)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	ext := image.format
	if ext == "" {
		ext = "bin"
	}
	filename := fmt.Sprintf("%s_%s.%s",
		safePathToken(firstNonEmpty(trimFileExt(image.name), item.FileID, item.SampleID, item.SampleNo), "image"),
		time.Now().UTC().Format("20060102T150405.000Z0700"),
		ext,
	)
	absPath := filepath.Join(targetDir, filename)
	if err := os.WriteFile(absPath, image.data, 0o644); err != nil {
		return "", err
	}
	relPath, err := filepath.Rel(m.rt.ConfigDir(), absPath)
	if err != nil {
		return absPath, nil
	}
	return relPath, nil
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

func normalizeImageMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "no_image", "no-image", "none", "off", "":
		return "no_image"
	case "2", "bitmap", "bmp":
		return "bitmap"
	case "3", "base64", "base_64", "b64":
		return "base64"
	default:
		return "no_image"
	}
}

func safePathToken(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", " ", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "._-")
	if value == "" {
		return fallback
	}
	return value
}

func trimFileExt(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
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
