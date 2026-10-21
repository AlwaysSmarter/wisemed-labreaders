package history

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRequestFilesPreservePayload(t *testing.T) {
	xml := "<?xml version=\"1.0\"?>\r\n<report a=\"1\" />"
	zipped := []byte{'P', 'K', 0, 1, 2}
	p := map[string]string{"reportXML": xml, "invoiceFile": base64.StdEncoding.EncodeToString(zipped), "pid": "123", "bad/xml": "x"}
	f := RequestFiles(p)
	if len(f) != 2 || string(f["request-reportXML"]) != xml || string(f["request-invoiceFile"]) != string(zipped) {
		t.Fatal(f)
	}
	p["reportXML"] = strings.Repeat("x", 10000)
	if len(RequestPreview(p)["reportXML"]) >= 10000 || len(p["reportXML"]) != 10000 {
		t.Fatal("preview mutated request")
	}
}
