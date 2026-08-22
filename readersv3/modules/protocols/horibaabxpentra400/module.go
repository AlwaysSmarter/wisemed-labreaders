package horibaabxpentra400

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
	sharedastm "wisemed-labreaders/readersv3/shared/astm"
)

const (
	enq = byte(0x05)
	ack = byte(0x06)
	nak = byte(0x15)
	stx = byte(0x02)
	etx = byte(0x03)
	eot = byte(0x04)
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
	Port, SenderName string
	Baud, DataBits   int
	OrderLookupDays  int
	SpecimenType     string
	SpecimenSettings map[string]interface{}
	Parity           serial.Parity
	StopBits         serial.StopBits
}

type result struct{ sampleID, code, tag, name, value, unit, timestamp string }
type queryStatus struct {
	SampleID       string    `json:"sample_id"`
	QueriedAt      time.Time `json:"queried_at,omitempty"`
	FinalizedAt    time.Time `json:"finalized_at,omitempty"`
	Finalized      bool      `json:"finalized"`
	Results        int       `json:"results"`
	Expected       int       `json:"expected_analyses"`
	Completed      int       `json:"completed_analyses"`
	LastError      string    `json:"last_error,omitempty"`
	OrderAvailable bool      `json:"order_available"`
}

type Module struct {
	rt     module.Runtime
	mu     sync.RWMutex
	status map[string]queryStatus
}

func New() module.Module     { return &Module{status: map[string]queryStatus{}} }
func (m *Module) ID() string { return "protocol-horiba-abx-pentra400" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: m.ID(), Group: "admin", Label: "Protocol HORIBA ABX Pentra 400", Path: "/settings/protocol/horiba-abx-pentra400", Order: 46})
	rt.Handle("/api/protocol/horiba-abx-pentra400/query-status", http.HandlerFunc(m.handleQueryStatus))
	rt.Handle("/protocol/horiba-abx-pentra400/query", http.HandlerFunc(m.handleQueryPage))
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	for {
		cfg := m.readConfig()
		if cfg.Port == "" {
			m.rt.Logf("horiba-abx-pentra400 idle: serial port is not configured")
		} else if err := m.run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			m.rt.Logf("horiba-abx-pentra400 serial error: %v", err)
			m.audit("error", "horiba_serial_error", "Eroare comunicare seriala Pentra", map[string]interface{}{"error": err.Error(), "port": cfg.Port})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func (m *Module) run(ctx context.Context, cfg config) error {
	port, err := serial.Open(cfg.Port, &serial.Mode{BaudRate: cfg.Baud, DataBits: cfg.DataBits, Parity: cfg.Parity, StopBits: cfg.StopBits})
	if err != nil {
		return fmt.Errorf("open %s: %w", cfg.Port, err)
	}
	defer port.Close()
	if err := port.SetReadTimeout(500 * time.Millisecond); err != nil {
		return err
	}
	m.rt.Logf("horiba-abx-pentra400 serial connected port=%s baud=%d", cfg.Port, cfg.Baud)
	m.audit("info", "horiba_serial_connected", "Comunicare seriala Pentra conectata", map[string]interface{}{"port": cfg.Port, "baud": cfg.Baud})

	var records []string
	frame := make([]byte, 0, 512)
	inFrame := false
	pending := [][]byte(nil)
	buf := make([]byte, 512)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		n, readErr := port.Read(buf)
		for _, b := range buf[:n] {
			switch b {
			case enq:
				records, frame, inFrame = nil, frame[:0], false
				if err := m.writeSerial(port, []byte{ack}); err != nil {
					return err
				}
			case stx:
				frame, inFrame = frame[:0], true
			case etx:
				if inFrame {
					records = append(records, parseFrame(frame))
					inFrame = false
					if err := m.writeSerial(port, []byte{ack}); err != nil {
						return err
					}
				}
			case eot:
				m.logPackage(records)
				if sampleID := querySampleID(records); sampleID != "" {
					m.audit("info", "horiba_host_query", "Interogare host primita de la Pentra", map[string]interface{}{"sample_id": sampleID})
					pending = m.orderReply(sampleID, cfg)
					if len(pending) > 0 {
						if err := m.writeSerial(port, []byte{enq}); err != nil {
							return err
						}
					}
				} else if err := m.saveResults(records, cfg.Port); err != nil && len(records) > 0 {
					m.rt.Logf("horiba-abx-pentra400 packet rejected: %v", err)
					m.audit("error", "horiba_packet_rejected", "Pachet Pentra respins", map[string]interface{}{"error": err.Error(), "records": records})
				}
				records = nil
			case ack:
				if len(pending) > 0 {
					if err := m.writeSerial(port, pending[0]); err != nil {
						return err
					}
					pending = pending[1:]
				} else {
					if err := m.writeSerial(port, []byte{eot}); err != nil {
						return err
					}
				}
			case nak:
				pending = nil
			default:
				if inFrame {
					frame = append(frame, b)
				}
			}
		}
		if readErr != nil {
			return fmt.Errorf("read serial: %w", readErr)
		}
	}
}

func parseFrame(frame []byte) string {
	text := strings.TrimSpace(string(frame))
	if len(text) > 1 && text[0] >= '0' && text[0] <= '7' {
		text = text[1:]
	}
	return strings.TrimSpace(text)
}

func (m *Module) orderReply(sampleID string, cfg config) [][]byte {
	status := queryStatus{SampleID: sampleID, QueriedAt: time.Now()}
	order, analyses, ok := m.findOrder(sampleID)
	// A previous host query may have created an empty order because the catalog
	// did not yet expose compatible tests. Refresh it before returning an ASTM reply.
	if !ok || len(analyses) == 0 {
		var err error
		order, analyses, err = m.loadOrderFromWiseMED(sampleID)
		if err != nil {
			status.LastError = err.Error()
			m.setStatus(status)
			m.rt.Logf("horiba-abx-pentra400 host query sample_id=%s: %v", sampleID, err)
			return m.emptyOrderReply(cfg)
		}
		ok = true
	}
	status.OrderAvailable = ok
	status.LastError = ""
	status.Expected = len(analyses)
	m.setStatus(status)
	testGroups := m.orderTestGroups(analyses, cfg, stringValue(order.Meta["wisemed_specimen_code"]))
	groupCodes := make([]string, 0, len(testGroups))
	totalTests := 0
	for specimenCode, tests := range testGroups {
		groupCodes = append(groupCodes, specimenCode)
		totalTests += len(tests)
	}
	sort.Strings(groupCodes)
	patient := strings.ReplaceAll(strings.TrimSpace(order.PatientName), " ", "^")
	if patient == "" {
		patient = sampleID
	}
	records := []string{
		"H|\\^&|||" + cfg.SenderName + "|||||||P|E1394-97|" + time.Now().Format("20060102150405"),
		"P|1||" + sampleID + "||" + patient,
	}
	for _, specimenCode := range groupCodes {
		records = append(records, buildOrderRecord(sampleID, testGroups[specimenCode], specimenCode, time.Now()))
	}
	records = append(records, "L|1|N")
	m.markOrderSentToAnalyzer(order, analyses, totalTests)
	frames := make([][]byte, 0, len(records))
	for index, record := range records {
		frames = append(frames, frame(record, byte((index+1)%8)))
	}
	m.rt.Logf("horiba-abx-pentra400 host query sample_id=%s: sent %d analyses in %d specimen group(s)", sampleID, totalTests, len(groupCodes))
	m.audit("info", "horiba_order_reply", "Raspuns ordin trimis catre Pentra", map[string]interface{}{"sample_id": sampleID, "analyses": totalTests, "test_groups": testGroups, "wisemed_specimen_code": stringValue(order.Meta["wisemed_specimen_code"])})
	return frames
}

// buildOrderRecord follows the actual ABX Pentra 400 trace from the old Delphi reader.
// Pentra requires O.16 (Specimen Descriptor/Sample type); this equipment uses 1.
func buildOrderRecord(sampleID string, tests []string, specimenType string, now time.Time) string {
	fields := make([]string, 31)
	fields[0] = "O"
	fields[1] = "1"
	fields[2] = sampleID
	fields[4] = strings.Join(tests, "\\")
	fields[5] = "R"
	fields[7] = now.Format("20060102150405")
	fields[11] = "A"
	fields[15] = firstNonEmpty(strings.TrimSpace(specimenType), "1")
	fields[22] = now.Format("20060102150405")
	fields[25] = "O"
	return strings.Join(fields, "|")
}

// Pentra order records use PARAMETRI_ECHIPAMENTE.pe_codificare, not the WiseMED tag.
// O.16 applies to an ASTM order segment, so tests are grouped by specimen code.
func (m *Module) orderTestGroups(analyses []coremodel.OrderAnalysis, cfg config, fallbackSampleCode string) map[string][]string {
	codesByTag := map[string]string{}
	if service := m.store(); service != nil {
		if analytes, err := service.ListAnalytes(); err == nil {
			for _, analyte := range analytes {
				if tag := strings.TrimSpace(analyte.Tag); tag != "" {
					codesByTag[tag] = strings.TrimSpace(analyte.Code)
				}
			}
		}
	}
	seen := map[string]map[string]bool{}
	groups := map[string][]string{}
	for _, item := range analyses {
		tag := strings.TrimSpace(item.AnalyteTag)
		code := firstNonEmpty(codesByTag[tag], tag)
		wisemedSpecimenCode := firstNonEmpty(stringValue(item.Flags["wisemed_sample_code"]), fallbackSampleCode)
		if filter := m.dailyAnalysisFilter(); filter != nil && filter.IsAnalysisSendBlocked(time.Now().Format("2006-01-02"), tag, wisemedSpecimenCode) {
			m.rt.Logf("horiba-abx-pentra400 filtered analyte=%s specimen=%s", tag, wisemedSpecimenCode)
			continue
		}
		specimenCode := sharedastm.ResolveSpecimenCode(cfg.SpecimenSettings, wisemedSpecimenCode, cfg.SpecimenType)
		if code == "" || specimenCode == "" {
			continue
		}
		if seen[specimenCode] == nil {
			seen[specimenCode] = map[string]bool{}
		}
		if seen[specimenCode][code] {
			continue
		}
		seen[specimenCode][code] = true
		groups[specimenCode] = append(groups[specimenCode], "^^^"+code)
	}
	return groups
}

func (m *Module) markOrderSentToAnalyzer(order coremodel.Order, analyses []coremodel.OrderAnalysis, sentTests int) {
	store := m.store()
	if store == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, analysis := range analyses {
		flags := copyFlags(analysis.Flags)
		flags["analyzer_send_status"] = "sent"
		flags["analyzer_sent_at"] = now
		flags["analyzer_send_count"] = numberValue(flags["analyzer_send_count"]) + 1
		analysis.Flags = flags
		if _, err := store.SaveOrderAnalysis(analysis); err != nil {
			m.rt.Logf("horiba-abx-pentra400 cannot mark analysis %d sent: %v", analysis.ID, err)
		}
	}
	meta := copyFlags(order.Meta)
	meta["analyzer_send_status"] = "sent"
	meta["analyzer_sent_at"] = now
	meta["analyzer_send_count"] = numberValue(meta["analyzer_send_count"]) + 1
	meta["analyzer_sent_analyses"] = sentTests
	order.Meta = meta
	if _, err := store.UpsertOrder(order); err != nil {
		m.rt.Logf("horiba-abx-pentra400 cannot mark order %d sent: %v", order.ID, err)
	}
}

func copyFlags(source map[string]interface{}) map[string]interface{} {
	copy := map[string]interface{}{}
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func numberValue(value interface{}) int {
	var number int
	_, _ = fmt.Sscan(fmt.Sprint(value), &number)
	return number
}

func (m *Module) emptyOrderReply(cfg config) [][]byte {
	m.audit("info", "horiba_order_reply", "Raspuns gol trimis catre Pentra", map[string]interface{}{"analyses": 0})
	return [][]byte{
		frame("H|\\^&|||"+cfg.SenderName+"|||||||P|E1394-97|"+time.Now().Format("20060102150405"), 1),
		frame("L|1|N", 2),
	}
}

// loadOrderFromWiseMED makes ASTM host queries self-sufficient: an analyzer can
// request a file before the daily order list has been manually synchronized.
func (m *Module) loadOrderFromWiseMED(sampleID string) (coremodel.Order, []coremodel.OrderAnalysis, error) {
	store := m.store()
	if store == nil {
		return coremodel.Order{}, nil, errors.New("storage service unavailable")
	}
	wiseMED := m.wiseMED()
	if wiseMED == nil {
		return coremodel.Order{}, nil, errors.New("wisemed api service unavailable")
	}
	equipmentID := strings.TrimSpace(wiseMED.Settings()["echipament_id"])
	if equipmentID == "" {
		return coremodel.Order{}, nil, errors.New("missing echipament_id in WiseMED settings")
	}

	response, err := wiseMED.FetchFileForAnalyzer(sampleID, equipmentID)
	if err != nil {
		return coremodel.Order{}, nil, fmt.Errorf("WiseMED file lookup failed: %w", err)
	}
	orderDate := firstNonEmpty(stringValue(response["o_file_date"]), time.Now().Format("2006-01-02"))
	roundNo, err := store.CurrentRoundNo(orderDate)
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	order, err := store.UpsertOrder(coremodel.Order{
		RoundNo:     roundNo,
		OrderDate:   orderDate,
		SampleID:    sampleID,
		FileID:      firstNonEmpty(stringValue(response["o_file_id"]), sampleID),
		PatientID:   stringValue(response["o_patient_id"]),
		PatientName: stringValue(response["o_patient_name"]),
		Status:      "scheduled",
		SourceFile:  "wisemed-host-query",
		Meta: map[string]interface{}{
			"source":                "wisemed-host-query",
			"equipment_id":          equipmentID,
			"wisemed_specimen_code": firstNonEmpty(stringValue(response["o_specimen_code"]), stringValue(response["o_sample_type_code"])),
		},
	})
	if err != nil {
		return coremodel.Order{}, nil, err
	}

	existingAnalytes, err := store.ListAnalytes()
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	analytesByTag := make(map[string]coremodel.Analyte, len(existingAnalytes))
	for _, analyte := range existingAnalytes {
		if tag := strings.TrimSpace(analyte.Tag); tag != "" {
			analytesByTag[tag] = analyte
		}
	}
	for _, item := range mapSlice(response["o_tests"]) {
		tag := strings.TrimSpace(stringValue(item["t_tag"]))
		if tag == "" {
			continue
		}
		if filter := m.dailyAnalysisFilter(); filter != nil && filter.IsAnalysisSendBlocked(time.Now().Format("2006-01-02"), tag, firstNonEmpty(stringValue(item["t_sample_code"]), stringValue(response["o_specimen_code"]))) {
			m.rt.Logf("horiba-abx-pentra400 filtered WiseMED test=%s", tag)
			continue
		}
		sourceName := stringValue(item["t_name"])
		name := firstNonEmpty(sourceName, tag)
		analyte, exists := analytesByTag[tag]
		if !exists {
			analyte = coremodel.Analyte{Tag: tag, Name: name, Active: true}
		}
		analyte.Code = firstNonEmpty(stringValue(item["t_code"]), analyte.Code, tag)
		analyte.Name = firstNonEmpty(sourceName, analyte.Name, tag)
		analyte.Active = true
		if _, err := store.SaveAnalyte(analyte); err != nil {
			return coremodel.Order{}, nil, fmt.Errorf("cannot save analyzer test %s: %w", tag, err)
		}
		if _, err := store.SaveOrderAnalysis(coremodel.OrderAnalysis{
			OrderID:      order.ID,
			AnalyteTag:   tag,
			AnalyteName:  name,
			WiseMEDSMID:  stringValue(item["t_sm_id"]),
			WiseMEDFSMID: stringValue(item["t_fsm_id"]),
			Status:       "scheduled",
			SourceFile:   "wisemed-host-query",
			Flags: map[string]interface{}{
				"wisemed_sample_code": firstNonEmpty(stringValue(item["t_sample_code"]), stringValue(response["o_specimen_code"])),
			},
		}); err != nil {
			return coremodel.Order{}, nil, fmt.Errorf("cannot save order test %s: %w", tag, err)
		}
	}
	analyses, err := store.ListOrderAnalyses(order.ID)
	if err != nil {
		return coremodel.Order{}, nil, err
	}
	m.rt.Logf("horiba-abx-pentra400 host query sample_id=%s: created WiseMED order id=%d with %d analyses", sampleID, order.ID, len(analyses))
	return order, analyses, nil
}

func frame(record string, sequence byte) []byte {
	body := append([]byte{byte('0' + sequence)}, []byte(record)...)
	checksumInput := append(append([]byte{}, body...), etx)
	sum := 0
	for _, item := range checksumInput {
		sum += int(item)
	}
	return append(append(append([]byte{stx}, checksumInput...), []byte(fmt.Sprintf("%02X", sum%256))...), '\r', '\n')
}

func querySampleID(records []string) string {
	for _, record := range records {
		fields := strings.Split(record, "|")
		if len(fields) < 3 || fields[0] != "Q" {
			continue
		}
		parts := strings.Split(fields[2], "^")
		for _, part := range parts {
			if value := strings.TrimSpace(part); value != "" {
				return value
			}
		}
	}
	return ""
}

func (m *Module) saveResults(records []string, sourcePort string) error {
	sampleID, results := "", []result{}
	for _, record := range records {
		fields := strings.Split(record, "|")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "P":
			if len(fields) > 3 && sampleID == "" {
				sampleID = strings.TrimSpace(fields[3])
			}
		case "O":
			if len(fields) > 2 {
				sampleID = firstNonEmpty(strings.TrimSpace(fields[2]), sampleID)
			}
		case "R":
			if len(fields) > 3 {
				parts := strings.Split(fields[2], "^")
				code, tag, name := pentraAnalyteIdentifier(parts)
				value, unit, timestamp := strings.TrimSpace(fields[3]), "", ""
				if len(fields) > 4 {
					unit = strings.TrimSpace(fields[4])
				}
				if len(fields) > 12 {
					timestamp = strings.TrimSpace(fields[12])
				}
				results = append(results, result{sampleID: sampleID, code: code, tag: tag, name: name, value: value, unit: unit, timestamp: timestamp})
			}
		}
	}
	if sampleID == "" || len(results) == 0 {
		return errors.New("no Pentra results in ASTM package")
	}
	store := m.store()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	known, _ := store.ListAnalytes()
	knownTags := map[string]bool{}
	for _, item := range known {
		knownTags[item.Tag] = true
	}
	runDate := time.Now().Format("2006-01-02")
	round, err := store.CurrentRoundNo(runDate)
	if err != nil {
		return err
	}
	orderIDs := []int64{}
	for _, item := range results {
		if item.tag == "" || item.value == "" {
			continue
		}
		if !knownTags[item.tag] {
			if _, err := store.SaveAnalyte(coremodel.Analyte{Code: item.code, Tag: item.tag, Name: firstNonEmpty(item.name, item.tag), Active: true}); err != nil {
				return err
			}
			knownTags[item.tag] = true
		}
		order, _, _, err := store.RecordImportedResult(runDate, round, coremodel.ImportedRecord{SampleID: sampleID, FileID: sampleID, AnalyteTag: item.tag, AnalyteName: firstNonEmpty(item.name, item.tag), ResultValue: item.value, RawValue: item.value, Unit: item.unit, Meta: map[string]interface{}{"protocol": "horiba-abx-pentra400", "analyzer_code": item.code, "result_timestamp": item.timestamp}}, "serial:"+sourcePort)
		if err != nil {
			return err
		}
		orderIDs = append(orderIDs, order.ID)
	}
	if len(orderIDs) == 0 {
		return errors.New("no importable Pentra results")
	}
	if err := store.ReapplyOrderTransformations(orderIDs); err != nil {
		return err
	}
	m.setStatus(queryStatus{SampleID: sampleID, Finalized: true, FinalizedAt: time.Now(), Results: len(orderIDs), OrderAvailable: true})
	m.rt.Logf("horiba-abx-pentra400 import ok sample_id=%s results=%d", sampleID, len(orderIDs))
	m.audit("info", "horiba_results_imported", "Rezultate Pentra preluate", map[string]interface{}{"sample_id": sampleID, "results": len(orderIDs)})
	return nil
}

// Pentra sends test identifiers as ^^^numeric-code^readable-tag.
func pentraAnalyteIdentifier(parts []string) (code, tag, name string) {
	if len(parts) > 4 {
		code, tag = strings.TrimSpace(parts[3]), strings.TrimSpace(parts[4])
	}
	if tag == "" && len(parts) > 0 {
		tag = strings.TrimSpace(parts[len(parts)-1])
	}
	return code, tag, tag
}

func (m *Module) findOrder(sampleID string) (coremodel.Order, []coremodel.OrderAnalysis, bool) {
	service := m.store()
	if service == nil {
		return coremodel.Order{}, nil, false
	}
	lookupDays := intSetting(m.rt.ModuleSettings(m.ID())["order_lookup_days"], 7)
	for offset := 0; offset < lookupDays; offset++ {
		orders, err := service.ListOrders(0, time.Now().AddDate(0, 0, -offset).Format("2006-01-02"))
		if err != nil {
			continue
		}
		for _, order := range orders {
			if order.SampleID == sampleID || order.FileID == sampleID || order.PatientID == sampleID {
				analyses, _ := service.ListOrderAnalyses(order.ID)
				return order, analyses, true
			}
		}
	}
	return coremodel.Order{}, nil, false
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

func mapSlice(value interface{}) []map[string]interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]interface{}); ok {
			out = append(out, row)
		}
	}
	return out
}

func stringValue(value interface{}) string {
	return strings.TrimSpace(fmt.Sprint(value))
}
func (m *Module) setStatus(status queryStatus) {
	m.mu.Lock()
	m.status[status.SampleID] = status
	m.mu.Unlock()
}
func (m *Module) logPackage(records []string) {
	if intSetting(m.rt.ModuleSettings("logging")["verbose_level"], 1) >= 5 && len(records) > 0 {
		m.rt.Logf("PARSED SERIAL PACKAGE: TEXT: %s", strings.Join(records, "\\r"))
	}
}

func (m *Module) writeSerial(port serial.Port, payload []byte) error {
	if m.verboseEnabled(5) {
		visible := serialPayload(payload)
		m.rt.Logf("horiba-abx-pentra400 serial tx: %s", visible)
		m.audit("debug", "horiba_serial_tx", "Cadru serial transmis catre Pentra", map[string]interface{}{"payload": visible, "bytes": len(payload)})
	}
	_, err := port.Write(payload)
	return err
}

func (m *Module) verboseEnabled(level int) bool {
	return intSetting(m.rt.ModuleSettings("logging")["verbose_level"], 1) >= level
}

func (m *Module) audit(level, eventType, message string, meta map[string]interface{}) {
	value, ok := m.rt.Service("storage")
	if !ok {
		return
	}
	logger, ok := value.(auditLogger)
	if !ok {
		return
	}
	_ = logger.AppendAuditLog(level, "horiba-abx-pentra400", eventType, message, meta)
}

func serialPayload(payload []byte) string {
	replacer := strings.NewReplacer(
		string(stx), "<STX>", string(etx), "<ETX>", string(enq), "<ENQ>",
		string(ack), "<ACK>", string(nak), "<NAK>", string(eot), "<EOT>",
		"\r", "<CR>", "\n", "<LF>",
	)
	return replacer.Replace(string(payload))
}

func (m *Module) handleQueryStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sampleID := strings.TrimSpace(r.URL.Query().Get("sample_id"))
	if sampleID == "" {
		http.Error(w, "sample_id is required", http.StatusBadRequest)
		return
	}
	m.mu.RLock()
	status := m.status[sampleID]
	m.mu.RUnlock()
	status.SampleID = sampleID
	if _, analyses, ok := m.findOrder(sampleID); ok {
		status.OrderAvailable = true
		status.Expected = len(analyses)
		status.Completed = completedAnalyses(analyses)
		if status.Expected > 0 && status.Completed >= status.Expected {
			status.Finalized = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "status": status})
}

func (m *Module) handleQueryPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html lang="ro"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HORIBA ABX Pentra 400 - verificare</title><style>body{font:16px Georgia,serif;max-width:680px;margin:3rem auto;padding:0 1rem;color:#18352a;background:#f5f1e8}input,button{font:inherit;padding:.6rem}button{background:#1f6751;color:#fff;border:0}pre{padding:1rem;background:#fff;border:1px solid #cdc5b5;white-space:pre-wrap}</style></head><body><h1>Verificare rezultate Pentra 400</h1><p>Introduceți barcode-ul/proba programată. Pagina arată dacă există cererea și dacă analizele au rezultate finalizate.</p><form id="form"><input id="sample" required placeholder="Barcode / ID probă"><button>Verifică</button></form><pre id="out">Așteptare interogare.</pre><script>document.querySelector('#form').onsubmit=async e=>{e.preventDefault();const id=document.querySelector('#sample').value.trim();const r=await fetch('/api/protocol/horiba-abx-pentra400/query-status?sample_id='+encodeURIComponent(id));document.querySelector('#out').textContent=await r.text()}</script></body></html>`))
}

func completedAnalyses(analyses []coremodel.OrderAnalysis) int {
	completed := 0
	for _, analysis := range analyses {
		if strings.TrimSpace(analysis.ResultValue) != "" || strings.TrimSpace(analysis.RawValue) != "" {
			completed++
		}
	}
	return completed
}

func (m *Module) readConfig() config {
	settings := m.rt.ModuleSettings("transport-serial")
	protocol := m.rt.ModuleSettings(m.ID())
	astmSettings := m.rt.ModuleSettings("protocol-astm")
	return config{Port: strings.TrimSpace(asString(settings["port"])), Baud: intSetting(settings["baud"], 9600), DataBits: intSetting(settings["data_bits"], 8), Parity: parseParity(asString(settings["parity"])), StopBits: parseStopBits(asString(settings["stop_bits"])), SenderName: firstNonEmpty(strings.TrimSpace(asString(protocol["sender_name"])), "WISEMED"), OrderLookupDays: intSetting(protocol["order_lookup_days"], 7), SpecimenType: firstNonEmpty(strings.TrimSpace(asString(astmSettings["specimen_code_default"])), strings.TrimSpace(asString(protocol["specimen_type"])), "1"), SpecimenSettings: astmSettings}
}
func parseParity(value string) serial.Parity {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "odd":
		return serial.OddParity
	case "even":
		return serial.EvenParity
	default:
		return serial.NoParity
	}
}
func parseStopBits(value string) serial.StopBits {
	if strings.TrimSpace(value) == "2" {
		return serial.TwoStopBits
	}
	return serial.OneStopBit
}
func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func intSetting(value interface{}, fallback int) int {
	var out int
	if _, err := fmt.Sscan(asString(value), &out); err == nil && out > 0 {
		return out
	}
	return fallback
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
