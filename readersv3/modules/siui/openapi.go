package siui

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// This document and Debug share the same bundled WSDL-derived operation catalog.
func openAPIDocument(readerIDs ...string) map[string]any {
	readerID := "siui-bridge"
	if len(readerIDs) > 0 {
		readerID = strings.TrimSpace(readerIDs[0])
	}
	cookieHash := sha256.Sum256([]byte(readerID))
	cookieName := fmt.Sprintf("wmr_local_session_%x", cookieHash[:6])
	paths := map[string]any{}
	for _, op := range operations() {
		properties := map[string]any{}
		required := []string{}
		for _, p := range op.Parameters {
			properties[p.Name] = map[string]any{"type": "string", "description": "WSDL xsd:" + p.Type}
			if p.Type == "boolean" {
				properties[p.Name].(map[string]any)["enum"] = []string{"true", "false", "1", "0"}
			}
			if p.Nullable {
				properties[p.Name].(map[string]any)["nullable"] = true
			}
			required = append(required, p.Name)
		}
		parameterSchema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		if len(required) > 0 {
			parameterSchema["required"] = required
		}
		paths["/api/siui/"+op.Group+"/"+op.Name] = map[string]any{"post": map[string]any{
			"operationId": op.Name, "tags": []string{op.Group}, "summary": op.Label, "description": "CNAS " + op.Service + ". Returns a durable execution ID. No automatic retry. Poll /api/siui/calls/{id}. completed means a response was received; inspect values for CNAS business errors.",
			"x-cnas-mutating": op.Mutating, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"operation_id", "parameters"}, "properties": map[string]any{"operation_id": map[string]any{"type": "string", "pattern": operationIDPattern.String()}, "parameters": parameterSchema, "callwmapi": wmCallbackSchema()}}}}},
			"responses": map[string]any{"202": map[string]any{"description": "Queued; response.call.id identifies the execution"}, "200": map[string]any{"description": "Existing execution for this operation_id; not submitted again"}, "400": map[string]any{"description": "Invalid input"}, "401": map[string]any{"description": "Authentication required"}, "403": map[string]any{"description": "Administrator required for mutations"}, "409": map[string]any{"description": "Conflicting operation_id, missing configuration or token busy"}, "503": map[string]any{"description": "Windows/token or worker unavailable"}},
		}}
	}
	paths["/api/siui/calls/{id}/callwmapi/retry"] = map[string]any{"post": map[string]any{
		"summary":    "Retry a failed WiseMED delivery without re-running CNAS (call owner)",
		"parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
		"responses":  map[string]any{"202": map[string]any{"description": "Delivery queued"}, "409": map[string]any{"description": "Delivery cannot be retried"}},
	}}
	for path, method := range map[string]string{"general/ocsp": "post", "auth/token": "post", "catalog": "get", "openapi.json": "get", "calls/{id}": "get", "calls/{id}/file": "get", "settings": "get", "certificates": "get", "status": "get", "invoice/model": "get"} {
		action := map[string]any{"summary": path, "responses": map[string]any{"200": map[string]any{"description": "JSON result"}, "401": map[string]any{"description": "Authentication required"}}}
		if method == "post" {
			action["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false}}}}
		}
		if path == "calls/{id}" || path == "calls/{id}/file" {
			action["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
			if path == "calls/{id}/file" {
				action["parameters"] = append(action["parameters"].([]any), map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0}})
				for _, name := range []string{"unzip", "entry", "format", "validateXSD"} {
					parameterSchema := map[string]any{"type": "string"}
					if name == "validateXSD" {
						parameterSchema = map[string]any{"type": "boolean", "default": true}
					}
					action["parameters"] = append(action["parameters"].([]any), map[string]any{"name": name, "in": "query", "schema": parameterSchema, "description": "Optional file processing: unzip=true|false, exact ZIP entry, format=bytes|xml|json, validateXSD=false explicitly skips default server-selected XSD validation. Keep identical options across offsets."})
				}
				action["description"] = "Original or processed file in base64 chunks. Processing order: unzip, default server-selected XSD (unless validateXSD=false), XML/JSON conversion. Offset applies to processed bytes; HTTP 422 on invalid processing. Limits: 64 MiB, XML depth 40; XSD uses existing 1 MiB limit."
			}
		}
		if path == "auth/token" {
			action["description"] = "Requires an authenticated browser session cookie from /api/session/login (reader-specific cookie name), or a verified WSM principal. Existing Bearer JWTs cannot mint tokens. HTTPS required. Returns access_token, token_type=Bearer and expires_in=900."
			action["security"] = []any{map[string]any{"browserSession": []string{}}}
		}
		paths["/api/siui/"+path] = map[string]any{method: action}
	}
	paths["/api/siui/token-pin"] = map[string]any{
		"get": map[string]any{"summary": "PIN status only (admin); never returns plaintext or ciphertext", "responses": map[string]any{"200": map[string]any{"description": "configured, hint (<hidden>), blocked, manual_required, pkcs11_module"}, "403": map[string]any{"description": "Administrator required"}}},
		"put": map[string]any{"summary": "Replace encrypted PIN, remove it, or supply a temporary manual PIN (admin)", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"pin": map[string]any{"type": "string", "format": "password", "writeOnly": true, "maxLength": 256}, "forget": map[string]any{"type": "boolean"}, "manual": map[string]any{"type": "boolean"}}}}}}, "responses": map[string]any{"200": map[string]any{"description": "Saved. Temporary manual PIN expires after 150 seconds or first failure."}, "403": map[string]any{"description": "Administrator required"}, "409": map[string]any{"description": "Token busy"}}},
	}

	for _, path := range []string{"/api/siui/operations", "/api/siui/operations/{id}", "/api/siui/operations/{id}/file", "/api/siui/operations/{id}/request", "/api/siui/operations/{id}/history/request", "/api/siui/operations/{id}/history", "/api/siui/operations/{id}/history/{artifact}"} {
		parameters := []any{}
		if strings.Contains(path, "{id}") {
			parameters = append(parameters, map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}})
		}
		if strings.Contains(path, "{artifact}") {
			parameters = append(parameters, map[string]any{"name": "artifact", "in": "path", "required": true, "schema": map[string]string{"type": "string"}})
		}
		if path == "/api/siui/operations" {
			for _, name := range []string{"from", "to", "operation_id", "operation", "status", "offset"} {
				parameters = append(parameters, map[string]any{"name": name, "in": "query", "schema": map[string]string{"type": "string"}})
			}
		}
		if strings.HasSuffix(path, "/file") || strings.HasSuffix(path, "/{artifact}") {
			parameters = append(parameters, map[string]any{"name": "offset", "in": "query", "schema": map[string]string{"type": "integer"}, "description": "Chunk offset; returns data_base64 and next_offset when supplied"})
		}
		paths[path] = map[string]any{"get": map[string]any{"tags": []string{"operations"}, "summary": "Local SIUI operation audit / History access", "description": "Administrator sees all calls; other authenticated callers see their own. List filters from/to use RFC3339, to exclusive; default whole current server day. Page size 100. History credentials remain on bridge.", "parameters": parameters, "responses": map[string]any{"200": map[string]string{"description": "Operation data or file"}, "400": map[string]string{"description": "Invalid filter"}, "404": map[string]string{"description": "Not found or not owned"}, "502": map[string]string{"description": "History unavailable"}}}}
	}
	paths["/api/siui/dashboard"] = map[string]any{"get": map[string]any{"tags": []string{"operations"}, "summary": "SIUI load, scoped call metrics, queues and endpoint observations", "description": "from/to RFC3339, exclusive end; default current server day, maximum 31 calendar days. Administrator sees all calls; other users see their own. Runtime load is global. No token is returned.", "parameters": []any{map[string]any{"name": "from", "in": "query", "schema": map[string]string{"type": "string", "format": "date-time"}}, map[string]any{"name": "to", "in": "query", "schema": map[string]string{"type": "string", "format": "date-time"}}}, "responses": map[string]any{"200": map[string]string{"description": "Dashboard snapshot"}, "400": map[string]string{"description": "Invalid interval"}}}}
	paths["/api/siui/dashboard/checks"] = map[string]any{"post": map[string]any{"tags": []string{"operations"}, "summary": "Probe configured HTTPS endpoints (admin)", "description": "HEAD without token/client certificate, strict TLS, six-second timeout per endpoint, maximum four parallel probes and 60-second cache. HTTP reachability does not prove SOAP functionality.", "responses": map[string]any{"200": map[string]string{"description": "Endpoint observations"}, "403": map[string]string{"description": "Administrator required"}, "409": map[string]string{"description": "Check already running"}}}}
	paths["/api/siui/downloads/storage"] = map[string]any{
		"get":  map[string]any{"summary": "Local download usage and eligible archived copies (administrator)", "description": "Bytes/counts, percent of local downloads eligible for cleanup. Migrates legacy files to UTC year/month/date folders.", "responses": map[string]any{"200": map[string]string{"description": "Storage totals"}, "409": map[string]string{"description": "Files busy; retry later"}}},
		"post": map[string]any{"summary": "Delete confirmed archived local downloads (administrator)", "description": "Requires confirm=true. Checks final state, archive manifest, SHA-256 and callbacks; preserves calls and sent request data.", "requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"confirm": map[string]any{"type": "boolean"}}}}}}, "responses": map[string]any{"200": map[string]string{"description": "Removed bytes/files and errors"}}},
	}
	addInvoiceOpenAPI(paths)
	settingsProperties := map[string]any{}
	for _, name := range []string{"fiscal_code", "cas_code", "licence", "certificate_store", "certificate_thumbprint", "base_url", "pe_base_url"} {
		settingsProperties[name] = map[string]any{"type": "string"}
	}
	settingsProperties["allow_invalid_server_certificate_date"] = map[string]any{"type": "boolean"}
	paths["/api/siui/settings"].(map[string]any)["put"] = map[string]any{"summary": "Save CNAS configuration and selected certificate profile (admin)", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": settingsProperties, "required": []string{"fiscal_code", "cas_code", "base_url", "certificate_store", "certificate_thumbprint"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "Saved; licence remains masked"}, "403": map[string]any{"description": "Administrator required"}, "409": map[string]any{"description": "Wait for active operations"}}}
	return map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": "WiseMED SIUI Bridge — GENERAL / CLINICE / FACTURARE", "version": "2.1.0"}, "tags": []any{map[string]string{"name": "general"}, map[string]string{"name": "clinice"}, map[string]string{"name": "facturare"}}, "security": []any{map[string]any{"bearerJWT": []string{}}}, "components": map[string]any{"securitySchemes": map[string]any{"browserSession": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName, "description": "Session cookie returned by /api/session/login."}, "bearerJWT": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}}, "paths": paths}
}

func wmCallbackSchema() map[string]any {
	mapping := map[string]any{"type": "object", "required": []string{"from_field", "to_field"}, "properties": map[string]any{
		"from_field": map[string]any{"type": "string", "description": "Dot path; transform_path reads parameters (cnp aliases pid), transform_result reads result (cid/state alias insured fields). Numeric indexes supported."},
		"to_field":   map[string]any{"type": "string"},
		"type":       map[string]any{"type": "string", "enum": []string{"string", "integer", "number", "boolean"}, "description": "Optional result conversion; insurance state defaults to integer, CID/CNP remain strings."},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"apipath", "method", "body"}, "description": "Optional durable WiseMED delivery for authenticated callers. WiseMED enforces write permissions. Only after successful CNAS response and transforms. Repeated deliveries use a stable Idempotency-Key; WiseMED must deduplicate.",
		"properties": map[string]any{
			"lt":             map[string]any{"type": "string", "writeOnly": true, "description": "Optional per-call WiseMED login token, forwarded as JWT lt. No preliminary login; omitted when absent."},
			"file":           map[string]any{"type": "object", "additionalProperties": false, "description": "Optional server-side download XML processing before callback. Exposes file_json to transform_result; independent of browser.", "properties": map[string]any{"unzip": map[string]any{"type": "boolean"}, "entry": map[string]any{"type": "string"}, "validateXSD": map[string]any{"type": "boolean", "default": true}}},
			"apipath":        map[string]any{"type": "string", "example": "/pacient/{cnp_pacient}"},
			"method":         map[string]any{"type": "string", "enum": []string{"POST", "PUT"}},
			"transform_path": map[string]any{"type": "array", "items": mapping},
			"body":           map[string]any{"type": "object", "additionalProperties": true, "properties": map[string]any{"transform_result": map[string]any{"type": "array", "items": mapping}}},
		}}
}
