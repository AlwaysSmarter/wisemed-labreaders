package labnovationld560

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

type parsedMessage struct {
	InstrumentModel            string
	InstrumentSequentialNumber string
	RunDate                    string
	RunTime                    string
	Sample                     string
	SampleID                   string
	SampleNo                   string
	RackNo                     string
	RackPosition               string
	PatientID                  string
	PatientName                string
	FileID                     string
	SampleType                 string
	ControlLevel               string
	IsQC                       bool
	Results                    []parsedResult
	image                      *parsedImage
}

type parsedResult struct {
	AnalyteTag      string
	AnalyteName     string
	ResultValue     string
	RawValue        string
	Interpreted     string
	Unit            string
	Flags           map[string]interface{}
	ProtocolOptions map[string]interface{}
}

type parsedImage struct {
	tag      string
	name     string
	format   string
	encoding string
	data     []byte
}

type ignoreLogger func(kind, reason string, payload map[string]interface{})

type hl7Settings struct {
	Framing               string
	SampleIDPath          string
	SampleIDFallbackPaths []string
	SampleNoPath          string
	RackNoPath            string
	RackPositionPath      string
	RunDatePath           string
	PatientIDPath         string
	PatientNamePath       string
	QCIndicatorPath       string
	QCIndicatorValues     []string
	ResultIdentifierPath  string
	ResultNamePath        string
	ResultValuePath       string
	ResultUnitPath        string
	ResultInterpretedPath string
	AnalyteMappings       map[string]string
	AllowedResultIDs      []string
}

type simpleSettings struct {
	SampleModeQCValues []string
	AnalyteMappings    map[string]string
	AnalyteUnits       map[string]string
	ImageMode          string
}

type hl7Message struct {
	Segments []hl7Segment
}

type hl7Segment struct {
	Name            string
	Fields          []string
	FieldSeparator  string
	ComponentSep    string
	RepeatSep       string
	EscapeSep       string
	SubcomponentSep string
}

func defaultHL7Settings() map[string]interface{} {
	return map[string]interface{}{
		"framing":                  "mllp",
		"sample_id_path":           "OBR.3.1",
		"sample_id_fallback_paths": []interface{}{"OBR.2.1", "PID.3.1"},
		"sample_no_path":           "OBR.3.1",
		"rack_no_path":             "OBR.18.1",
		"rack_position_path":       "OBR.19.1",
		"run_date_path":            "OBR.7.1",
		"patient_id_path":          "PID.3.1",
		"patient_name_path":        "PID.5.1",
		"qc_indicator_path":        "OBR.11.1",
		"qc_indicator_values":      []interface{}{"Q", "QC", "1"},
		"result_identifier_path":   "OBX.3.1",
		"result_name_path":         "OBX.3.2",
		"result_value_path":        "OBX.5.1",
		"result_unit_path":         "OBX.6.1",
		"result_interpreted_path":  "OBX.8.1",
		"analyte_mappings":         map[string]interface{}{},
		"allowed_result_ids":       []interface{}{},
	}
}

func defaultSimpleSettings() map[string]interface{} {
	return map[string]interface{}{
		"sample_mode_qc_values": []interface{}{"1"},
		"analyte_mappings": map[string]interface{}{
			"HbA1a": "HbA1a",
			"HbA1b": "HbA1b",
			"HbF":   "HbF",
			"L-A1C": "L-A1C",
			"HbA1c": "HbA1c",
			"HbA0":  "HbA0",
			"eAG":   "eAG",
		},
		"analyte_units": map[string]interface{}{
			"HbA1a": "%",
			"HbA1b": "%",
			"HbF":   "%",
			"L-A1C": "%",
			"HbA1c": "%",
			"HbA0":  "%",
			"eAG":   "",
		},
		"image_mode": "no_image",
	}
}

func hl7SettingsFromMap(raw map[string]interface{}) hl7Settings {
	return hl7Settings{
		Framing:               firstNonEmpty(asString(raw["framing"]), "mllp"),
		SampleIDPath:          firstNonEmpty(asString(raw["sample_id_path"]), "OBR.3.1"),
		SampleIDFallbackPaths: stringList(raw["sample_id_fallback_paths"]),
		SampleNoPath:          firstNonEmpty(asString(raw["sample_no_path"]), "OBR.3.1"),
		RackNoPath:            asString(raw["rack_no_path"]),
		RackPositionPath:      asString(raw["rack_position_path"]),
		RunDatePath:           firstNonEmpty(asString(raw["run_date_path"]), "OBR.7.1"),
		PatientIDPath:         asString(raw["patient_id_path"]),
		PatientNamePath:       asString(raw["patient_name_path"]),
		QCIndicatorPath:       asString(raw["qc_indicator_path"]),
		QCIndicatorValues:     stringList(raw["qc_indicator_values"]),
		ResultIdentifierPath:  firstNonEmpty(asString(raw["result_identifier_path"]), "OBX.3.1"),
		ResultNamePath:        firstNonEmpty(asString(raw["result_name_path"]), "OBX.3.2"),
		ResultValuePath:       firstNonEmpty(asString(raw["result_value_path"]), "OBX.5.1"),
		ResultUnitPath:        firstNonEmpty(asString(raw["result_unit_path"]), "OBX.6.1"),
		ResultInterpretedPath: firstNonEmpty(asString(raw["result_interpreted_path"]), "OBX.8.1"),
		AnalyteMappings:       stringMap(raw["analyte_mappings"]),
		AllowedResultIDs:      stringList(raw["allowed_result_ids"]),
	}
}

func simpleSettingsFromMap(raw map[string]interface{}) simpleSettings {
	return simpleSettings{
		SampleModeQCValues: stringList(raw["sample_mode_qc_values"]),
		AnalyteMappings:    stringMap(raw["analyte_mappings"]),
		AnalyteUnits:       stringMap(raw["analyte_units"]),
		ImageMode:          normalizeImageMode(asString(raw["image_mode"])),
	}
}

func readHL7Message(reader *bufio.Reader, settings hl7Settings) ([]byte, error) {
	if strings.EqualFold(settings.Framing, "raw") {
		return reader.ReadBytes('\r')
	}
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0x0b {
			break
		}
	}
	var out bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0x1c {
			next, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if next == 0x0d {
				break
			}
			out.WriteByte(b)
			out.WriteByte(next)
			continue
		}
		out.WriteByte(b)
	}
	return out.Bytes(), nil
}

func readSimpleMessage(reader *bufio.Reader) ([]byte, error) {
	var out bytes.Buffer
	terminator := []byte("</TRANSMIT>")
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		out.WriteByte(b)
		if bytes.Contains(out.Bytes(), terminator) {
			return out.Bytes(), nil
		}
	}
}

func parseHL7Results(raw []byte, settings hl7Settings, ignored ...ignoreLogger) ([]parsedMessage, error) {
	var logIgnored ignoreLogger
	if len(ignored) > 0 {
		logIgnored = ignored[0]
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New("empty hl7 message")
	}
	raw = bytes.TrimPrefix(raw, []byte{0x0b})
	raw = bytes.TrimSuffix(raw, []byte{0x1c, 0x0d})
	msg, err := parseHL7Message(raw)
	if err != nil {
		return nil, err
	}
	current := parsedMessage{
		RunDate:      normalizeRunDate(resolvePathValue(msg, settings.RunDatePath, nil)),
		SampleID:     resolveWithFallback(msg, settings.SampleIDPath, settings.SampleIDFallbackPaths, nil),
		SampleNo:     resolvePathValue(msg, settings.SampleNoPath, nil),
		RackNo:       resolvePathValue(msg, settings.RackNoPath, nil),
		RackPosition: resolvePathValue(msg, settings.RackPositionPath, nil),
		PatientID:    resolvePathValue(msg, settings.PatientIDPath, nil),
		PatientName:  resolvePathValue(msg, settings.PatientNamePath, nil),
		FileID:       resolveWithFallback(msg, "MSH.10.1", nil, nil),
	}
	current.IsQC = containsFold(settings.QCIndicatorValues, resolvePathValue(msg, settings.QCIndicatorPath, nil))

	results := []parsedResult{}
	var currentOBR *hl7Segment
	for i := range msg.Segments {
		segment := msg.Segments[i]
		if segment.Name == "OBR" {
			currentOBR = &segment
			if value := resolvePathValue(msg, settings.RunDatePath, currentOBR); strings.TrimSpace(value) != "" {
				current.RunDate = normalizeRunDate(value)
			}
			if value := resolveWithFallback(msg, settings.SampleIDPath, settings.SampleIDFallbackPaths, currentOBR); strings.TrimSpace(value) != "" {
				current.SampleID = value
			}
			if value := resolvePathValue(msg, settings.SampleNoPath, currentOBR); strings.TrimSpace(value) != "" {
				current.SampleNo = value
			}
			if value := resolvePathValue(msg, settings.RackNoPath, currentOBR); strings.TrimSpace(value) != "" {
				current.RackNo = value
			}
			if value := resolvePathValue(msg, settings.RackPositionPath, currentOBR); strings.TrimSpace(value) != "" {
				current.RackPosition = value
			}
			if value := resolvePathValue(msg, settings.QCIndicatorPath, currentOBR); strings.TrimSpace(value) != "" {
				current.IsQC = containsFold(settings.QCIndicatorValues, value)
			}
			continue
		}
		if segment.Name != "OBX" {
			continue
		}
		identifier := resolvePathValue(msg, settings.ResultIdentifierPath, &segment)
		if len(settings.AllowedResultIDs) > 0 && !containsFold(settings.AllowedResultIDs, identifier) {
			if logIgnored != nil {
				logIgnored("hl7_result", "result identifier filtered by allowed_result_ids", map[string]interface{}{
					"identifier": identifier,
					"name":       resolvePathValue(msg, settings.ResultNamePath, &segment),
				})
			}
			continue
		}
		name := resolvePathValue(msg, settings.ResultNamePath, &segment)
		tag := mapAnalyte(identifier, name, settings.AnalyteMappings)
		value := resolvePathValue(msg, settings.ResultValuePath, &segment)
		if strings.TrimSpace(value) == "" {
			if logIgnored != nil {
				logIgnored("hl7_result", "empty result value", map[string]interface{}{
					"identifier": identifier,
					"name":       name,
					"tag":        tag,
				})
			}
			continue
		}
		results = append(results, parsedResult{
			AnalyteTag:  tag,
			AnalyteName: firstNonEmpty(name, identifier, tag),
			ResultValue: value,
			RawValue:    value,
			Interpreted: resolvePathValue(msg, settings.ResultInterpretedPath, &segment),
			Unit:        resolvePathValue(msg, settings.ResultUnitPath, &segment),
			Flags: map[string]interface{}{
				"hl7_identifier": identifier,
			},
			ProtocolOptions: map[string]interface{}{
				"hl7_identifier": identifier,
			},
		})
	}
	current.Results = results
	if current.RunDate == "" {
		current.RunDate = time.Now().Format("2006-01-02")
	}
	return []parsedMessage{current}, nil
}

func parseSimpleResults(raw []byte, settings simpleSettings, ignored ...ignoreLogger) ([]parsedMessage, error) {
	var logIgnored ignoreLogger
	if len(ignored) > 0 {
		logIgnored = ignored[0]
	}
	text := string(raw)
	if !strings.Contains(text, "<TRANSMIT>") {
		return nil, errors.New("invalid simple payload")
	}
	model, serial := parseSimpleInstrument(text)
	message := parsedMessage{
		InstrumentModel:            model,
		InstrumentSequentialNumber: serial,
	}
	info := extractFirst(text, `<I>(.*?)</I>`)
	parts := strings.Split(info, "|")
	message.Sample = fieldAt(parts, 0)
	rawTimestamp := fieldAt(parts, 1)
	message.RunDate = normalizeRunDate(rawTimestamp)
	message.RunTime = normalizeRunTime(rawTimestamp)
	parseSimpleInfoFields(&message, parts, settings)
	if message.RunDate == "" {
		message.RunDate = time.Now().Format("2006-01-02")
	}
	if message.SampleID == "" {
		message.SampleID = firstNonEmpty(message.FileID, message.SampleNo)
	}
	message.image = extractSimpleImage(raw, settings.ImageMode)
	resultBody := extractFirst(text, `<R>(.*?)</R>`)
	if strings.TrimSpace(resultBody) == "" {
		return nil, errors.New("simple payload does not contain results")
	}
	pairs := parseSimpleResultPairs(resultBody)
	if len(pairs) == 0 {
		return nil, errors.New("simple payload contains no analyte/value pairs")
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(pairs[key])
		if value == "" {
			if logIgnored != nil {
				logIgnored("simple_result", "empty parsed value", map[string]interface{}{
					"analyte": key,
				})
			}
			continue
		}
		tag := mapAnalyte(key, key, settings.AnalyteMappings)
		message.Results = append(message.Results, parsedResult{
			AnalyteTag:  tag,
			AnalyteName: key,
			ResultValue: value,
			RawValue:    value,
			Unit:        settings.AnalyteUnits[key],
			Flags: map[string]interface{}{
				"simple_analyte": key,
			},
			ProtocolOptions: map[string]interface{}{
				"simple_name": key,
			},
		})
	}
	return []parsedMessage{message}, nil
}

func extractSimpleImage(raw []byte, imageMode string) *parsedImage {
	imageMode = normalizeImageMode(imageMode)
	if imageMode == "no_image" {
		return nil
	}
	upper := bytes.ToUpper(raw)
	for _, tag := range []string{"IMG", "IMAGE", "PIC", "PHOTO", "GRAPH"} {
		start := bytes.Index(upper, []byte("<"+tag))
		if start < 0 {
			continue
		}
		gt := bytes.IndexByte(upper[start:], '>')
		if gt < 0 {
			continue
		}
		bodyStart := start + gt + 1
		end := bytes.Index(upper[bodyStart:], []byte("</"+tag+">"))
		if end < 0 {
			continue
		}
		body := bytes.TrimSpace(raw[bodyStart : bodyStart+end])
		name := extractFirst(string(body), `<NAME>(.*?)</NAME>`)
		dataBody := extractFirst(string(body), `<DATA>(.*?)</DATA>`)
		if strings.TrimSpace(dataBody) != "" {
			body = []byte(dataBody)
		}
		data, format, ok := decodeSimpleImageBody(body, imageMode)
		if !ok || len(data) == 0 {
			continue
		}
		if format == "bin" && strings.TrimSpace(name) != "" {
			switch strings.ToLower(strings.TrimPrefix(filepathExt(name), ".")) {
			case "png", "bmp", "jpg", "jpeg", "gif":
				format = strings.ToLower(strings.TrimPrefix(filepathExt(name), "."))
				if format == "jpeg" {
					format = "jpg"
				}
			}
		}
		return &parsedImage{
			tag:      tag,
			name:     strings.TrimSpace(name),
			format:   format,
			encoding: imageMode,
			data:     data,
		}
	}
	return nil
}

func decodeSimpleImageBody(body []byte, imageMode string) ([]byte, string, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, "", false
	}
	if imageMode == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(removeWhitespace(string(body)))
		if err == nil && len(decoded) > 0 {
			return decoded, detectImageFormat(decoded), true
		}
	}
	if decoded, ok := decodeHexImage(body); ok {
		return decoded, detectImageFormat(decoded), true
	}
	if imageMode == "bitmap" && len(body) > 0 {
		return append([]byte(nil), body...), detectImageFormat(body), true
	}
	if decoded, err := base64.StdEncoding.DecodeString(removeWhitespace(string(body))); err == nil && len(decoded) > 0 {
		return decoded, detectImageFormat(decoded), true
	}
	return nil, "", false
}

func decodeHexImage(body []byte) ([]byte, bool) {
	normalized := removeWhitespace(string(body))
	if normalized == "" || len(normalized)%2 != 0 {
		return nil, false
	}
	for _, ch := range normalized {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return nil, false
		}
	}
	decoded, err := hex.DecodeString(normalized)
	if err != nil || len(decoded) == 0 {
		return nil, false
	}
	return decoded, true
}

func detectImageFormat(data []byte) string {
	switch {
	case len(data) >= 2 && data[0] == 'B' && data[1] == 'M':
		return "bmp"
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpg"
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return "gif"
	default:
		return "bin"
	}
}

func removeWhitespace(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\t", "")
	value = strings.ReplaceAll(value, " ", "")
	return strings.TrimSpace(value)
}

func filepathExt(name string) string {
	idx := strings.LastIndex(strings.TrimSpace(name), ".")
	if idx < 0 {
		return ""
	}
	return name[idx:]
}

func parseHL7Message(raw []byte) (hl7Message, error) {
	text := strings.ReplaceAll(string(raw), "\n", "\r")
	text = strings.Trim(text, "\r")
	lines := []string{}
	for _, line := range strings.Split(text, "\r") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return hl7Message{}, errors.New("hl7 message contains no segments")
	}
	fieldSep := "|"
	componentSep := "^"
	repeatSep := "~"
	escapeSep := `\`
	subcomponentSep := "&"
	segments := make([]hl7Segment, 0, len(lines))
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		name := line[:3]
		if name == "MSH" && len(line) >= 8 {
			fieldSep = string(line[3])
			fields := strings.Split(line, fieldSep)
			if len(fields) > 1 {
				enc := fields[1]
				if len(enc) >= 4 {
					componentSep = string(enc[0])
					repeatSep = string(enc[1])
					escapeSep = string(enc[2])
					subcomponentSep = string(enc[3])
				}
			}
		}
		fields := strings.Split(line, fieldSep)
		segments = append(segments, hl7Segment{
			Name:            name,
			Fields:          fields,
			FieldSeparator:  fieldSep,
			ComponentSep:    componentSep,
			RepeatSep:       repeatSep,
			EscapeSep:       escapeSep,
			SubcomponentSep: subcomponentSep,
		})
	}
	return hl7Message{Segments: segments}, nil
}

func resolveWithFallback(msg hl7Message, primary string, fallbacks []string, context *hl7Segment) string {
	if value := resolvePathValue(msg, primary, context); strings.TrimSpace(value) != "" {
		return value
	}
	for _, candidate := range fallbacks {
		if value := resolvePathValue(msg, candidate, context); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func resolvePathValue(msg hl7Message, path string, context *hl7Segment) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}
	segName := strings.ToUpper(strings.TrimSpace(parts[0]))
	fieldIndex := atoi(parts[1])
	componentIndex := 0
	subcomponentIndex := 0
	if len(parts) > 2 {
		componentIndex = atoi(parts[2])
	}
	if len(parts) > 3 {
		subcomponentIndex = atoi(parts[3])
	}
	var segment *hl7Segment
	if context != nil && strings.EqualFold(context.Name, segName) {
		segment = context
	} else {
		for i := range msg.Segments {
			if strings.EqualFold(msg.Segments[i].Name, segName) {
				segment = &msg.Segments[i]
				break
			}
		}
	}
	if segment == nil {
		return ""
	}
	value := hl7FieldValue(*segment, fieldIndex)
	if componentIndex > 0 {
		chunks := strings.Split(value, segment.ComponentSep)
		if componentIndex-1 >= 0 && componentIndex-1 < len(chunks) {
			value = chunks[componentIndex-1]
		} else {
			value = ""
		}
	}
	if subcomponentIndex > 0 && value != "" {
		chunks := strings.Split(value, segment.SubcomponentSep)
		if subcomponentIndex-1 >= 0 && subcomponentIndex-1 < len(chunks) {
			value = chunks[subcomponentIndex-1]
		} else {
			value = ""
		}
	}
	return strings.TrimSpace(value)
}

func hl7FieldValue(segment hl7Segment, fieldIndex int) string {
	if fieldIndex <= 0 {
		return ""
	}
	actual := fieldIndex
	if segment.Name == "MSH" {
		if fieldIndex == 1 {
			return segment.FieldSeparator
		}
		actual = fieldIndex - 1
	}
	if actual < 0 || actual >= len(segment.Fields) {
		return ""
	}
	return strings.TrimSpace(segment.Fields[actual])
}

func parseSimpleResultPairs(body string) map[string]string {
	body = strings.ReplaceAll(body, "\r", "")
	body = strings.ReplaceAll(body, "\n", "")
	body = strings.TrimSpace(body)
	keys := []string{"HbA1a", "HbA1b", "HbF", "L-A1C", "HbA1c", "HbA0", "eAG"}
	keyPattern := strings.Join(keys, "|")
	re := regexp.MustCompile(`(` + keyPattern + `)\|([-+]?[0-9]+(?:\.[0-9]+)?)`)
	matches := re.FindAllStringSubmatch(body, -1)
	out := map[string]string{}
	for _, match := range matches {
		if len(match) >= 3 {
			out[match[1]] = match[2]
		}
	}
	return out
}

func parseSimpleInstrument(text string) (string, string) {
	re := regexp.MustCompile(`<M>\s*([^|<]+)\|([^<|]+)`)
	match := re.FindStringSubmatch(text)
	if len(match) < 3 {
		return "", ""
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
}

func parseSimpleInfoFields(message *parsedMessage, parts []string, settings simpleSettings) {
	if message == nil {
		return
	}
	switch len(parts) {
	case 7:
		message.SampleNo = fieldAt(parts, 2)
		message.SampleID = fieldAt(parts, 3)
		message.RackNo = fieldAt(parts, 4)
		message.RackPosition = fieldAt(parts, 5)
		message.SampleType = fieldAt(parts, 6)
	default:
		message.SampleNo = fieldAt(parts, 2)
		message.FileID = fieldAt(parts, 3)
		rackNo, rackPosition := splitCompactRackPosition(fieldAt(parts, 4))
		message.RackNo = rackNo
		message.RackPosition = rackPosition
		message.SampleType = fieldAt(parts, 5)
	}
	message.ControlLevel = message.SampleType
	message.IsQC = containsFold(settings.SampleModeQCValues, message.SampleType)
	if message.FileID == "" && len(parts) >= 7 {
		message.FileID = fieldAt(parts, 3)
	}
	if message.SampleID == "" {
		message.SampleID = firstNonEmpty(message.FileID, message.SampleNo)
	}
}

func splitCompactRackPosition(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if len(value) == 1 {
		return "", value
	}
	return strings.TrimSpace(value[:1]), strings.TrimSpace(value[1:])
}

func fieldAt(parts []string, index int) string {
	if index < 0 || index >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[index])
}

func normalizeRunDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"20060102150405",
		"200601021504",
		"20060102",
		"2006-01-02",
		"2006/01/02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	if len(raw) >= 10 {
		return raw[:10]
	}
	return raw
}

func normalizeRunTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"20060102150405",
		"200601021504",
		"2006/01/02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format("15:04:05")
		}
	}
	if len(raw) >= 14 {
		return raw[8:10] + ":" + raw[10:12] + ":" + raw[12:14]
	}
	return ""
}

func mapAnalyte(identifier, name string, mappings map[string]string) string {
	for _, key := range []string{strings.TrimSpace(identifier), strings.TrimSpace(name)} {
		if key == "" {
			continue
		}
		for mapKey, mapValue := range mappings {
			if strings.EqualFold(strings.TrimSpace(mapKey), key) && strings.TrimSpace(mapValue) != "" {
				return strings.TrimSpace(mapValue)
			}
		}
	}
	return firstNonEmpty(identifier, name)
}

func containsFold(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, item := range values {
		if strings.EqualFold(strings.TrimSpace(item), needle) {
			return true
		}
	}
	return false
}

func extractFirst(text, pattern string) string {
	re := regexp.MustCompile(`(?is)` + pattern)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func stringList(raw interface{}) []string {
	switch t := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if value := strings.TrimSpace(asString(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if value := strings.TrimSpace(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{strings.TrimSpace(t)}
	default:
		return nil
	}
}

func stringMap(raw interface{}) map[string]string {
	out := map[string]string{}
	switch t := raw.(type) {
	case map[string]interface{}:
		for key, value := range t {
			if mapped := strings.TrimSpace(asString(value)); mapped != "" {
				out[key] = mapped
			}
		}
	case map[string]string:
		for key, value := range t {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	return out
}

func prettyJSON(value interface{}) string {
	blob, _ := json.MarshalIndent(value, "", "  ")
	return string(blob)
}

func drainReader(r io.Reader) string {
	blob, _ := io.ReadAll(r)
	return string(blob)
}
