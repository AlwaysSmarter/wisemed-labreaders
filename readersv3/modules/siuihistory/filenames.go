package siuihistory

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"io"

	"wisemed-labreaders/readersv3/shared/history"
)

// Enrich legacy manifests for presentation without changing their stored digest,
// object keys or originals. New Bridge manifests already include this metadata.
func (m *Module) populateFileNames(ctx context.Context, db *sql.DB, tenant, bridge string, rec *history.Record) {
	for i := range rec.Artifacts {
		a := &rec.Artifacts[i]
		if a.Name == "record-gzip" {
			a.Filename = a.DownloadName()
			a.ContentType = "application/gzip"
		}
	}
	need := false
	for _, a := range rec.Artifacts {
		if a.Name == "download" && a.Filename == "" {
			need = true
		}
	}
	if !need {
		return
	}
	store, err := m.archiveStore(ctx, db, tenant, bridge, rec.CallID)
	if err != nil {
		return
	}
	populateLegacyNames(ctx, store, tenant, bridge, rec)
}
func populateLegacyNames(ctx context.Context, store objectStore, tenant, bridge string, rec *history.Record) {
	var original struct {
		Result struct {
			Values []string `json:"values"`
			File   struct {
				Filename string `json:"filename"`
			} `json:"file"`
		} `json:"result"`
	}
	if file, err := store.Open(ctx, objectKey(tenant, bridge, *rec, "record-gzip"), 0); err == nil {
		if gz, err := gzip.NewReader(file); err == nil {
			if raw, err := io.ReadAll(io.LimitReader(gz, 2<<20)); err == nil {
				_ = json.Unmarshal(raw, &original)
			}
			gz.Close()
		}
		file.Close()
	}
	address := ""
	if len(original.Result.Values) > 0 {
		address = original.Result.Values[0]
	}
	fallback := original.Result.File.Filename
	if fallback == "" {
		fallback = rec.CallID
	}
	for i := range rec.Artifacts {
		a := &rec.Artifacts[i]
		if a.Name != "download" || a.Filename != "" {
			continue
		}
		file, err := store.Open(ctx, objectKey(tenant, bridge, *rec, a.Name), 0)
		if err != nil {
			continue
		}
		head, err := io.ReadAll(io.LimitReader(file, 512))
		file.Close()
		if err != nil {
			continue
		}
		a.Filename, a.ContentType = history.DownloadIdentity(address, fallback, head)
	}
}
