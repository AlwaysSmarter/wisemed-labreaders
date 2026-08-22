package biomerieuxminividas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.bug.st/serial"

	"wisemed-labreaders/readersv3/core/module"
	coremodel "wisemed-labreaders/readersv3/modules/core/model"
)

const (
	ctrlENQ = byte(0x05)
	ctrlACK = byte(0x06)
	ctrlSTX = byte(0x02)
	ctrlETX = byte(0x03)
	ctrlEOT = byte(0x04)
	ctrlRS  = byte(0x1e)
	ctrlGS  = byte(0x1d)
)

type importStore interface {
	CurrentRoundNo(orderDate string) (int, error)
	RecordImportedResult(orderDate string, roundNo int, rec coremodel.ImportedRecord, sourceFile string) (coremodel.Order, coremodel.OrderAnalysis, coremodel.OrderAnalysisResult, error)
	ReapplyOrderTransformations(orderIDs []int64) error
	ListAnalytes() ([]coremodel.Analyte, error)
	SaveAnalyte(item coremodel.Analyte) (coremodel.Analyte, error)
}

type miniVidasConfig struct {
	Port                string
	Baud                int
	DataBits            int
	StopBits            serial.StopBits
	Parity              serial.Parity
	QualitativeAnalytes map[string]bool
}

type miniVidasResult struct {
	SampleID     string
	AnalyteTag   string
	AnalyteName  string
	Qualitative  string
	Quantitative string
	ResultDate   string
	ResultTime   string
}

type Module struct {
	rt         module.Runtime
	mu         sync.Mutex
	lastError  string
	lastImport int
}

func New() module.Module { return &Module{} }

func (m *Module) ID() string { return "protocol-biomerieux-minividas" }

func (m *Module) Init(rt module.Runtime) error {
	m.rt = rt
	rt.AddMenu(module.MenuEntry{ID: m.ID(), Group: "admin", Label: "Protocol BioMerieux miniVIDAS", Path: "/settings/protocol/biomerieux-minividas", Order: 45})
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	for {
		cfg := m.readConfig()
		if cfg.Port == "" {
			m.rt.Logf("biomerieux-minividas protocol idle: serial port is not configured")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		if err := m.runSerial(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			m.setError(err)
			m.rt.Logf("biomerieux-minividas serial error: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}

func (m *Module) runSerial(ctx context.Context, cfg miniVidasConfig) error {
	port, err := serial.Open(cfg.Port, &serial.Mode{BaudRate: cfg.Baud, DataBits: cfg.DataBits, Parity: cfg.Parity, StopBits: cfg.StopBits})
	if err != nil {
		return fmt.Errorf("open serial port %s: %w", cfg.Port, err)
	}
	defer port.Close()
	if err := port.SetReadTimeout(500 * time.Millisecond); err != nil {
		return fmt.Errorf("set serial read timeout: %w", err)
	}
	m.rt.Logf("biomerieux-minividas serial connected port=%s baud=%d data_bits=%d", cfg.Port, cfg.Baud, cfg.DataBits)

	buf := make([]byte, 512)
	packet := make([]byte, 0, 2048)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		n, readErr := port.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				switch b {
				case ctrlENQ:
					if _, err := port.Write([]byte{ctrlACK}); err != nil {
						return fmt.Errorf("send ACK after ENQ: %w", err)
					}
					packet = packet[:0]
				case ctrlETX:
					packet = append(packet, b)
					if _, err := port.Write([]byte{ctrlACK}); err != nil {
						return fmt.Errorf("send ACK after ETX: %w", err)
					}
				case ctrlEOT:
					m.logParsedPackage(packet)
					if err := m.processPacket(packet, cfg); err != nil {
						m.setError(err)
						m.rt.Logf("biomerieux-minividas packet rejected: %v", err)
					}
					packet = packet[:0]
				default:
					packet = append(packet, b)
				}
			}
		}
		if readErr != nil {
			return fmt.Errorf("read serial port %s: %w", cfg.Port, readErr)
		}
	}
}

func (m *Module) logParsedPackage(packet []byte) {
	if m.verboseLevel() < 5 {
		return
	}
	text := strings.NewReplacer(
		"\x02", "<STX>",
		"\x03", "<ETX>",
		"\x04", "<EOT>",
		"\x1e", "<RS>",
		"\x1d", "<GS>",
		"\r", "\\r",
		"\n", "\\n",
	).Replace(string(packet))
	m.rt.Logf("PARSED SERIAL PACKAGE: TEXT: %s", text)
}

func (m *Module) verboseLevel() int {
	level := intSetting(m.rt.ModuleSettings("logging")["verbose_level"], 1)
	if level <= 0 {
		return 1
	}
	return level
}

func (m *Module) processPacket(packet []byte, cfg miniVidasConfig) error {
	results := parseMiniVidasPacket(packet)
	if len(results) == 0 {
		return errors.New("message contains no miniVIDAS result")
	}
	store := m.importStore()
	if store == nil {
		return errors.New("storage service unavailable")
	}
	known, _ := m.knownAnalytes()
	orderIDs := make([]int64, 0, len(results))
	imported := 0
	for _, item := range results {
		if item.SampleID == "" || item.AnalyteTag == "" {
			m.rt.Logf("biomerieux-minividas skip sample_id=%q analyte=%q", item.SampleID, item.AnalyteTag)
			continue
		}
		value := item.Quantitative
		if cfg.QualitativeAnalytes[normalizeTag(item.AnalyteTag)] || strings.TrimSpace(value) == "" {
			value = item.Qualitative
		}
		if strings.TrimSpace(value) == "" {
			m.rt.Logf("biomerieux-minividas skip sample_id=%s analyte=%s: result value is empty", item.SampleID, item.AnalyteTag)
			continue
		}
		name := firstNonEmpty(item.AnalyteName, item.AnalyteTag)
		if err := ensureAnalyte(store, known, item.AnalyteTag, name); err != nil {
			return err
		}
		runDate := miniVidasRunDate(item.ResultDate)
		roundNo, err := store.CurrentRoundNo(runDate)
		if err != nil {
			return err
		}
		order, _, _, err := store.RecordImportedResult(runDate, roundNo, coremodel.ImportedRecord{
			SampleID:    item.SampleID,
			FileID:      item.SampleID,
			AnalyteTag:  normalizeTag(item.AnalyteTag),
			AnalyteName: name,
			ResultValue: strings.TrimSpace(value),
			RawValue:    strings.TrimSpace(value),
			Interpreted: strings.TrimSpace(item.Qualitative),
			Meta:        map[string]interface{}{"protocol": "biomerieux-minividas", "result_time": item.ResultTime, "quantitative": item.Quantitative},
		}, "serial:"+cfg.Port)
		if err != nil {
			return err
		}
		orderIDs = append(orderIDs, order.ID)
		imported++
	}
	if imported == 0 {
		return errors.New("message parsed but produced no importable results")
	}
	if err := store.ReapplyOrderTransformations(orderIDs); err != nil {
		return err
	}
	m.mu.Lock()
	m.lastImport = imported
	m.lastError = ""
	m.mu.Unlock()
	m.rt.Logf("biomerieux-minividas import ok results=%d", imported)
	return nil
}

func parseMiniVidasPacket(packet []byte) []miniVidasResult {
	text := strings.Map(func(r rune) rune {
		switch byte(r) {
		case ctrlSTX, ctrlETX, ctrlEOT, '\r', '\n':
			return -1
		case ctrlRS, ctrlGS:
			return ' '
		default:
			return r
		}
	}, string(packet))
	fields := strings.Split(text, "|")
	results := []miniVidasResult{}
	current := miniVidasResult{}
	sampleID := ""
	flush := func() {
		if current.AnalyteTag == "" {
			return
		}
		current.SampleID = sampleID
		results = append(results, current)
		current = miniVidasResult{}
	}
	for _, raw := range fields {
		field := strings.TrimSpace(raw)
		if len(field) < 2 {
			continue
		}
		prefix := strings.ToLower(field[:2])
		value := strings.TrimSpace(field[2:])
		switch prefix {
		case "ci":
			sampleID = trimLeadingZeroes(value)
		case "rt":
			flush()
			current.AnalyteTag = value
		case "rn":
			current.AnalyteName = value
		case "ql":
			current.Qualitative = value
		case "qn":
			current.Quantitative = value
		case "td":
			current.ResultDate = value
		case "tt":
			current.ResultTime = value
		}
	}
	flush()
	return results
}

func (m *Module) readConfig() miniVidasConfig {
	settings := m.rt.ModuleSettings("transport-serial")
	protocol := m.rt.ModuleSettings(m.ID())
	qualitative := map[string]bool{}
	for _, item := range listSetting(protocol["qualitative_analytes"]) {
		qualitative[normalizeTag(item)] = true
	}
	return miniVidasConfig{
		Port:                strings.TrimSpace(asString(settings["port"])),
		Baud:                intSetting(settings["baud"], 38400),
		DataBits:            intSetting(settings["data_bits"], 8),
		StopBits:            stopBitsSetting(settings["stop_bits"]),
		Parity:              paritySetting(settings["parity"]),
		QualitativeAnalytes: qualitative,
	}
}

func paritySetting(value interface{}) serial.Parity {
	switch strings.ToLower(strings.TrimSpace(asString(value))) {
	case "odd":
		return serial.OddParity
	case "even":
		return serial.EvenParity
	case "mark":
		return serial.MarkParity
	case "space":
		return serial.SpaceParity
	default:
		return serial.NoParity
	}
}

func stopBitsSetting(value interface{}) serial.StopBits {
	switch strings.TrimSpace(asString(value)) {
	case "1.5":
		return serial.OnePointFiveStopBits
	case "2":
		return serial.TwoStopBits
	default:
		return serial.OneStopBit
	}
}

func (m *Module) importStore() importStore {
	service, ok := m.rt.Service("storage")
	if !ok {
		return nil
	}
	store, _ := service.(importStore)
	return store
}

func (m *Module) knownAnalytes() (map[string]coremodel.Analyte, error) {
	store := m.importStore()
	if store == nil {
		return map[string]coremodel.Analyte{}, nil
	}
	items, err := store.ListAnalytes()
	if err != nil {
		return nil, err
	}
	known := make(map[string]coremodel.Analyte, len(items))
	for _, item := range items {
		known[normalizeTag(item.Tag)] = item
	}
	return known, nil
}

func ensureAnalyte(store importStore, known map[string]coremodel.Analyte, tag, name string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return errors.New("empty analyte tag")
	}
	if _, ok := known[tag]; ok {
		return nil
	}
	saved, err := store.SaveAnalyte(coremodel.Analyte{Tag: tag, Name: firstNonEmpty(name, tag), Active: true})
	if err != nil {
		return err
	}
	known[tag] = saved
	return nil
}

func (m *Module) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = err.Error()
}

func miniVidasRunDate(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"01/02/06", "01/02/2006", "02/01/06", "02/01/2006", "20060102"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return time.Now().Format("2006-01-02")
}

func trimLeadingZeroes(value string) string {
	value = strings.TrimSpace(value)
	trimmed := strings.TrimLeft(value, "0")
	if trimmed == "" && value != "" {
		return "0"
	}
	return trimmed
}

func normalizeTag(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	underscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(unicode.ToUpper(r))
			underscore = false
			continue
		}
		if out.Len() > 0 && !underscore {
			out.WriteByte('_')
			underscore = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func intSetting(value interface{}, fallback int) int {
	var parsed int
	if _, err := fmt.Sscanf(strings.TrimSpace(asString(value)), "%d", &parsed); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func listSetting(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		if strings.TrimSpace(asString(value)) == "" {
			return nil
		}
		return []string{asString(value)}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item := strings.TrimSpace(asString(item)); item != "" {
			out = append(out, item)
		}
	}
	return out
}
