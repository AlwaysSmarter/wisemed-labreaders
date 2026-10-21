package history

import "testing"

func TestDownloadIdentity(t *testing.T) {
	for _, tc := range []struct{ url, fallback, body, want, mime string }{
		{"/svapntws/download?fileName=7pf6aug8hu06t1k5ff9gnc", "call-id", "PK\x03\x04zip", "7pf6aug8hu06t1k5ff9gnc.zip", "application/zip"},
		{"/svapntws/download?fileName=7pf6aug8hu06t1k5ff9gnc", "call-id", "\xef\xbb\xbf  <?xml version=\"1.0\"?><root/>", "7pf6aug8hu06t1k5ff9gnc.xml", "application/xml"},
		{"/download?fileName=report.xml", "call-id", "PK\x05\x06", "report.zip", "application/zip"},
		{"/download?fileName=report.zip", "call-id", "<root/>", "report.xml", "application/xml"},
		{"/download?fileName=..%2Fsecret", "call-id", "<root/>", "call-id.xml", "application/xml"},
		{"/download?fileName=bad%0D%0Aheader", "call-id", "<root/>", "call-id.xml", "application/xml"},
		{"/download", "old.zip", "PK\x03\x04", "old.zip", "application/zip"},
		{"/download", "call-id", "opaque", "call-id.download", "application/octet-stream"},
	} {
		name, mime := DownloadIdentity(tc.url, tc.fallback, []byte(tc.body))
		if name != tc.want || mime != tc.mime || !ValidFilename(name) {
			t.Fatal(tc.url, name, mime)
		}
	}
}
