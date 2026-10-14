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
			required = append(required, p.Name)
		}
		parameterSchema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		if len(required) > 0 {
			parameterSchema["required"] = required
		}
		paths["/api/siui/"+op.Group+"/"+op.Name] = map[string]any{"post": map[string]any{
			"operationId": op.Name, "tags": []string{op.Group}, "summary": op.Label, "description": "CNAS " + op.Service + ". Returns a durable execution ID. No automatic retry. Poll /api/siui/calls/{id}. completed means a response was received; inspect values for CNAS business errors.",
			"x-cnas-mutating": op.Mutating, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"operation_id", "parameters"}, "properties": map[string]any{"operation_id": map[string]any{"type": "string", "pattern": operationIDPattern.String()}, "parameters": parameterSchema}}}}},
			"responses": map[string]any{"202": map[string]any{"description": "Queued; response.call.id identifies the execution"}, "200": map[string]any{"description": "Existing execution for this operation_id; not submitted again"}, "400": map[string]any{"description": "Invalid input"}, "401": map[string]any{"description": "Authentication required"}, "403": map[string]any{"description": "Administrator required for mutations"}, "409": map[string]any{"description": "Conflicting operation_id, missing configuration or token busy"}, "503": map[string]any{"description": "Windows/token or worker unavailable"}},
		}}
	}
	for path, method := range map[string]string{"general/ocsp": "post", "auth/token": "post", "catalog": "get", "openapi.json": "get", "calls/{id}": "get", "calls/{id}/file": "get", "settings": "get", "certificates": "get", "status": "get", "invoice/model": "get"} {
		action := map[string]any{"summary": path, "responses": map[string]any{"200": map[string]any{"description": "JSON result"}, "401": map[string]any{"description": "Authentication required"}}}
		if method == "post" {
			action["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false}}}}
		}
		if path == "calls/{id}" || path == "calls/{id}/file" {
			action["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
			if path == "calls/{id}/file" {
				action["parameters"] = append(action["parameters"].([]any), map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0}})
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
	addInvoiceOpenAPI(paths)
	settingsProperties := map[string]any{}
	for _, name := range []string{"fiscal_code", "cas_code", "licence", "certificate_store", "certificate_thumbprint", "base_url", "pe_base_url"} {
		settingsProperties[name] = map[string]any{"type": "string"}
	}
	settingsProperties["allow_invalid_server_certificate_date"] = map[string]any{"type": "boolean"}
	paths["/api/siui/settings"].(map[string]any)["put"] = map[string]any{"summary": "Save CNAS configuration and selected certificate profile (admin)", "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": settingsProperties, "required": []string{"fiscal_code", "cas_code", "base_url", "certificate_store", "certificate_thumbprint"}}}}}, "responses": map[string]any{"200": map[string]any{"description": "Saved; licence remains masked"}, "403": map[string]any{"description": "Administrator required"}, "409": map[string]any{"description": "Wait for active operations"}}}
	return map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": "WiseMED SIUI Bridge — GENERAL / CLINICE / FACTURARE", "version": "2.1.0"}, "tags": []any{map[string]string{"name": "general"}, map[string]string{"name": "clinice"}, map[string]string{"name": "facturare"}}, "security": []any{map[string]any{"bearerJWT": []string{}}}, "components": map[string]any{"securitySchemes": map[string]any{"browserSession": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName, "description": "Session cookie returned by /api/session/login."}, "bearerJWT": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}}, "paths": paths}
}
