package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"regexp"
)

// Allow CMS and ZIP overhead above the 10 MiB source XML limit.
const maxReportArchive = maxReportXML + 256*1024
const maxReportParameter = (maxReportArchive + 2) / 3 * 4

var reportFilenamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*_[0-9]+_[0-9]{8}_[0-9]{4}\.xml$`)

func validateReportArchive(value string) error {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) > maxReportArchive {
		return errors.New("reportXML must be a Base64 ZIP containing the CMS-signed report; use /api/siui/report/package for raw XML")
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(z.File) != 1 {
		return errors.New("reportXML ZIP must contain exactly one signed report file")
	}
	f := z.File[0]
	if !reportFilenamePattern.MatchString(f.Name) || f.UncompressedSize64 == 0 || f.UncompressedSize64 > maxReportArchive {
		return errors.New("report ZIP requires a nonempty signed file named PREFIX_CODE_YYYYMMDD_HHMM.xml within the size limit")
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(r, maxReportArchive+1))
	if err != nil || n == 0 || n > maxReportArchive {
		return errors.New("invalid or oversized report ZIP entry")
	}
	return nil
}

func (m *Module) reportPackageHTTP(w http.ResponseWriter, r *http.Request, admin bool) {
	if r.Method != "POST" {
		fail(w, 405, "POST required")
		return
	}
	if !admin {
		fail(w, 403, "administrator required for token signing")
		return
	}
	var in struct {
		XML      string `json:"report_xml"`
		Filename string `json:"filename"`
	}
	if err := decodeLimit(w, r, &in, 6*maxReportXML+maxData); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !reportFilenamePattern.MatchString(in.Filename) {
		fail(w, 400, "filename must be PREFIX_CODE_YYYYMMDD_HHMM.xml (CNAS reporting convention)")
		return
	}
	if _, err := parseXMLLimit([]byte(in.XML), maxReportXML); err != nil {
		fail(w, 400, err.Error())
		return
	}
	c, release, ok := m.passiveClient(w, "report/package")
	if !ok {
		return
	}
	defer release()
	signer, ok := c.backend.(interface {
		SignInvoice(context.Context, []byte) ([]byte, error)
	})
	if !ok {
		fail(w, 503, "CMS signing unavailable on this platform")
		return
	}
	// Legacy SIUISendReport.cs: FileSign (SignedCms(content, false)), then
	// ZIP the signed bytes under the original .xml filename, then Base64.
	// The shared token signer implements the same attached CMS format.
	signed, err := signer.SignInvoice(r.Context(), []byte(in.XML))
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	if len(signed) == 0 || len(signed) > maxReportArchive {
		fail(w, 502, "empty or oversized CMS signature result")
		return
	}
	packed, err := invoiceZIP(in.Filename, signed)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	encoded := base64.StdEncoding.EncodeToString(packed)
	if len(encoded) > maxReportParameter {
		fail(w, 413, "signed report exceeds size limit")
		return
	}
	respond(w, 200, map[string]any{"ok": true, "reportXML": encoded, "filename": in.Filename, "signed": true, "cnas_contacted": false})
}
