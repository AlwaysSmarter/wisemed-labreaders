package siuihistory

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"wisemed-labreaders/readersv3/shared/history"
)

// Index only small day-to-day responses. Large archives remain original files.
func (m *Module) indexDetails(ctx context.Context, db *sql.DB, tenant, bridge string, rec history.Record) error {
	details := map[string]any{"operation": rec.Operation, "status": rec.Status, "parser_version": 1}
	pid, cid, parseError := "", "", ""
	if rec.Operation == "getInsured" || rec.Operation == "getInsuredByCID" {
		parsed, e := parseInsuredArtifact(m.stage(tenant, bridge, rec.CallID, "record-gzip"))
		if e != nil {
			parseError = e.Error()
		} else {
			details["insured"] = parsed
			pid, _ = parsed["pid"].(string)
			cid, _ = parsed["cid"].(string)
		}
	}
	data, _ := json.Marshal(details)
	_, e := db.ExecContext(ctx, `INSERT INTO siui_history_details(tenant,bridge,call_id,pid,cid,details,parse_error) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE pid=VALUES(pid),cid=VALUES(cid),details=VALUES(details),parse_error=VALUES(parse_error)`, tenant, bridge, rec.CallID, pid, cid, string(data), parseError)
	return e
}
func parseInsuredArtifact(path string) (map[string]any, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return nil, e
	}
	defer gz.Close()
	b, e := io.ReadAll(io.LimitReader(gz, (2<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("insured record exceeds 2 MiB extraction limit; original retained")
	}
	var record struct {
		Result struct {
			Values []string `json:"values"`
		} `json:"result"`
	}
	if e = json.Unmarshal(b, &record); e != nil {
		return nil, e
	}
	if len(record.Result.Values) != 1 {
		return nil, fmt.Errorf("expected one insured response")
	}
	type attrs struct {
		Attributes []xml.Attr `xml:",any,attr"`
	}
	var response struct {
		XMLName xml.Name `xml:"insuredResponse"`
		Insured []struct {
			Attributes []xml.Attr `xml:",any,attr"`
			Categories []attrs    `xml:"categories>category"`
			States     []attrs    `xml:"states>state"`
		} `xml:"insured"`
	}
	if e = xml.Unmarshal([]byte(record.Result.Values[0]), &response); e != nil {
		return nil, e
	}
	if len(response.Insured) != 1 {
		return nil, fmt.Errorf("expected exactly one insured person")
	}
	p := response.Insured[0]
	convert := func(attrs []xml.Attr) map[string]any {
		m := map[string]any{}
		for _, a := range attrs {
			if a.Name.Space == "" {
				m[a.Name.Local] = a.Value
			}
		}
		return m
	}
	result := convert(p.Attributes)
	if s, ok := result["state"].(string); ok {
		if n, e := strconv.Atoi(s); e == nil {
			result["state"] = n
		}
	}
	categories, states := []map[string]any{}, []map[string]any{}
	for _, a := range p.Categories {
		categories = append(categories, convert(a.Attributes))
	}
	for _, a := range p.States {
		states = append(states, convert(a.Attributes))
	}
	result["categories"] = categories
	result["states"] = states
	for _, key := range []string{"pid", "cid"} {
		if v, ok := result[key].(string); ok && len(strings.TrimSpace(v)) > 64 {
			return nil, fmt.Errorf("invalid %s length", key)
		}
	}
	return result, nil
}
