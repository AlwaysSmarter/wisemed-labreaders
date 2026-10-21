package history

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var safeFilename = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,179}$`)

func ValidFilename(name string) bool {
	return safeFilename.MatchString(name) && !strings.Contains(name, "..")
}

// DownloadIdentity uses the remote fileName token, but content determines its extension.
func DownloadIdentity(address, fallback string, data []byte) (string, string) {
	name := fallback
	if u, err := url.Parse(address); err == nil {
		if candidate := u.Query().Get("fileName"); ValidFilename(candidate) {
			name = candidate
		}
	}
	if !ValidFilename(name) {
		name = "download"
	}
	ext, mime := ".download", "application/octet-stream"
	if len(data) >= 4 && (bytes.HasPrefix(data, []byte{'P', 'K', 3, 4}) || bytes.HasPrefix(data, []byte{'P', 'K', 5, 6}) || bytes.HasPrefix(data, []byte{'P', 'K', 7, 8})) {
		ext, mime = ".zip", "application/zip"
	} else {
		head := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
		if bytes.HasPrefix(head, []byte("<")) || bytes.HasPrefix(head, []byte{0xff, 0xfe, '<', 0}) || bytes.HasPrefix(head, []byte{0xfe, 0xff, 0, '<'}) {
			ext, mime = ".xml", "application/xml"
		}
	}
	previous := path.Ext(name)
	if strings.EqualFold(previous, ".zip") || strings.EqualFold(previous, ".xml") || strings.EqualFold(previous, ".download") {
		name = strings.TrimSuffix(name, previous)
	}
	if len(name) > 170 {
		name = name[:170]
	}
	name = strings.TrimRight(name, ".")
	if name == "" {
		name = "download"
	}
	return name + ext, mime
}
func (a Artifact) DownloadName() string {
	if ValidFilename(a.Filename) {
		return a.Filename
	}
	if a.Name == "record-gzip" {
		return "record.json.gz"
	}
	return a.Name
}
