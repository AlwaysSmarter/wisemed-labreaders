// Package zoncixl1000i implements the native Zonci XL-series LIS protocol.
// It is not ASTM: frames are STX + command text + ETX and result rows use a configured separator.
package zoncixl1000i

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

const (
	stx = byte(0x02)
	etx = byte(0x03)
)

type store interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ReapplyOrderTransformations(orderIDs []int64) error
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
	ListOrders(roundNo int, orderDate string) ([]coremodel.Order, error)
	ListOrderAnalyses(orderID int64) ([]coremodel.OrderAnalysis, error)
	UpsertOrder(item coremodel.Order) (coremodel.Order, error)
	SaveOrderAnalysis(item coremodel.OrderAnalysis) (coremodel.OrderAnalysis, error)
	UpsertOrderResultForAnalysis(orderAnalysisID int64, resultValue, rawValue, interpreted, sourceResultValue, sourceRawValue, sourceInterpreted, unit, sourceFile string, flags map[string]interface{}) (coremodel.OrderAnalysisResult, bool, error)
}

type wiseMEDLookup interface {
	Settings() map[string]string
	FetchFileForAnalyzer(fileID, equipmentID string) (map[string]interface{}, error)
}

type auditLogger interface {
	AppendAuditLog(level, actor, eventType, message string, meta map[string]interface{}) error
}
type dailyAnalysisFilter interface {
	IsAnalysisSendBlocked(scopeDate, analyteTag, specimenCode string) bool
}

type config struct {
	CommType, TCPHost, TCPPort, SerialPort string
	SerialBaud                             int
	LookupDays                             int
	ResultSeparator                        string
}

type Module struct {
	rt module.Runtime
	mu sync.Mutex
}

func New() module.Module     { return &Module{} }
func (m *Module) ID() string { return "protocol-zonci-xl1000i" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	for {
		cfg := m.readConfig()
		m.rt.Logf("zonci-xl1000i result separator: %s (%q)", zonciSeparatorLabel(cfg.ResultSeparator), cfg.ResultSeparator)
		var err error
		if strings.EqualFold(cfg.CommType, "serial") {
			err = m.runSerial(ctx, cfg)
		} else {
			err = m.runTCPServer(ctx, cfg)
		}
		if ctx.Err() != nil {
			return nil
		}
		m.rt.Logf("zonci-xl1000i communication error: %v", err)
		m.audit("error", "communication_error", "Eroare comunicare Zonci XL1000i", map[string]interface{}{"error": fmt.Sprint(err), "transport": cfg.CommType})
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func (m *Module) runTCPServer(ctx context.Context, cfg config) error {
	address := net.JoinHostPort(cfg.TCPHost, cfg.TCPPort)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", address, err)
	}
	defer listener.Close()
	m.rt.Logf("zonci-xl1000i TCP server listening on %s", address)
	m.audit("info", "tcp_listening", "Server TCP Zonci XL1000i pornit", map[string]interface{}{"address": address})
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go m.handleStream(ctx, conn, cfg, "tcp:"+conn.RemoteAddr().String())
	}
}

func (m *Module) runSerial(ctx context.Context, cfg config) error {
	if cfg.SerialPort == "" {
		return errors.New("transport-serial.port is not configured")
	}
	port, err := serial.Open(cfg.SerialPort, &serial.Mode{BaudRate: cfg.SerialBaud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err != nil {
		return err
	}
	defer port.Close()
	if err := port.SetReadTimeout(500 * time.Millisecond); err != nil {
		return err
	}
	m.rt.Logf("zonci-xl1000i serial connected port=%s baud=%d", cfg.SerialPort, cfg.SerialBaud)
	buf := make([]byte, 1024)
	frame := make([]byte, 0, 2048)
	active := false
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, readErr := port.Read(buf)
		for _, b := range buf[:n] {
			switch b {
			case stx:
				frame, active = frame[:0], true
			case etx:
				if active {
					m.dispatchFrame(frame, cfg, "serial:"+cfg.SerialPort, func(reply []byte) error { _, err := port.Write(reply); return err })
					active = false
				}
			default:
				if active {
					frame = append(frame, b)
				}
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (m *Module) handleStream(ctx context.Context, conn net.Conn, cfg config, source string) {
	defer conn.Close()
	m.rt.Logf("zonci-xl1000i connected %s", source)
	buf := make([]byte, 1024)
	frame := make([]byte, 0, 2048)
	active := false
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := conn.Read(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				m.rt.Logf("zonci-xl1000i %s disconnected: %v", source, err)
			}
			return
		}
		for _, b := range buf[:n] {
			switch b {
			case stx:
				frame, active = frame[:0], true
			case etx:
				if active {
					m.dispatchFrame(frame, cfg, source, func(reply []byte) error { _, err := conn.Write(reply); return err })
					active = false
				}
			default:
				if active {
					frame = append(frame, b)
				}
			}
		}
	}
}

func (m *Module) dispatchFrame(frame []byte, cfg config, source string, send func([]byte) error) {
	// Zonci uses a single active LIS session. Serialize requests so an order
	// reply and a result cannot update the same local order concurrently.
	m.mu.Lock()
	defer m.mu.Unlock()
	text := strings.TrimSpace(strings.Trim(string(frame), "\r\n"))
	if text == "" {
		return
	}
	m.rt.Logf("zonci-xl1000i rx %s: %s", source, printable(text))
	parts := strings.Fields(strings.SplitN(text, "\n", 2)[0])
	if len(parts) < 2 {
		m.rt.Logf("zonci-xl1000i ignored malformed frame: %q", text)
		return
	}
	sampleID := strings.TrimSpace(parts[1])
	switch parts[0] {
	case "1":
		codes, err := m.loadOrder(sampleID, true)
		if err != nil {
			m.rt.Logf("zonci-xl1000i host query sample_id=%s: %v", sampleID, err)
			return
		}
		reply := []byte{stx}
		reply = append(reply, []byte("2 "+sampleID)...)
		if len(codes) > 0 {
			reply = append(reply, ' ')
			reply = append(reply, []byte(strings.Join(codes, " "))...)
		}
		reply = append(reply, etx)
		if err := send(reply); err != nil {
			m.rt.Logf("zonci-xl1000i cannot reply sample_id=%s: %v", sampleID, err)
			return
		}
		m.rt.Logf("zonci-xl1000i tx order sample_id=%s tests=%d", sampleID, len(codes))
		m.audit("info", "order_reply", "Ordin Zonci trimis", map[string]interface{}{"sample_id": sampleID, "tests": codes})
	case "3":
		if err := m.saveResults(sampleID, text, source, cfg); err != nil {
			m.rt.Logf("zonci-xl1000i result import sample_id=%s: %v", sampleID, err)
		}
	case "4":
		m.rt.Logf("zonci-xl1000i QC frame received and retained in protocol log")
	default:
		m.rt.Logf("zonci-xl1000i unsupported command=%s sample_id=%s", parts[0], sampleID)
	}
}

// loadOrder creates/refreshes the local WiseMED order. applyFilters is true only
// for an outgoing LIS order; historical incoming results must never be filtered.
func (m *Module) loadOrder(sampleID string, applyFilters bool) ([]string, error) {
	if order, analyses, ok := m.findOrder(sampleID); ok && len(analyses) > 0 {
		return m.orderCodes(order, analyses), nil
	}
	store, api := m.store(), m.wiseMED()
	if store == nil || api == nil {
		return nil, errors.New("storage sau WiseMED API indisponibil")
	}
	equipmentID := strings.TrimSpace(api.Settings()["echipament_id"])
	if equipmentID == "" {
		return nil, errors.New("lipseste echipament_id in configurarea WiseMED")
	}
	response, err := api.FetchFileForAnalyzer(sampleID, equipmentID)
	if err != nil {
		return nil, fmt.Errorf("WiseMED file lookup: %w", err)
	}
	date := normalizeOrderDate(firstNonEmpty(stringValue(response["o_file_date"]), time.Now().Format("2006-01-02")))
	round, err := store.CurrentRoundNo(date)
	if err != nil {
		return nil, err
	}
	order, err := store.UpsertOrder(coremodel.Order{RoundNo: round, OrderDate: date, SampleID: sampleID, FileID: firstNonEmpty(stringValue(response["o_file_id"]), sampleID), PatientID: stringValue(response["o_patient_id"]), PatientName: stringValue(response["o_patient_name"]), Status: "scheduled", SourceFile: "wisemed-host-query", Meta: map[string]interface{}{"protocol": "zonci-xl1000i", "equipment_id": equipmentID}})
	if err != nil {
		return nil, err
	}
	analytes, err := store.ListAnalytes()
	if err != nil {
		return nil, err
	}
	byTag := map[string]coremodel.Analyte{}
	for _, item := range analytes {
		byTag[strings.TrimSpace(item.Tag)] = item
	}
	for _, test := range mapSlice(response["o_tests"]) {
		tag := strings.TrimSpace(stringValue(test["t_tag"]))
		if tag == "" {
			continue
		}
		if applyFilters && m.isBlocked(tag, firstNonEmpty(stringValue(test["t_sample_code"]), stringValue(response["o_specimen_code"]))) {
			m.rt.Logf("zonci-xl1000i filtered WiseMED test=%s", tag)
			continue
		}
		name, code := firstNonEmpty(stringValue(test["t_name"]), tag), firstNonEmpty(stringValue(test["t_code"]), tag)
		analyte := byTag[tag]
		analyte.Tag, analyte.Name, analyte.Code, analyte.Active = tag, name, firstNonEmpty(code, analyte.Code), true
		if _, err := store.SaveAnalyte(analyte); err != nil {
			return nil, err
		}
		if _, err := store.SaveOrderAnalysis(coremodel.OrderAnalysis{OrderID: order.ID, AnalyteTag: tag, AnalyteName: name, WiseMEDSMID: stringValue(test["t_sm_id"]), WiseMEDFSMID: stringValue(test["t_fsm_id"]), Status: "scheduled", SourceFile: "wisemed-host-query", Flags: map[string]interface{}{"zonci_code": code, "wisemed_sample_code": firstNonEmpty(stringValue(test["t_sample_code"]), stringValue(response["o_specimen_code"]))}}); err != nil {
			return nil, err
		}
	}
	analyses, err := store.ListOrderAnalyses(order.ID)
	if err != nil {
		return nil, err
	}
	codes := m.orderCodes(order, analyses)
	m.rt.Logf("zonci-xl1000i host query sample_id=%s: created WiseMED order id=%d with %d tests", sampleID, order.ID, len(codes))
	return codes, nil
}

func (m *Module) orderCodes(_ coremodel.Order, analyses []coremodel.OrderAnalysis) []string {
	byTag := map[string]string{}
	if store := m.store(); store != nil {
		if analytes, err := store.ListAnalytes(); err == nil {
			for _, item := range analytes {
				byTag[item.Tag] = item.Code
			}
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, item := range analyses {
		if filter := m.dailyAnalysisFilter(); filter != nil && filter.IsAnalysisSendBlocked(time.Now().Format("2006-01-02"), item.AnalyteTag, stringValue(item.Flags["wisemed_sample_code"])) {
			continue
		}
		code := firstNonEmpty(stringValue(item.Flags["zonci_code"]), byTag[item.AnalyteTag], item.AnalyteTag)
		if code != "" && !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Module) saveResults(sampleID, text, source string, cfg config) error {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	if len(lines) < 2 {
		return errors.New("result frame does not contain result rows")
	}
	resultDate := zonciResultDate(lines, cfg.ResultSeparator)
	metadata := zonciMetadata(lines, cfg.ResultSeparator)
	order, analyses, found := m.findOrderForDate(sampleID, resultDate)
	if !found {
		// Reception is local and independent from WiseMED. A later, separate
		// synchronization can attach this order after the WiseMED file exists.
		var err error
		order, err = m.createUnmatchedResultOrder(sampleID, resultDate, metadata)
		if err != nil {
			return err
		}
		analyses = nil
	}
	byCode, byTag := map[string]coremodel.OrderAnalysis{}, map[string]coremodel.OrderAnalysis{}
	for _, item := range analyses {
		byTag[strings.ToUpper(item.AnalyteTag)] = item
		byCode[strings.ToUpper(firstNonEmpty(stringValue(item.Flags["zonci_code"]), item.AnalyteTag))] = item
	}
	updated := []int64{}
	for _, line := range lines[1:] {
		fields := zonciResultFields(line, cfg.ResultSeparator)
		if len(fields) < 2 {
			continue
		}
		code, value := strings.TrimSpace(fields[0]), normalizeValue(fields[1])
		if zonciMetadataKey(code) {
			continue
		}
		if code == "" || value == "" {
			continue
		}
		analysis, ok := byCode[strings.ToUpper(code)]
		if !ok {
			analysis, ok = byTag[strings.ToUpper(code)]
		}
		if !ok {
			var err error
			analysis, err = m.createUnmappedResultAnalysis(order, code)
			if err != nil {
				return err
			}
			byCode[strings.ToUpper(code)], byTag[strings.ToUpper(code)] = analysis, analysis
			m.rt.Logf("zonci-xl1000i unmapped result retained locally sample_id=%s code=%s", sampleID, code)
		}
		unit := ""
		if len(fields) > 2 {
			unit = strings.TrimSpace(fields[2])
		}
		analysis.Status, analysis.ResultValue, analysis.RawValue, analysis.SourceResultValue, analysis.SourceRawValue, analysis.Unit, analysis.SourceFile = "completed", value, value, value, value, unit, source
		analysis.Flags = mergeFlags(analysis.Flags, map[string]interface{}{"zonci_result_code": code, "zonci_received_at": time.Now().UTC().Format(time.RFC3339)})
		if _, err := m.store().SaveOrderAnalysis(analysis); err != nil {
			return err
		}
		if _, _, err := m.store().UpsertOrderResultForAnalysis(analysis.ID, value, value, "", value, value, "", unit, source, analysis.Flags); err != nil {
			return err
		}
		updated = append(updated, order.ID)
	}
	if len(updated) == 0 {
		return errors.New("no result row matched a requested Zonci test")
	}
	// Transformations enrich the locally persisted result. A transformation
	// problem must never reject an analyzer result or make it unavailable in
	// Cereri analize.
	if err := m.store().ReapplyOrderTransformations(updated); err != nil {
		m.rt.Logf("zonci-xl1000i transformation warning sample_id=%s: %v", sampleID, err)
	}
	m.rt.Logf("zonci-xl1000i import ok sample_id=%s results=%d", sampleID, len(updated))
	m.audit("info", "results_imported", "Rezultate Zonci importate", map[string]interface{}{"sample_id": sampleID, "results": len(updated), "source": source, "lookup_days": cfg.LookupDays})
	return nil
}

func (m *Module) createUnmatchedResultOrder(sampleID, resultDate string, metadata map[string]string) (coremodel.Order, error) {
	store := m.store()
	if store == nil {
		return coremodel.Order{}, errors.New("storage service unavailable")
	}
	date := firstNonEmpty(resultDate, time.Now().Format("2006-01-02"))
	round, err := store.CurrentRoundNo(date)
	if err != nil {
		return coremodel.Order{}, err
	}
	return store.UpsertOrder(coremodel.Order{RoundNo: round, OrderDate: date, SampleID: sampleID, FileID: sampleID, PatientID: firstNonEmpty(metadata["patientno"], sampleID), PatientName: metadata["name"], Status: "received", SourceFile: "zonci-unmatched-result", Meta: map[string]interface{}{"protocol": "zonci-xl1000i", "wisemed_match": "pending", "warning": "Rezultat primit fara fisa sau analize configurate in WiseMED la momentul receptiei"}})
}

func (m *Module) createUnmappedResultAnalysis(order coremodel.Order, code string) (coremodel.OrderAnalysis, error) {
	store := m.store()
	if store == nil {
		return coremodel.OrderAnalysis{}, errors.New("storage service unavailable")
	}
	if _, err := store.SaveAnalyte(coremodel.Analyte{Tag: code, Code: code, Name: code, Active: true}); err != nil {
		return coremodel.OrderAnalysis{}, err
	}
	return store.SaveOrderAnalysis(coremodel.OrderAnalysis{OrderID: order.ID, AnalyteTag: code, AnalyteName: code, Status: "scheduled", SourceFile: "zonci-unmatched-result", Flags: map[string]interface{}{"zonci_code": code, "wisemed_match": "missing"}})
}

func (m *Module) findOrder(sampleID string) (coremodel.Order, []coremodel.OrderAnalysis, bool) {
	return m.findOrderForDate(sampleID, "")
}

func (m *Module) findOrderForDate(sampleID, preferredDate string) (coremodel.Order, []coremodel.OrderAnalysis, bool) {
	store := m.store()
	if store == nil {
		return coremodel.Order{}, nil, false
	}
	if strings.TrimSpace(preferredDate) != "" {
		if orders, err := store.ListOrders(0, preferredDate); err == nil {
			for _, order := range orders {
				if order.SampleID == sampleID || order.FileID == sampleID {
					analyses, _ := store.ListOrderAnalyses(order.ID)
					return order, analyses, true
				}
			}
		}
	}
	for offset := 0; offset < m.readConfig().LookupDays; offset++ {
		orders, err := store.ListOrders(0, time.Now().AddDate(0, 0, -offset).Format("2006-01-02"))
		if err != nil {
			continue
		}
		for _, order := range orders {
			if order.SampleID == sampleID || order.FileID == sampleID {
				analyses, _ := store.ListOrderAnalyses(order.ID)
				return order, analyses, true
			}
		}
	}
	return coremodel.Order{}, nil, false
}

func (m *Module) isBlocked(tag, specimenCode string) bool {
	if filter := m.dailyAnalysisFilter(); filter != nil {
		return filter.IsAnalysisSendBlocked(time.Now().Format("2006-01-02"), tag, specimenCode)
	}
	return false
}

func (m *Module) readConfig() config {
	tcp, serialSettings, protocol := m.rt.ModuleSettings("transport-tcpip"), m.rt.ModuleSettings("transport-serial"), m.rt.ModuleSettings(m.ID())
	return config{CommType: firstNonEmpty(stringValue(m.rt.ModuleSettings("analyzer")["comm_type"]), "tcpip"), TCPHost: firstNonEmpty(stringValue(tcp["host"]), "0.0.0.0"), TCPPort: firstNonEmpty(stringValue(tcp["port"]), "3125"), SerialPort: stringValue(serialSettings["port"]), SerialBaud: intValue(serialSettings["baud"], 115200), LookupDays: intValue(protocol["order_lookup_days"], 7), ResultSeparator: zonciSeparator(stringValue(protocol["result_separator"]))}
}
func (m *Module) store() store {
	value, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	service, _ := value.(store)
	return service
}
func (m *Module) wiseMED() wiseMEDLookup {
	value, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil
	}
	service, _ := value.(wiseMEDLookup)
	return service
}
func (m *Module) dailyAnalysisFilter() dailyAnalysisFilter {
	value, ok := m.rt.Service("daily-analysis-send-filters")
	if !ok {
		return nil
	}
	service, _ := value.(dailyAnalysisFilter)
	return service
}
func (m *Module) audit(level, eventType, message string, meta map[string]interface{}) {
	if value, ok := m.rt.Service("storage"); ok {
		if logger, ok := value.(auditLogger); ok {
			_ = logger.AppendAuditLog(level, "zonci-xl1000i", eventType, message, meta)
		}
	}
}
func mapSlice(value interface{}) []map[string]interface{} {
	list, _ := value.([]interface{})
	out := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if row, ok := item.(map[string]interface{}); ok {
			out = append(out, row)
		}
	}
	return out
}
func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func intValue(value interface{}, fallback int) int {
	var out int
	if _, err := fmt.Sscan(stringValue(value), &out); err == nil && out > 0 {
		return out
	}
	return fallback
}
func normalizeValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.Count(value, ",") == 1 && !strings.Contains(value, ".") {
		return strings.Replace(value, ",", ".", 1)
	}
	return value
}
func zonciResultFields(line, separator string) []string {
	return strings.Split(line, zonciSeparator(separator))
}
func zonciMetadata(lines []string, separator string) map[string]string {
	metadata := map[string]string{}
	for _, line := range lines {
		fields := zonciResultFields(line, separator)
		if len(fields) < 2 {
			continue
		}
		if key := strings.ToLower(strings.TrimSpace(fields[0])); zonciMetadataKey(key) {
			metadata[key] = strings.TrimSpace(fields[1])
		}
	}
	return metadata
}
func zonciMetadataKey(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sampleno", "patientno", "name", "sex", "age", "sampletype", "date":
		return true
	default:
		return false
	}
}
func zonciResultDate(lines []string, separator string) string {
	for _, line := range lines {
		fields := zonciResultFields(line, separator)
		if len(fields) >= 2 && strings.EqualFold(strings.TrimSpace(fields[0]), "date") {
			return normalizeOrderDate(strings.TrimSpace(fields[1]))
		}
	}
	return ""
}
func zonciSeparator(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "VIRGULA", "COMMA":
		return ","
	case "TAB", "\\T", "0X09":
		return "\t"
	case "SPATIU", "SPACE":
		return " "
	}
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return ","
}
func zonciSeparatorLabel(value string) string {
	switch zonciSeparator(value) {
	case ",":
		return "VIRGULA"
	case "\t":
		return "TAB"
	case " ":
		return "SPATIU"
	}
	return "CARACTER DEFINIT DE UTILIZATOR"
}
func normalizeOrderDate(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "02/01/2006", "02.01.2006"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return value
}
func mergeFlags(base, add map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range add {
		out[key] = value
	}
	return out
}
func printable(value string) string {
	return strings.NewReplacer("\t", "<TAB>", "\n", "<LF>", "\r", "<CR>").Replace(value)
}
