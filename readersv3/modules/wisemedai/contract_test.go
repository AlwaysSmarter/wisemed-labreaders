package wisemedai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAPIAndCatalogContract(t *testing.T) {
	root := filepath.Join("..", "..", "apps", "wisemed-ai", "docs")
	raw, e := os.ReadFile(filepath.Join(root, "openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		OpenAPI    string                    `json:"openapi"`
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.OpenAPI != "3.1.0" {
		t.Fatal("invalid OpenAPI")
	}
	for _, path := range []string{"/v1/sessions", "/v1/sessions/{id}", "/v1/sessions/{id}/transcript-chunks", "/v1/sessions/{id}/events", "/v1/sessions/{id}/finalize", "/v1/sessions/{id}/result", "/v1/sessions/{id}/approve", "/v1/runtime-settings", "/v1/templates/proposals"} {
		if len(doc.Paths[path]) == 0 {
			t.Fatal("missing contract route", path)
		}
	}
	raw, e = os.ReadFile(filepath.Join(root, "fixtures", "specialty-templates.json"))
	if e != nil {
		t.Fatal(e)
	}
	var catalog []Template
	if json.Unmarshal(raw, &catalog) != nil || len(catalog) != 52 {
		t.Fatal("invalid catalog fixture")
	}
	for _, tpl := range catalog {
		if e := tpl.validate(); e != nil {
			t.Fatal(tpl.Name, e)
		}
	}
}
