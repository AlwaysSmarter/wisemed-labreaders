package siui

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"regexp"
	"strconv"
	"strings"
	"time"
)

type wmMapping struct {
	From string `json:"from_field"`
	To   string `json:"to_field"`
	Type string `json:"type,omitempty"`
}
type wmCallbackFile struct {
	Unzip       bool   `json:"unzip,omitempty"`
	Entry       string `json:"entry,omitempty"`
	ValidateXSD *bool  `json:"validateXSD,omitempty"`
}
type wmCallback struct {
	File          *wmCallbackFile `json:"file,omitempty"`
	LT            string          `json:"lt,omitempty"`
	Path          string          `json:"apipath"`
	Method        string          `json:"method"`
	TransformPath []wmMapping     `json:"transform_path,omitempty"`
	Body          map[string]any  `json:"body"`
}

// Preserve numeric literals in fixed body fields, including large identifiers.
func (c *wmCallback) UnmarshalJSON(data []byte) error {
	type plain wmCallback
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(c))
}

type wmDelivery struct {
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Error      string `json:"error,omitempty"`
}
type wmSender interface {
	CallbackDestination() (string, error)
	SendCallback(context.Context, string, string, string, string, string, []byte) (int, error)
}

var wmTemplate = regexp.MustCompile(`^/[A-Za-z0-9_/{}/-]+$`)
var wmField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

func initializeCallbacks(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS siui_wm_callbacks(
 call_id TEXT PRIMARY KEY, spec TEXT NOT NULL, parameters TEXT NOT NULL,
 destination TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'waiting',
 attempts INTEGER NOT NULL DEFAULT 0,http_status INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '',next_attempt INTEGER NOT NULL DEFAULT 0,
 path TEXT NOT NULL DEFAULT '',body TEXT NOT NULL DEFAULT '');
 UPDATE siui_wm_callbacks SET status=CASE WHEN attempts>=5 THEN 'failed' ELSE 'pending' END,error='Delivery interrupted; confirmation unknown' WHERE status='sending';
 UPDATE siui_wm_callbacks SET status='skipped',error='CNAS result unavailable after restart'
 WHERE status='waiting' AND call_id IN (SELECT id FROM siui_calls WHERE status IN ('failed','unknown'));`)
	return err
}
func (c *wmCallback) mappings() ([]wmMapping, error) {
	raw, ok := c.Body["transform_result"]
	if !ok {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var mappings []wmMapping
	if err = json.Unmarshal(data, &mappings); err != nil {
		return nil, errors.New("body.transform_result must be an array of mappings")
	}
	return mappings, nil
}
func validateWMMappings(mappings []wmMapping) error {
	seen := map[string]bool{}
	for _, m := range mappings {
		if !wmField.MatchString(m.From) || !wmField.MatchString(m.To) || strings.Contains(m.To, ".") || m.To == "transform_result" || seen[m.To] {
			return errors.New("invalid or duplicate callback mapping")
		}
		switch m.Type {
		case "", "string", "integer", "number", "boolean":
		default:
			return errors.New("unsupported callback mapping type")
		}
		seen[m.To] = true
	}
	return nil
}
func (c *wmCallback) validate() error {
	if c.File != nil {
		if c.File.Entry != "" && !c.File.Unzip {
			return errors.New("callwmapi.file.entry requires unzip")
		}

	}
	if c.Method != "POST" && c.Method != "PUT" {
		return errors.New("callwmapi.method must be POST or PUT")
	}
	if !wmTemplate.MatchString(c.Path) || strings.HasPrefix(c.Path, "//") || strings.Contains(c.Path, "//") {
		return errors.New("callwmapi.apipath must be a relative WiseMED API path")
	}
	if c.Body == nil {
		return errors.New("callwmapi.body must be an object")
	}
	if err := validateWMMappings(c.TransformPath); err != nil {
		return err
	}
	mappings, err := c.mappings()
	if err != nil {
		return err
	}
	if err = validateWMMappings(mappings); err != nil {
		return err
	}
	rest := c.Path
	for _, m := range c.TransformPath {
		placeholder := "{" + m.To + "}"
		if !strings.Contains(rest, placeholder) {
			return errors.New("transform_path target missing from apipath")
		}
		rest = strings.ReplaceAll(rest, placeholder, "value")
	}
	if strings.ContainsAny(rest, "{}") {
		return errors.New("unmapped or invalid path placeholder")
	}
	return nil
}
func wmLookup(source map[string]any, field string) (any, bool) {
	var value any = source
	for _, part := range strings.Split(field, ".") {
		switch node := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = node[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			value = node[index]
		case []string:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			value = node[index]
		case []map[string]any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			value = node[index]
		default:
			return nil, false
		}
	}
	return value, value != nil
}

func (c *wmCallback) path(parameters map[string]string) (string, error) {
	path := c.Path
	source := map[string]any{}
	for key, value := range parameters {
		source[key] = value
	}
	if _, ok := source["cnp"]; !ok {
		if pid, yes := source["pid"]; yes {
			source["cnp"] = pid
		}
	}
	source["parameters"] = mapStringAny(parameters)
	for _, m := range c.TransformPath {
		value, ok := wmLookup(source, m.From)
		if !ok {
			return "", fmt.Errorf("missing path source field: %s", m.From)
		}
		text, ok := value.(string)
		if !ok || text == "" || text == "." || text == ".." || strings.ContainsAny(text, "/\\%?#\r\n") {
			return "", errors.New("invalid callback path value")
		}
		path = strings.ReplaceAll(path, "{"+m.To+"}", url.PathEscape(text))
	}
	return path, nil
}
func mapStringAny(in map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func wmConvert(value any, m wmMapping) (any, error) {
	typ := m.Type
	// Match the established insurance-state contract, without coercing CNP/CID.
	if typ == "" && (m.From == "state" || m.From == "insured.state" || m.From == "result.insured.state") {
		typ = "integer"
	}
	text, ok := value.(string)
	if typ == "" {
		return value, nil
	}
	if !ok {
		text = fmt.Sprint(value)
	}
	switch typ {
	case "string":
		return text, nil
	case "integer":
		return strconv.ParseInt(text, 10, 64)
	case "number":
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		var number any
		if err := decoder.Decode(&number); err != nil {
			return nil, errors.New("invalid number")
		}
		if value, ok := number.(json.Number); ok && json.Valid([]byte(text)) {
			return value, nil
		}
		return nil, errors.New("invalid number")
	case "boolean":
		return strconv.ParseBool(text)
	}
	return nil, errors.New("invalid conversion")
}
func (c *wmCallback) body(result map[string]any) ([]byte, error) {
	if result["parse_error"] != nil {
		return nil, errors.New("CNAS response could not be parsed")
	}
	source := map[string]any{"result": result}
	for key, value := range result {
		source[key] = value
	}
	if insured, ok := result["insured"].(map[string]any); ok {
		for key, value := range insured {
			source[key] = value
		}
	}
	body := map[string]any{}
	for key, value := range c.Body {
		if key != "transform_result" {
			body[key] = value
		}
	}
	mappings, err := c.mappings()
	if err != nil {
		return nil, err
	}
	for _, m := range mappings {
		value, ok := wmLookup(source, m.From)
		if !ok {
			return nil, fmt.Errorf("missing result source field: %s", m.From)
		}
		value, err = wmConvert(value, m)
		if err != nil {
			return nil, fmt.Errorf("cannot convert result field: %s", m.From)
		}
		body[m.To] = value
	}
	return json.Marshal(body)
}
func (m *Module) callbackSender() (wmSender, bool) {
	service, ok := m.rt.Service("wisemed-api")
	if !ok {
		return nil, false
	}
	sender, ok := service.(wmSender)
	return sender, ok
}
func (m *Module) deliverCallback(ctx context.Context) {
	var id, spec, destination, path, body string
	var attempts int
	err := m.db.QueryRow(`SELECT call_id,spec,destination,path,body,attempts FROM siui_wm_callbacks WHERE status='pending' AND next_attempt<=? ORDER BY next_attempt,call_id LIMIT 1`, time.Now().Unix()).Scan(&id, &spec, &destination, &path, &body, &attempts)
	if err != nil {
		return
	}
	res, err := m.db.Exec(`UPDATE siui_wm_callbacks SET status='sending',attempts=attempts+1 WHERE call_id=? AND status='pending'`, id)
	if err != nil {
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return
	}
	var callback wmCallback
	m.mu.Lock()
	database := m.cfg.Database
	m.mu.Unlock()
	err = decodeWMCallback(database, id, spec, &callback)
	code := 0
	if err == nil {
		sender, ok := m.callbackSender()
		if !ok {
			err = errors.New("WiseMED API unavailable")
		} else {
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			code, err = sender.SendCallback(requestCtx, destination, callback.Method, path, "siui-"+id, callback.LT, []byte(body))
			cancel()
		}
	}
	status, message := "delivered", ""
	if err != nil || code < 200 || code >= 300 {
		status = "failed"
		message = "WiseMED callback rejected"
		if err != nil {
			message = "WiseMED callback unavailable or transport failed"
		}
		if (err != nil || code == 429 || code >= 500) && attempts+1 < 5 {
			status = "pending"
		}
	}
	delay := time.Duration(1<<uint(attempts)) * 30 * time.Second
	_, e := m.db.Exec(`UPDATE siui_wm_callbacks SET status=?,http_status=?,error=?,next_attempt=? WHERE call_id=?`, status, code, message, time.Now().Add(delay).Unix(), id)
	if e != nil {
		m.rt.Logf("SIUI WiseMED delivery persistence failed call=%s", id)
	}
}

// Token-bearing specifications are encrypted at rest; legacy jobs remain readable.
func encodeWMCallback(database, id string, c *wmCallback) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if c.LT == "" {
		return string(raw), nil
	}
	sealed, err := sealPIN(database+".wm-callback", raw, "wm-callback:"+id)
	if err != nil {
		return "", err
	}
	return "sealed-v1:" + base64.StdEncoding.EncodeToString(sealed), nil
}
func decodeWMCallback(database, id, spec string, c *wmCallback) error {
	raw := []byte(spec)
	if strings.HasPrefix(spec, "sealed-v1:") {
		sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(spec, "sealed-v1:"))
		if err != nil {
			return err
		}
		raw, err = openPIN(database+".wm-callback", sealed, "wm-callback:"+id)
		if err != nil {
			return err
		}
	}
	*c = wmCallback{}
	return json.Unmarshal(raw, c)
}

// Prepare downloaded XML on the bridge before persisting the delivery body.
// Browser lifecycle and browser-side processResult do not affect this work.
func (c *wmCallback) prepareBody(ctx context.Context, database, id string, result map[string]any) ([]byte, error) {
	if c.File == nil {
		return c.body(result)
	}
	if result["file"] == nil {
		return nil, errors.New("callback requires a downloaded XML file")
	}
	downloadStorageMu.RLock()
	defer downloadStorageMu.RUnlock()
	file, err := os.Open(existingDownloadPath(database, id))
	if err != nil {
		return nil, errors.New("callback download unavailable")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, processedFileLimit+1))
	if err != nil {
		return nil, errors.New("cannot read callback download")
	}
	q := url.Values{"format": {"json"}}
	if c.File.Unzip {
		q.Set("unzip", "true")
	}
	if c.File.Entry != "" {
		q.Set("entry", c.File.Entry)
	}
	if c.File.ValidateXSD != nil && !*c.File.ValidateXSD {
		q.Set("validateXSD", "false")
	}
	processed, err := processCallFile(ctx, raw, q, func() string {
		if file, ok := result["file"].(map[string]any); ok {
			schema, _ := file["validation_schema"].(string)
			return schema
		}
		return ""
	}())
	if err != nil {
		return nil, err
	}
	var value any
	if err = json.Unmarshal(processed, &value); err != nil {
		return nil, err
	}
	enriched := map[string]any{}
	for k, v := range result {
		enriched[k] = v
	}
	enriched["file_json"] = value
	return c.body(enriched)
}
