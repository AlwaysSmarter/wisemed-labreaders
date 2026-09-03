package astm

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	"wisemed-labreaders/readersv3/shared/analyzeractivity"
	"wisemed-labreaders/readersv3/shared/bindguard"
	"wisemed-labreaders/readersv3/shared/debugreplay"
)

const (
	ctrlENQ = byte(0x05)
	ctrlACK = byte(0x06)
	ctrlNAK = byte(0x15)
	ctrlEOT = byte(0x04)
	ctrlSTX = byte(0x02)
	ctrlETX = byte(0x03)
	ctrlETB = byte(0x17)
)

type analyteStore interface {
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type importStore interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ReapplyOrderTransformations(orderIDs []int64) error
	UpsertQCRecord(item coremodel.QCRecord) (coremodel.QCRecord, error)
	UpsertQCAnalysis(item coremodel.QCAnalysis) (coremodel.QCAnalysis, error)
	ReapplyQCTransformations(recordIDs []int64) error
	ListOrders(roundNo int, orderDate string) ([]coremodel.Order, error)
	ListOrderAnalyses(orderID int64) ([]coremodel.OrderAnalysis, error)
	UpsertOrder(item coremodel.Order) (coremodel.Order, error)
	SaveOrderAnalysis(item coremodel.OrderAnalysis) (coremodel.OrderAnalysis, error)
}

type wiseMEDLookup interface {
	Settings() map[string]string
	FetchFileForAnalyzer(fileID, equipmentID string) (map[string]interface{}, error)
}

type auditLogger interface {
	AppendAuditLog(level, actor, eventType, message string, meta map[string]interface{}) error
}

type analyzerActivityTracker interface {
	Connected(delta int)
	Packet(direction, transport string)
	Snapshot() analyzeractivity.Snapshot
}

type tcpConfig struct {
	CommType          string
	Mode              string
	ListenHost        string
	ListenPort        string
	RemoteHost        string
	RemotePort        string
	SendACK           bool
	FrameTimeout      time.Duration
	ChecksumMode      string
	TrailerMode       string
	SampleIDPaths     []string
	SampleIDTrimLeft  string
	SampleIDTrimRight string
	PatientIDPath     []string
	PatientName       []string
	RunDatePaths      []string
	ResultIDPaths     []string
	ResultName        []string
	ResultValue       []string
	ResultUnit        []string
	ResultFlag        []string
	QCPrefixes        []string
	AnalyteMap        map[string]string
	HostQuerySpecimen string
}

type protocolStatus struct {
	ConnectedClients int       `json:"connected_clients"`
	LastMessageAt    time.Time `json:"last_message_at"`
	LastImportCount  int       `json:"last_import_count"`
	LastError        string    `json:"last_error"`
	LastSource       string    `json:"last_source"`
}

type astmRecord struct {
	Type   string
	Fields []string
	Raw    string
}

type astmOrder struct {
	SampleID    string
	FileID      string
	PatientID   string
	PatientName string
	RunDate     string
	IsQC        bool
}

type astmResult struct {
	Order       astmOrder
	AnalyteTag  string
	AnalyteName string
	Value       string
	RawValue    string
	Unit        string
	Flag        string
}

type astmFrameInfo struct {
	Text             string
	RawPayload       []byte
	Terminator       byte
	ChecksumRaw      string
	ChecksumExpected string
	ChecksumPresent  bool
	ChecksumValid    bool
	TrailerRaw       []byte
}

type Module struct {
	rt module.Runtime

	mu      sync.Mutex
	clients int
	status  protocolStatus
}

func New() module.Module { return &Module{} }

func (m *Module) ID() string { return "protocol-astm" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: "protocol-astm", Group: "admin", Label: "Protocol ASTM", Path: "/settings/protocol/astm", Order: 45})
	rt.Handle("/api/protocol/meta", http.HandlerFunc(m.handleMeta))
	rt.Handle("/api/protocol/astm/status", http.HandlerFunc(m.handleStatus))
	rt.Handle("/settings/protocol/astm", http.HandlerFunc(m.handleSettingsPage))
	rt.RegisterService("protocol-astm-status", m)
	rt.RegisterService("debug-replay-runner", m)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	for {
		cfg := m.readConfig()
		if !strings.EqualFold(cfg.CommType, "tcpip") {
			m.rt.Logf("astm protocol idle: comm_type=%q", cfg.CommType)
			<-ctx.Done()
			return nil
		}
		runCtx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func(current tcpConfig) {
			if strings.EqualFold(current.Mode, "client") {
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
		case err := <-errCh:
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			if err == nil {
				time.Sleep(1 * time.Second)
				continue
			}
			m.setError(err)
			m.rt.Logf("astm communication error: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func (m *Module) Snapshot() protocolStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Module) ConnectedClients() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clients
}

func (m *Module) handleMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"protocol":      "astm",
		"communication": "tcpip",
		"tcp_mode":      m.readConfig().Mode,
	})
}

func (m *Module) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	cfg := m.readConfig()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"status": m.Snapshot(),
		"tcpip": map[string]string{
			"mode":        cfg.Mode,
			"host":        cfg.ListenHost,
			"port":        cfg.ListenPort,
			"remote_host": cfg.RemoteHost,
			"remote_port": cfg.RemotePort,
		},
	})
}

func (m *Module) handleSettingsPage(w http.ResponseWriter, _ *http.Request) {
	cfg := m.readConfig()
	payload, _ := json.MarshalIndent(map[string]interface{}{
		"comm_type": cfg.CommType,
		"tcpip": map[string]string{
			"mode":        cfg.Mode,
			"host":        cfg.ListenHost,
			"port":        cfg.ListenPort,
			"remote_host": cfg.RemoteHost,
			"remote_port": cfg.RemotePort,
		},
		"paths": map[string][]string{
			"sample_id":    cfg.SampleIDPaths,
			"patient_id":   cfg.PatientIDPath,
			"patient_name": cfg.PatientName,
			"run_date":     cfg.RunDatePaths,
			"result_id":    cfg.ResultIDPaths,
			"result_name":  cfg.ResultName,
			"result_value": cfg.ResultValue,
			"result_unit":  cfg.ResultUnit,
			"result_flag":  cfg.ResultFlag,
		},
		"qc_prefixes": cfg.QCPrefixes,
		"framing": map[string]string{
			"checksum_mode": cfg.ChecksumMode,
			"trailer_mode":  cfg.TrailerMode,
		},
	}, "", "  ")
	html := fmt.Sprintf(`<!doctype html><html lang="ro"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Protocol ASTM</title><style>body{font:15px/1.5 Arial,sans-serif;margin:24px;color:#1f2937}pre{background:#111827;color:#e5e7eb;padding:16px;border-radius:12px;overflow:auto}code{background:#f3f4f6;padding:2px 6px;border-radius:6px}</style></head><body><h1>Protocol ASTM</h1><p>Stack activ pentru Gemini `+"`TCP/IP + ASTM`"+`. Status live: <code>/api/protocol/astm/status</code>.</p><pre>%s</pre></body></html>`, string(payload))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (m *Module) RunDebugReplay(ctx context.Context, scriptName string, steps []debugreplay.Step) (debugreplay.Result, error) {
	cfg := m.readConfig()
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
		m.handleConn(runCtx, conn, cfg, "debug")
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
		if len(step.Input) > 0 {
			if err := writeReplayInput(client, step.Input, step.WriteBytewise); err != nil {
				stepResult.Passed = false
				stepResult.Error = err.Error()
			}
		}
		if stepResult.Passed {
			switch step.Terminator {
			case debugreplay.TerminatorOUT:
				actual, readErr := readReplayOutput(client, 2*time.Second, 250*time.Millisecond)
				stepResult.ActualOutput = debugreplay.DescribePayload(actual)
				if readErr != nil {
					stepResult.Passed = false
					stepResult.Error = readErr.Error()
				} else if string(actual) != string(step.ExpectedOutput) {
					stepResult.Passed = false
					stepResult.Error = "output mismatch"
				}
			case debugreplay.TerminatorEOT:
				actual, _ := readReplayOutput(client, 250*time.Millisecond, 150*time.Millisecond)
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

func writeReplayInput(conn net.Conn, payload []byte, bytewise bool) error {
	if !bytewise {
		_, err := conn.Write(payload)
		return err
	}
	for _, value := range payload {
		if _, err := conn.Write([]byte{value}); err != nil {
			return err
		}
	}
	return nil
}

func readReplayOutput(conn net.Conn, startTimeout, quietWindow time.Duration) ([]byte, error) {
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
			if errors.Is(err, io.EOF) {
				return payload, nil
			}
			return payload, err
		}
	}
}

func (m *Module) runTCPServer(ctx context.Context, cfg tcpConfig) error {
	addr := net.JoinHostPort(cfg.ListenHost, cfg.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if bindguard.IsAddressInUse(err) {
			nextAddr, _, handleErr := bindguard.HandleAddressInUse(m.rt.ConfigPath(), addr, map[string]interface{}{
				"modules.transport-tcpip.host": cfg.ListenHost,
				"modules.transport-tcpip.port": cfg.ListenPort,
			}, m.rt.Logf)
			if handleErr == nil {
				msg := fmt.Sprintf("astm nu se poate binda la %s: %v. Propun %s si scriu aceasta valoare in config.yaml", addr, err, nextAddr)
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
		return err
	}
	defer ln.Close()
	m.rt.Logf("astm listening as tcp server on %s", addr)
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
				return err
			}
		}
		go m.handleConn(ctx, conn, cfg, "server")
	}
}

func (m *Module) runTCPClient(ctx context.Context, cfg tcpConfig) error {
	target := net.JoinHostPort(cfg.RemoteHost, cfg.RemotePort)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		conn, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}
		m.handleConn(ctx, conn, cfg, "client")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Module) handleConn(ctx context.Context, conn net.Conn, cfg tcpConfig, mode string) {
	defer conn.Close()
	m.connected(1)
	defer m.connected(-1)
	if tracker := m.activityTracker(); tracker != nil {
		tracker.Connected(1)
		defer tracker.Connected(-1)
	}
	remote := conn.RemoteAddr().String()
	m.rt.Logf("astm client connected mode=%s remote=%s", mode, remote)
	reader := bufio.NewReader(conn)
	var frames []string
	var rawLines []string
	var lineBuf strings.Builder
	var pendingReply [][]byte
	replyActive := false
	frameActive := false
	frameEnded := false
	frameTerminator := byte(0)
	framePayload := make([]byte, 0, 256)
	frameTrailer := make([]byte, 0, 4)
	expectedTrailerBytes := astmExpectedTrailerBytes(cfg)
	resetFrame := func() {
		frameActive = false
		frameEnded = false
		frameTerminator = 0
		framePayload = framePayload[:0]
		frameTrailer = frameTrailer[:0]
	}
	for {
		if cfg.FrameTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(cfg.FrameTimeout))
		}
		b, err := reader.ReadByte()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if frameActive {
					continue
				}
				if len(frames) > 0 {
					_ = m.processBatch(strings.Join(frames, ""), remote, cfg)
					frames = nil
				}
				continue
			}
			if errors.Is(err, io.EOF) {
				break
			}
			m.setError(err)
			m.rt.Logf("astm read error remote=%s: %v", remote, err)
			break
		}
		m.tracePacket("in", "tcpip", remote, []byte{b})
		if tracker := m.activityTracker(); tracker != nil {
			tracker.Packet("in", "tcpip")
		}
		if frameActive {
			if !frameEnded {
				if b == ctrlETX || b == ctrlETB {
					frameTerminator = b
					if expectedTrailerBytes == 0 {
						frame := m.buildFrameInfo(framePayload, frameTerminator, nil, cfg)
						frames = append(frames, frame.Text)
						m.traceFrame("in", "tcpip", remote+" FRAME", frame)
						if cfg.SendACK {
							_, _ = conn.Write([]byte{ctrlACK})
							m.tracePacket("out", "tcpip", remote+" ACK", []byte{ctrlACK})
						}
						resetFrame()
						continue
					}
					frameEnded = true
					frameTrailer = frameTrailer[:0]
					continue
				}
				framePayload = append(framePayload, b)
				continue
			}
			frameTrailer = append(frameTrailer, b)
			if len(frameTrailer) < expectedTrailerBytes {
				continue
			}
			frame := m.buildFrameInfo(framePayload, frameTerminator, frameTrailer, cfg)
			frames = append(frames, frame.Text)
			m.traceFrame("in", "tcpip", remote+" FRAME", frame)
			if cfg.SendACK {
				_, _ = conn.Write([]byte{ctrlACK})
				m.tracePacket("out", "tcpip", remote+" ACK", []byte{ctrlACK})
			}
			resetFrame()
			continue
		}
		switch b {
		case ctrlENQ:
			if cfg.SendACK {
				_, _ = conn.Write([]byte{ctrlACK})
				m.tracePacket("out", "tcpip", remote+" ACK", []byte{ctrlACK})
			}
			frames = nil
			rawLines = nil
			lineBuf.Reset()
		case ctrlSTX:
			frameActive = true
			frameEnded = false
			framePayload = framePayload[:0]
			frameTrailer = frameTrailer[:0]
		case ctrlEOT:
			if len(frames) > 0 {
				payload := strings.Join(frames, "")
				if sampleID := querySampleID(parseRecords(payload)); sampleID != "" {
					// ASTM transfers change direction only after the analyzer's EOT.
					// Send ENQ first so an API lookup cannot make the analyzer time out.
					replyActive = true
					_, _ = conn.Write([]byte{ctrlENQ})
					m.tracePacket("out", "tcpip", remote+" ENQ query-reply", []byte{ctrlENQ})
					pendingReply = m.hostQueryReply(sampleID, cfg)
					m.logLevel1("astm query received sample_id=%s reply_frames=%d", sampleID, len(pendingReply))
				} else if err := m.processBatch(payload, remote, cfg); err != nil {
					m.setError(err)
				}
				frames = nil
			}
			resetFrame()
		case ctrlACK:
			if replyActive {
				if len(pendingReply) == 0 {
					_, _ = conn.Write([]byte{ctrlEOT})
					m.tracePacket("out", "tcpip", remote+" EOT query-reply", []byte{ctrlEOT})
					replyActive = false
					continue
				}
				framePayload := pendingReply[0]
				pendingReply = pendingReply[1:]
				_, _ = conn.Write(framePayload)
				m.tracePacket("out", "tcpip", remote+" query-reply frame", framePayload)
			}
		case ctrlNAK:
			if replyActive {
				m.rt.Logf("astm query reply rejected by analyzer remote=%s", remote)
				replyActive = false
				pendingReply = nil
			}
		case '\r', '\n':
			line := strings.TrimSpace(lineBuf.String())
			lineBuf.Reset()
			if line == "" {
				continue
			}
			rawLines = append(rawLines, line)
			if strings.HasPrefix(line, "L|") {
				if err := m.processBatch(strings.Join(rawLines, "\r"), remote, cfg); err != nil {
					m.setError(err)
				}
				rawLines = nil
			}
		default:
			if isProtocolByte(b) {
				lineBuf.WriteByte(b)
			}
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	if len(frames) > 0 {
		_ = m.processBatch(strings.Join(frames, ""), remote, cfg)
	}
	if len(rawLines) > 0 {
		_ = m.processBatch(strings.Join(rawLines, "\r"), remote, cfg)
	}
}

func normalizeFrameText(text string) string {
	if text != "" && unicode.IsDigit(rune(text[0])) {
		text = text[1:]
	}
	return text
}

func formatASTMPackageText(payload string) string {
	return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(payload)
}

func (m *Module) processBatch(payload, remote string, cfg tcpConfig) error {
	records := parseRecords(payload)
	if len(records) == 0 {
		m.logProcessing("astm parse source=%s records=0", remote)
		return nil
	}
	m.rt.Logf("PARSED ASTM PACKAGE: TEXT: %s", formatASTMPackageText(payload))
	m.logProcessing("astm parse source=%s records=%d types=%s", remote, len(records), summarizeRecordTypes(records))
	results := parseBatch(records, cfg)
	if len(results) == 0 {
		m.logProcessing("astm parse source=%s produced no results", remote)
		return errors.New("astm payload parsed but produced no results")
	}
	store := m.importStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	known, _ := m.knownAnalytes()
	imported := 0
	qcCache := map[string]coremodel.QCRecord{}
	roundCache := map[string]int{}
	orderIDs := []int64{}
	qcRecordIDs := []int64{}
	sourceName := sanitizeSourceName(remote)
	for _, item := range results {
		m.logLevel1("astm rezultat primit sample_id=%s patient_id=%s analyte=%s value=%s",
			firstNonEmpty(item.Order.SampleID, "-"),
			firstNonEmpty(item.Order.PatientID, "-"),
			firstNonEmpty(item.AnalyteTag, item.AnalyteName, "-"),
			firstNonEmpty(strings.TrimSpace(item.Value), "-"))
		tag := normalizeTag(firstNonEmpty(mappedAnalyte(cfg.AnalyteMap, item.AnalyteTag), item.AnalyteName))
		if tag == "" || strings.TrimSpace(item.Value) == "" {
			m.logProcessing("astm skip source=%s sample_id=%s patient_id=%s analyte_tag=%s analyte_name=%s reason=%s",
				remote,
				firstNonEmpty(item.Order.SampleID, "-"),
				firstNonEmpty(item.Order.PatientID, "-"),
				firstNonEmpty(item.AnalyteTag, "-"),
				firstNonEmpty(item.AnalyteName, "-"),
				missingResultReason(tag, item.Value))
			continue
		}
		name := firstNonEmpty(mappedAnalyte(cfg.AnalyteMap, item.AnalyteName), item.AnalyteTag, tag)
		if err := m.ensureAnalyte(known, tag, name, item.Unit); err != nil {
			return err
		}
		runDate := effectiveDate(item.Order.RunDate)
		if item.Order.IsQC {
			recordKey := runDate + "|" + item.Order.SampleID
			record := qcCache[recordKey]
			if record.ID == 0 {
				saved, err := store.UpsertQCRecord(coremodel.QCRecord{
					RunDate:      runDate,
					ControlLabel: firstNonEmpty(item.Order.SampleID, "QC"),
					ControlLevel: detectQCLevel(item.Order.SampleID),
					LotNo:        "-",
					FileID:       item.Order.FileID,
					Status:       "completed",
					SourceFile:   sourceName,
				})
				if err != nil {
					return err
				}
				record = saved
				qcCache[recordKey] = saved
			}
			qcRecordIDs = append(qcRecordIDs, record.ID)
			if _, err := store.UpsertQCAnalysis(coremodel.QCAnalysis{
				QCRecordID:  record.ID,
				AnalyteTag:  tag,
				AnalyteName: name,
				Status:      "completed",
				ResultValue: strings.TrimSpace(item.Value),
				RawValue:    strings.TrimSpace(item.RawValue),
				Interpreted: strings.TrimSpace(item.Flag),
				Unit:        strings.TrimSpace(item.Unit),
				SourceFile:  sourceName,
				Flags: map[string]interface{}{
					"protocol":   "astm",
					"remote":     remote,
					"sample_id":  item.Order.SampleID,
					"patient_id": item.Order.PatientID,
				},
			}); err != nil {
				return err
			}
			imported++
			continue
		}
		roundNo := roundCache[runDate]
		if roundNo == 0 {
			current, err := store.CurrentRoundNo(runDate)
			if err != nil {
				return err
			}
			roundNo = current
			roundCache[runDate] = roundNo
		}
		order, _, _, err := store.RecordImportedResult(runDate, roundNo, coremodel.ImportedRecord{
			SampleID:    item.Order.SampleID,
			FileID:      firstNonEmpty(item.Order.FileID, item.Order.SampleID),
			PatientID:   item.Order.PatientID,
			PatientName: item.Order.PatientName,
			AnalyteTag:  tag,
			AnalyteName: name,
			ResultValue: strings.TrimSpace(item.Value),
			RawValue:    strings.TrimSpace(item.RawValue),
			Interpreted: strings.TrimSpace(item.Flag),
			Unit:        strings.TrimSpace(item.Unit),
			Meta: map[string]interface{}{
				"protocol": "astm",
				"remote":   remote,
			},
		}, sourceName)
		if err != nil {
			return err
		}
		orderIDs = append(orderIDs, order.ID)
		imported++
	}
	if err := store.ReapplyOrderTransformations(orderIDs); err != nil {
		return err
	}
	if err := store.ReapplyQCTransformations(qcRecordIDs); err != nil {
		return err
	}
	m.setImport(imported, remote)
	m.appendAudit("info", "protocol-astm", "astm-import", fmt.Sprintf("Import ASTM reusit din %s.", remote), map[string]interface{}{
		"imported": imported,
		"source":   remote,
	})
	m.rt.Logf("astm import ok source=%s records=%d", remote, imported)
	return nil
}

func parseRecords(payload string) []astmRecord {
	lines := strings.FieldsFunc(payload, func(r rune) bool { return r == '\r' || r == '\n' })
	out := make([]astmRecord, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(stripFrameSequence(raw))
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) == 0 {
			continue
		}
		out = append(out, astmRecord{
			Type:   strings.ToUpper(strings.TrimSpace(fields[0])),
			Fields: fields,
			Raw:    line,
		})
	}
	return out
}

func parseBatch(records []astmRecord, cfg tcpConfig) []astmResult {
	results := []astmResult{}
	currentPatientID := ""
	currentPatientName := ""
	currentRunDate := ""
	currentOrder := astmOrder{}
	for _, rec := range records {
		switch rec.Type {
		case "P":
			currentPatientID = firstFromPaths(rec, cfg.PatientIDPath)
			currentPatientName = normalizePersonName(firstFromPaths(rec, cfg.PatientName))
		case "O":
			sampleID := strings.TrimLeft(firstFromPaths(rec, cfg.SampleIDPaths), cfg.SampleIDTrimLeft)
			sampleID = strings.TrimRight(sampleID, cfg.SampleIDTrimRight)
			if strings.TrimSpace(sampleID) == "" {
				sampleID = currentPatientID
			}
			currentRunDate = firstNonEmpty(parseRunDate(firstFromPaths(rec, cfg.RunDatePaths)), currentRunDate)
			currentOrder = astmOrder{
				SampleID:    sampleID,
				FileID:      firstNonEmpty(sampleID, currentPatientID),
				PatientID:   currentPatientID,
				PatientName: currentPatientName,
				RunDate:     currentRunDate,
				IsQC:        isQCSample(sampleID, cfg.QCPrefixes),
			}
		case "R":
			if currentOrder.SampleID == "" {
				continue
			}
			analyteTag := normalizeTag(firstFromPaths(rec, cfg.ResultIDPaths))
			analyteName := firstNonEmpty(firstFromPaths(rec, cfg.ResultName), analyteTag)
			value := firstFromPaths(rec, cfg.ResultValue)
			unit := firstFromPaths(rec, cfg.ResultUnit)
			flag := firstFromPaths(rec, cfg.ResultFlag)
			results = append(results, astmResult{
				Order:       currentOrder,
				AnalyteTag:  analyteTag,
				AnalyteName: analyteName,
				Value:       strings.TrimSpace(value),
				RawValue:    strings.TrimSpace(value),
				Unit:        strings.TrimSpace(unit),
				Flag:        strings.TrimSpace(flag),
			})
		}
	}
	return results
}

func querySampleID(records []astmRecord) string {
	for _, record := range records {
		if record.Type != "Q" || len(record.Fields) < 3 {
			continue
		}
		for _, part := range strings.Split(record.Fields[2], "^") {
			if sampleID := strings.TrimSpace(part); sampleID != "" {
				return sampleID
			}
		}
	}
	return ""
}

// hostQueryReply starts a fresh ASTM ENQ/ACK exchange after a Q message arrives.
func (m *Module) hostQueryReply(sampleID string, cfg tcpConfig) [][]byte {
	order, analyses, found := m.findHostQueryOrder(sampleID, 7)
	if !found || len(analyses) == 0 {
		loadedOrder, loadedAnalyses, err := m.loadHostQueryOrder(sampleID)
		if err != nil {
			m.rt.Logf("astm query sample_id=%s WiseMED lookup failed: %v", sampleID, err)
		} else {
			order, analyses, found = loadedOrder, loadedAnalyses, true
		}
	}
	if !found {
		m.rt.Logf("astm query sample_id=%s: no order found; sending empty ASTM reply", sampleID)
		return m.hostQueryFrames(sampleID, "", nil, cfg)
	}
	return m.hostQueryFrames(firstNonEmpty(order.SampleID, order.FileID, sampleID), order.PatientName, analyses, cfg)
}

func (m *Module) findHostQueryOrder(sampleID string, lookupDays int) (coremodel.Order, []coremodel.OrderAnalysis, bool) {
	store := m.importStore()
	if store == nil {
		return coremodel.Order{}, nil, false
	}
	if lookupDays <= 0 {
		lookupDays = 7
	}
	for offset := 0; offset < lookupDays; offset++ {
		orders, err := store.ListOrders(0, time.Now().AddDate(0, 0, -offset).Format("2006-01-02"))
		if err != nil {
			continue
		}
		for _, order := range orders {
			if sampleID != order.SampleID && sampleID != order.FileID && sampleID != order.PatientID {
				continue
			}
			analyses, _ := store.ListOrderAnalyses(order.ID)
			return order, analyses, true
		}
	}
	return coremodel.Order{}, nil, false
}

func (m *Module) loadHostQueryOrder(sampleID string) (coremodel.Order, []coremodel.OrderAnalysis, error) {
	store := m.importStore()
	if store == nil {
		return coremodel.Order{}, nil, errors.New("storage service unavailable")
	}
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return coremodel.Order{}, nil, errors.New("wisemed api service unavailable")
	}
	wiseMED, ok := service.(wiseMEDLookup)
	if !ok {
		return coremodel.Order{}, nil, errors.New("wisemed api does not support file lookup")
	}
	equipmentID := strings.TrimSpace(wiseMED.Settings()["echipament_id"])
	if equipmentID == "" {
		return coremodel.Order{}, nil, errors.New("missing echipament_id in WiseMED settings")
	}
	response, err := wiseMED.FetchFileForAnalyzer(sampleID, equipmentID)
	if err != nil {
		return coremodel.Order{}, nil, fmt.Errorf("WiseMED file lookup failed: %w", err)
	}
	date := firstNonEmpty(valueString(response["o_file_date"]), time.Now().Format("2006-01-02"))
	round, err := store.CurrentRoundNo(date)
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	order, err := store.UpsertOrder(coremodel.Order{RoundNo: round, OrderDate: date, SampleID: sampleID, FileID: firstNonEmpty(valueString(response["o_file_id"]), sampleID), PatientID: valueString(response["o_patient_id"]), PatientName: valueString(response["o_patient_name"]), Status: "scheduled", SourceFile: "wisemed-host-query"})
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	for _, test := range mapValues(response["o_tests"]) {
		tag := strings.TrimSpace(valueString(test["t_tag"]))
		if tag == "" {
			continue
		}
		_, err := store.SaveOrderAnalysis(coremodel.OrderAnalysis{OrderID: order.ID, AnalyteTag: tag, AnalyteName: firstNonEmpty(valueString(test["t_name"]), tag), WiseMEDSMID: valueString(test["t_sm_id"]), WiseMEDFSMID: valueString(test["t_fsm_id"]), Status: "scheduled", SourceFile: "wisemed-host-query", Flags: map[string]interface{}{"analyzer_code": firstNonEmpty(valueString(test["t_code"]), tag)}})
		if err != nil {
			return coremodel.Order{}, nil, err
		}
	}
	analyses, err := store.ListOrderAnalyses(order.ID)
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	m.rt.Logf("astm query sample_id=%s: loaded WiseMED order id=%d tests=%d", sampleID, order.ID, len(analyses))
	return order, analyses, nil
}

func (m *Module) hostQueryFrames(sampleID, patientName string, analyses []coremodel.OrderAnalysis, cfg tcpConfig) [][]byte {
	codes := m.hostQueryCodes(analyses)
	patientName = strings.ReplaceAll(strings.TrimSpace(patientName), " ", "^")
	if patientName == "" {
		patientName = sampleID
	}
	records := []string{"H|\\^&|||WISEMED|||||||P|E1394-97|" + time.Now().Format("20060102150405"), "P|1||" + sampleID + "||" + patientName}
	if len(codes) > 0 {
		fields := make([]string, 27)
		fields[0], fields[1], fields[2] = "O", "1", sampleID
		fields[4], fields[5], fields[7], fields[15], fields[25] = strings.Join(codes, "\\"), "R", time.Now().Format("20060102150405"), firstNonEmpty(cfg.HostQuerySpecimen, "1"), "O"
		records = append(records, strings.Join(fields, "|"))
	}
	records = append(records, "L|1|N")
	frames := make([][]byte, 0, len(records))
	for index, record := range records {
		frames = append(frames, buildOutgoingFrame(record, byte((index+1)%8), cfg))
	}
	return frames
}

func (m *Module) hostQueryCodes(analyses []coremodel.OrderAnalysis) []string {
	analytes, _ := m.knownAnalytes()
	seen, codes := map[string]bool{}, []string{}
	for _, analysis := range analyses {
		tag := strings.ToUpper(strings.TrimSpace(analysis.AnalyteTag))
		code := firstNonEmpty(valueString(analysis.Flags["analyzer_code"]), analytes[tag].Code, analysis.AnalyteTag)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		codes = append(codes, "^^^"+code)
	}
	return codes
}

func buildOutgoingFrame(record string, sequence byte, cfg tcpConfig) []byte {
	payload := append([]byte{byte('0' + sequence)}, []byte(record)...)
	frame := append([]byte{ctrlSTX}, payload...)
	frame = append(frame, ctrlETX)
	if cfg.ChecksumMode != "none" {
		frame = append(frame, []byte(computeASTMChecksum(payload, ctrlETX))...)
	}
	if cfg.TrailerMode == "crlf" {
		frame = append(frame, '\r', '\n')
	}
	return frame
}

func valueString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mapValues(value interface{}) []map[string]interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]interface{}); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func readFrame(r *bufio.Reader) (string, error) {
	var payload []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == ctrlETX || b == ctrlETB {
			break
		}
		payload = append(payload, b)
	}
	checksum := make([]byte, 2)
	if _, err := io.ReadFull(r, checksum); err != nil {
		return "", err
	}
	_, _ = r.ReadByte()
	_, _ = r.ReadByte()
	text := string(payload)
	if text != "" && unicode.IsDigit(rune(text[0])) {
		text = text[1:]
	}
	return text, nil
}

func stripFrameSequence(value string) string {
	if len(value) > 0 && unicode.IsDigit(rune(value[0])) {
		return value[1:]
	}
	return value
}

func isProtocolByte(b byte) bool {
	return b == '|' || b == '^' || b == '\\' || b == '&' || b == '.' || b == '-' || b == '_' || b == ' ' || unicode.IsPrint(rune(b))
}

func firstFromPaths(rec astmRecord, paths []string) string {
	for _, path := range paths {
		if value := astmPath(rec, path); value != "" {
			return value
		}
	}
	return ""
}

func astmPath(rec astmRecord, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(parts[0]), rec.Type) {
		return ""
	}
	fieldIdx, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || fieldIdx <= 0 || fieldIdx >= len(rec.Fields) {
		return ""
	}
	value := rec.Fields[fieldIdx]
	if len(parts) >= 3 {
		compIdx, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil || compIdx <= 0 {
			return ""
		}
		components := strings.Split(value, "^")
		if compIdx > len(components) {
			return ""
		}
		value = components[compIdx-1]
	}
	return strings.TrimSpace(value)
}

func (m *Module) readConfig() tcpConfig {
	transport := m.rt.ModuleSettings("transport-tcpip")
	settings := m.rt.ModuleSettings(m.ID())
	return tcpConfig{
		CommType:          firstNonEmpty(asString(moduleServiceValue(m.rt, "analyzer-config", "comm_type")), "tcpip"),
		Mode:              firstNonEmpty(asString(transport["mode"]), "server"),
		ListenHost:        firstNonEmpty(asString(transport["host"]), "127.0.0.1"),
		ListenPort:        firstNonEmpty(asString(transport["port"]), "9000"),
		RemoteHost:        firstNonEmpty(asString(transport["remote_host"]), "127.0.0.1"),
		RemotePort:        firstNonEmpty(asString(transport["remote_port"]), firstNonEmpty(asString(transport["port"]), "9000")),
		SendACK:           boolSetting(settings, "send_ack", true),
		FrameTimeout:      time.Duration(intSetting(settings, "frame_timeout_ms", 2000)) * time.Millisecond,
		ChecksumMode:      astmFrameChecksumMode(settings),
		TrailerMode:       astmFrameTrailerMode(settings),
		SampleIDPaths:     listSetting(settings, "sample_id_paths", []string{"O.3.1", "O.2.1"}),
		SampleIDTrimLeft:  asString(settings["sample_id_trim_left"]),
		SampleIDTrimRight: asString(settings["sample_id_trim_right"]),
		PatientIDPath:     listSetting(settings, "patient_id_paths", []string{"P.3.1", "P.2.1"}),
		PatientName:       listSetting(settings, "patient_name_paths", []string{"P.5.1", "P.4.1"}),
		RunDatePaths:      listSetting(settings, "run_date_paths", []string{"O.7.1", "O.6.1", "H.13.1"}),
		ResultIDPaths:     listSetting(settings, "result_id_paths", []string{"R.2.4", "R.2.1"}),
		ResultName:        listSetting(settings, "result_name_paths", []string{"R.2.4", "R.2.1"}),
		ResultValue:       listSetting(settings, "result_value_paths", []string{"R.3.1"}),
		ResultUnit:        listSetting(settings, "result_unit_paths", []string{"R.4.1"}),
		ResultFlag:        listSetting(settings, "result_flag_paths", []string{"R.6.1", "R.8.1"}),
		QCPrefixes:        listSetting(settings, "qc_prefixes", []string{"QC", "CTRL", "CONTROL"}),
		AnalyteMap:        stringMapSetting(settings, "analyte_mappings"),
		HostQuerySpecimen: firstNonEmpty(asString(settings["specimen_code_default"]), "1"),
	}
}

func astmExpectedTrailerBytes(cfg tcpConfig) int {
	count := 0
	if cfg.ChecksumMode != "none" {
		count += 2
	}
	if cfg.TrailerMode == "crlf" {
		count += 2
	}
	return count
}

func astmFrameChecksumMode(settings map[string]interface{}) string {
	switch strings.ToLower(strings.TrimSpace(asString(settings["checksum_mode"]))) {
	case "none":
		return "none"
	default:
		return "astm"
	}
}

func astmFrameTrailerMode(settings map[string]interface{}) string {
	switch strings.ToLower(strings.TrimSpace(asString(settings["trailer_mode"]))) {
	case "none":
		return "none"
	default:
		return "crlf"
	}
}

func (m *Module) buildFrameInfo(payload []byte, terminator byte, trailer []byte, cfg tcpConfig) astmFrameInfo {
	info := astmFrameInfo{
		Text:       normalizeFrameText(string(payload)),
		RawPayload: append([]byte(nil), payload...),
		Terminator: terminator,
		TrailerRaw: append([]byte(nil), trailer...),
	}
	if cfg.ChecksumMode != "none" && len(trailer) >= 2 {
		info.ChecksumPresent = true
		info.ChecksumRaw = strings.ToUpper(string(trailer[:2]))
		info.ChecksumExpected = computeASTMChecksum(payload, terminator)
		info.ChecksumValid = strings.EqualFold(info.ChecksumRaw, info.ChecksumExpected)
	}
	return info
}

func computeASTMChecksum(payload []byte, terminator byte) string {
	sum := 0
	for _, b := range payload {
		sum += int(b)
	}
	sum += int(terminator)
	return strings.ToUpper(fmt.Sprintf("%02X", sum%256))
}

func summarizeRecordTypes(records []astmRecord) string {
	if len(records) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(records))
	for _, rec := range records {
		parts = append(parts, firstNonEmpty(rec.Type, "?"))
	}
	return strings.Join(parts, ",")
}

func missingResultReason(tag, value string) string {
	switch {
	case strings.TrimSpace(tag) == "" && strings.TrimSpace(value) == "":
		return "missing analyte tag and result value"
	case strings.TrimSpace(tag) == "":
		return "missing analyte tag"
	case strings.TrimSpace(value) == "":
		return "missing result value"
	default:
		return "unknown"
	}
}

func describeControlByte(value byte) string {
	switch value {
	case ctrlENQ:
		return "ENQ"
	case ctrlACK:
		return "ACK"
	case ctrlNAK:
		return "NAK"
	case ctrlEOT:
		return "EOT"
	case ctrlSTX:
		return "STX"
	case ctrlETX:
		return "ETX"
	case ctrlETB:
		return "ETB"
	case '\r':
		return "CR"
	case '\n':
		return "LF"
	default:
		if value == 0 {
			return "NONE"
		}
		if unicode.IsPrint(rune(value)) {
			return fmt.Sprintf("%q", value)
		}
		return fmt.Sprintf("0x%02X", value)
	}
}

func formatTraceText(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return "(empty)"
	}
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

func (m *Module) importStore() importStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(importStore)
	return store
}

func (m *Module) analyteStore() analyteStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(analyteStore)
	return store
}

func (m *Module) activityTracker() analyzerActivityTracker {
	service, ok := m.rt.Service("analyzer-activity")
	if !ok {
		return nil
	}
	tracker, _ := service.(analyzerActivityTracker)
	return tracker
}

func (m *Module) knownAnalytes() (map[string]coremodel.Analyte, error) {
	store := m.analyteStore()
	if store == nil {
		return map[string]coremodel.Analyte{}, nil
	}
	items, err := store.ListAnalytes()
	if err != nil {
		return nil, err
	}
	out := make(map[string]coremodel.Analyte, len(items))
	for _, item := range items {
		out[strings.ToUpper(strings.TrimSpace(item.Tag))] = item
	}
	return out, nil
}

func (m *Module) ensureAnalyte(known map[string]coremodel.Analyte, tag, name, unit string) error {
	store := m.analyteStore()
	if store == nil {
		return nil
	}
	key := strings.ToUpper(strings.TrimSpace(tag))
	if _, ok := known[key]; ok {
		return nil
	}
	saved, err := store.SaveAnalyte(coremodel.Analyte{
		Active:            true,
		Tag:               tag,
		Code:              tag,
		Name:              firstNonEmpty(strings.TrimSpace(name), tag),
		ResultType:        "text",
		ResultFormatting:  "raw",
		ResultMeasureUnit: strings.TrimSpace(unit),
		ProtocolOptions: map[string]interface{}{
			"source_protocol": "astm",
		},
	})
	if err != nil {
		return err
	}
	known[key] = saved
	return nil
}

func (m *Module) appendAudit(level, actor, eventType, message string, meta map[string]interface{}) {
	service, ok := m.rt.Service("storage")
	if !ok {
		return
	}
	logger, ok := service.(auditLogger)
	if !ok {
		return
	}
	_ = logger.AppendAuditLog(level, actor, eventType, message, meta)
}

func (m *Module) tracePacket(direction, transport, details string, payload []byte) {
	level := m.verboseLevel()
	arrow := "==>"
	label := "IN"
	if strings.EqualFold(strings.TrimSpace(direction), "out") {
		arrow = "<=="
		label = "OUT"
	}
	transport = strings.ToUpper(strings.TrimSpace(transport))
	details = strings.TrimSpace(details)
	if details != "" {
		details = " " + details
	}
	headline := fmt.Sprintf("%s %s %s%s len=%d", arrow, label, transport, details, len(payload))
	switch {
	case level <= 1:
		m.rt.Logf("%s", headline)
	case level == 2:
		m.rt.Logf("%s\nTEXT:\n%s", headline, formatTraceText(payload))
	default:
		m.rt.Logf("%s\nTEXT:\n%s\nHEX:\n%s", headline, formatTraceText(payload), strings.TrimRight(hex.Dump(payload), "\n"))
	}
}

func (m *Module) traceFrame(direction, transport, details string, frame astmFrameInfo) {
	m.tracePacket(direction, transport, details, frame.RawPayload)
	if m.verboseLevel() >= 3 {
		checksumStatus := "disabled"
		if frame.ChecksumPresent {
			checksumStatus = fmt.Sprintf("present raw=%s expected=%s valid=%t", frame.ChecksumRaw, frame.ChecksumExpected, frame.ChecksumValid)
		}
		m.rt.Logf("astm frame processing terminator=%s checksum=%s trailer_len=%d payload_len=%d",
			describeControlByte(frame.Terminator), checksumStatus, len(frame.TrailerRaw), len(frame.RawPayload))
	}
}

func (m *Module) verboseLevel() int {
	level := intSetting(m.rt.ModuleSettings("logging"), "verbose_level", 1)
	if level <= 0 {
		return 1
	}
	return level
}

func (m *Module) logLevel1(format string, args ...interface{}) {
	if m.verboseLevel() >= 1 {
		m.rt.Logf(format, args...)
	}
}

func (m *Module) logProcessing(format string, args ...interface{}) {
	if m.verboseLevel() >= 3 {
		m.rt.Logf(format, args...)
	}
}

func (m *Module) connected(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients += delta
	if m.clients < 0 {
		m.clients = 0
	}
	m.status.ConnectedClients = m.clients
}

func (m *Module) setError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.LastError = err.Error()
}

func (m *Module) setImport(count int, source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.LastImportCount = count
	m.status.LastMessageAt = time.Now()
	m.status.LastSource = source
	m.status.LastError = ""
}

func mappedAnalyte(mapping map[string]string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for key, mapped := range mapping {
		if strings.EqualFold(strings.TrimSpace(key), value) {
			return strings.TrimSpace(mapped)
		}
	}
	return value
}

func moduleServiceValue(rt module.Runtime, serviceName, key string) interface{} {
	service, ok := rt.Service(serviceName)
	if !ok {
		return nil
	}
	payload, ok := service.(map[string]interface{})
	if !ok {
		return nil
	}
	return payload[key]
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func boolSetting(settings map[string]interface{}, key string, fallback bool) bool {
	raw, ok := settings[key]
	if !ok {
		return fallback
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func intSetting(settings map[string]interface{}, key string, fallback int) int {
	raw, ok := settings[key]
	if !ok {
		return fallback
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func listSetting(settings map[string]interface{}, key string, fallback []string) []string {
	raw, ok := settings[key]
	if !ok {
		return append([]string(nil), fallback...)
	}
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(item); value != "" {
				out = append(out, value)
			}
		}
		if len(out) > 0 {
			return out
		}
	case []interface{}:
		out := []string{}
		for _, item := range typed {
			if value := strings.TrimSpace(asString(item)); value != "" {
				out = append(out, value)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return append([]string(nil), fallback...)
}

func stringMapSetting(settings map[string]interface{}, key string) map[string]string {
	raw, ok := settings[key]
	if !ok {
		return map[string]string{}
	}
	items, ok := raw.(map[string]interface{})
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(items))
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = strings.TrimSpace(asString(items[key]))
	}
	return out
}

func normalizeTag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToUpper(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if lastUnderscore {
			continue
		}
		b.WriteByte('_')
		lastUnderscore = true
	}
	return strings.Trim(b.String(), "_")
}

func sanitizeSourceName(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "astm-live"
	}
	remote = strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(remote)
	return filepath.Base("astm_" + remote)
}

func effectiveDate(value string) string {
	if parsed := parseRunDate(value); parsed != "" {
		return parsed
	}
	return time.Now().Format("2006-01-02")
}

func parseRunDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
	if len(digits) < 8 {
		return ""
	}
	raw := digits[:8]
	if _, err := time.Parse("20060102", raw); err != nil {
		return ""
	}
	return raw[:4] + "-" + raw[4:6] + "-" + raw[6:8]
}

func isQCSample(sampleID string, prefixes []string) bool {
	value := strings.ToUpper(strings.TrimSpace(sampleID))
	for _, prefix := range prefixes {
		prefix = strings.ToUpper(strings.TrimSpace(prefix))
		if prefix != "" && strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func detectQCLevel(sampleID string) string {
	value := strings.ToUpper(strings.TrimSpace(sampleID))
	for _, token := range []string{"L1", "L2", "L3", "L4", "L5", "LEVEL1", "LEVEL2", "LEVEL3"} {
		if strings.Contains(value, token) {
			return strings.ToLower(token)
		}
	}
	return "control"
}

func normalizePersonName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "^")
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}
