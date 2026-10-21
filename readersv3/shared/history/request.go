package history

import (
	"encoding/base64"
	"sort"
	"strings"
)

// RequestFiles preserves the application payload bytes, without reserializing XML.
func RequestFiles(parameters map[string]string) map[string][]byte {
	out := map[string][]byte{}
	for key, value := range parameters {
		if !ValidID(key) {
			continue
		}
		if strings.HasSuffix(strings.ToLower(key), "xml") {
			out["request-"+key] = []byte(value)
		}
		if key == "invoiceFile" {
			if b, err := base64.StdEncoding.DecodeString(value); err == nil {
				out["request-"+key] = b
			}
		}
	}
	return out
}
func RequestFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func RequestFilename(name string) string {
	if name == "request-invoiceFile" {
		return "invoiceFile.zip"
	}
	return strings.TrimPrefix(name, "request-") + ".xml"
}

// Preview prevents large XML/base64 fields from overwhelming HTTP/WSM/UI responses.
func RequestPreview(parameters map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range parameters {
		if len(v) > 4096 {
			v = v[:4096] + "\n[… previzualizare; conținut complet în fișier/arhiva cererii]"
		}
		out[k] = v
	}
	return out
}
