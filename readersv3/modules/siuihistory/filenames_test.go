package siuihistory

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"testing"

	"wisemed-labreaders/readersv3/shared/history"
)

func TestLegacyArchiveDownloadNameFromOriginalResponse(t *testing.T) {
	ctx := context.Background()
	store := localStore(t.TempDir())
	rec := history.Record{CallID: "old-call", CreatedAt: "2026-10-21T14:29:06Z", Artifacts: []history.Artifact{{Name: "download"}}}
	for _, tc := range []struct{ body, ext, mime string }{{"PK\x03\x04zip", ".zip", "application/zip"}, {"<root/>", ".xml", "application/xml"}} {
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		_ = json.NewEncoder(gz).Encode(map[string]any{"result": map[string]any{"values": []string{"/svapntws/download?fileName=7pf6aug8hu06t1k5ff9gnc", "7"}, "file": map[string]string{"filename": "old-call.zip"}}})
		gz.Close()
		if err := store.Put(ctx, objectKey("tenant", "bridge", rec, "record-gzip"), bytes.NewReader(compressed.Bytes()), int64(compressed.Len())); err != nil {
			t.Fatal(err)
		}
		if err := store.Put(ctx, objectKey("tenant", "bridge", rec, "download"), bytes.NewBufferString(tc.body), int64(len(tc.body))); err != nil {
			t.Fatal(err)
		}
		copy := rec
		copy.Artifacts = append([]history.Artifact(nil), rec.Artifacts...)
		populateLegacyNames(ctx, store, "tenant", "bridge", &copy)
		if copy.Artifacts[0].Filename != "7pf6aug8hu06t1k5ff9gnc"+tc.ext || copy.Artifacts[0].ContentType != tc.mime {
			t.Fatal(copy.Artifacts[0])
		}
		if rec.Artifacts[0].Name != "download" || rec.Artifacts[0].Filename != "" {
			t.Fatal("stored manifest changed")
		}
	}
}
