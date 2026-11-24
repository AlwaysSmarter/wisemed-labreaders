package ecard

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPublicErrorEnvelope(t *testing.T) {
	for _, input := range []struct{ raw, code, message string }{
		{"selected_reader_not_connected_or_supported", "ECARD_SELECTED_READER_NOT_CONNECTED_OR_SUPPORTED", "Terminalul de citire carduri selectat nu este detectat"},
		{"administrator required", "ECARD_ADMINISTRATOR_REQUIRED", "Operația necesită drepturi de administrator în aplicație."},
		{"cannot apply CEAS firewall: Windows error 5", "ECARD_CEAS_FIREWALL_FAILED", "Regulile firewall pentru modul CEAS offline nu au putut fi aplicate. Verificați detaliile Windows."},
	} {
		w := httptest.NewRecorder()
		respond(w, 400, map[string]interface{}{"ok": false, "error": input.raw})
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["error_code"] != input.code || body["message"] != input.message || body["error"] != input.raw {
			t.Fatalf("%s", w.Body.String())
		}
	}
}

func TestSDKErrorCodesAndHistoricalMessages(t *testing.T) {
	seen := map[string]bool{}
	for sdk := range sdkErrorDescriptions {
		if sdk == 0 {
			continue
		}
		value := map[string]interface{}{"ok": false, "code": "sdk_error", "sdk_code": float64(sdk), "message": "Operația nu a reușit: sdk_error."}
		explainErrors(value)
		code := value["error_code"].(string)
		if seen[code] || value["message"] != sdkErrorDescriptions[sdk] {
			t.Fatal(value)
		}
		seen[code] = true
	}
	// Old reports are normalized too, while the original SDK text remains available.
	value := map[string]interface{}{"response": map[string]interface{}{"ok": false, "code": "selected_reader_not_connected_or_supported", "message": "Operația nu a reușit: selected_reader_not_connected_or_supported."}}
	explainErrors(value)
	response := value["response"].(map[string]interface{})
	if response["message"] != "Terminalul de citire carduri selectat nu este detectat" || response["detail"] == nil {
		t.Fatal(response)
	}
	explainErrors(value)
	if response["error_code"] != "ECARD_SELECTED_READER_NOT_CONNECTED_OR_SUPPORTED" {
		t.Fatal(response)
	}
}

// Keep local validation failures translated as endpoints evolve.
func TestLiteralValidationErrorsHaveDescriptions(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "New" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "errors" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			raw, _ := strconv.Unquote(literal.Value)
			code, description := canonicalError(raw)
			if description == "" || (code == "internal_error" && raw != "internal_error") {
				t.Errorf("%s: missing translation for %q", name, raw)
			}
			return true
		})
	}
}
