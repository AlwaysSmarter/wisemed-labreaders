package siui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (m *Module) passiveClient(w http.ResponseWriter) (*client, func(), bool) {
	if !m.nativeSupported {
		fail(w, 503, "Conectarea reală CNAS necesită utilitarul Windows cu token. Pe Mac poți folosi Test local fără CNAS; acesta nu verifică OCSP sau calitatea reală de asigurat.")
		return nil, nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending > 0 {
		fail(w, 409, "token busy; wait for current operations")
		return nil, nil, false
	}
	cfg := m.cfg
	if cfg.Username == "" || cfg.Licence == "" || cfg.Thumbprint == "" {
		fail(w, 409, "configure CNAS username, licence and certificate first")
		return nil, nil, false
	}
	m.checking = true
	m.pending++
	return &client{cfg: cfg, backend: m.factory(cfg)}, func() { m.mu.Lock(); m.checking = false; m.pending--; m.mu.Unlock() }, true
}
func (m *Module) ocspHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	c, release, ok := m.passiveClient(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if _, err := c.sessionHeaders(ctx); err != nil {
		fail(w, 502, err.Error())
		return
	}
	respond(w, 200, map[string]any{"ok": true, "authenticated": true, "cnas_contacted": true, "message": "Autentificare OCSP reușită. Nu au fost trimise servicii la validare."})
}
func providerEnvelope(uic, start, stop string) []byte {
	var b bytes.Buffer
	b.WriteString(`<soap:Envelope xmlns:soap="` + soapNS + `"><soap:Body><getProviderInfo xmlns="` + insuredNS + `"><partnerCategory>PARA</partnerCategory>`)
	for _, p := range [][2]string{{"start", start + "T00:00:00"}, {"stop", stop + "T23:59:59"}, {"uic", uic}} {
		b.WriteString("<" + p[0] + ">")
		xml.EscapeText(&b, []byte(p[1]))
		b.WriteString("</" + p[0] + ">")
	}
	b.WriteString(`</getProviderInfo></soap:Body></soap:Envelope>`)
	return b.Bytes()
}
func providerDownloadURL(data []byte, base string) (string, error) {
	n, err := parseXML(data)
	if err != nil {
		return "", err
	}
	if n.XMLName.Local != "Envelope" || n.XMLName.Space != soapNS {
		return "", errors.New("invalid SOAP envelope")
	}
	body := n.child("Body")
	if body.XMLName.Space != soapNS {
		return "", errors.New("invalid SOAP body")
	}
	if body.child("Fault").XMLName.Local != "" {
		return "", errors.New("CNAS SOAP fault")
	}
	response := body.child("getProviderInfoResponse")
	if response.XMLName.Space != insuredNS {
		return "", errors.New("unexpected personalization response")
	}
	values := []string{}
	for _, v := range response.Children {
		if v.XMLName.Local != "getProviderInfoReturn" || v.XMLName.Space != insuredNS || len(v.Children) > 0 {
			return "", errors.New("invalid personalization result")
		}
		values = append(values, v.Text)
	}
	if len(values) != 2 {
		return "", errors.New("missing personalization URL/size")
	}
	size, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil || size < 1 || size > maxData {
		return "", errors.New("personalization download missing or exceeds 1 MiB limit")
	}
	origin, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(values[0])
	if err != nil {
		return "", errors.New("invalid personalization URL")
	}
	target := origin.ResolveReference(ref)
	if target.Scheme != "https" || !strings.EqualFold(target.Host, origin.Host) || target.User != nil || target.Fragment != "" || strings.Contains(target.Path, "\\") {
		return "", errors.New("personalization URL must remain on the configured CNAS HTTPS origin")
	}
	return target.String(), nil
}
func checkPersonalizationFile(data []byte) (string, string, error) {
	if len(data) == 0 || len(data) > maxData {
		return "", "", errors.New("invalid personalization size")
	}
	if bytes.HasPrefix(data, []byte("PK")) {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", "", errors.New("invalid personalization ZIP")
		}
		total := 0
		files := 0
		for _, file := range archive.File {
			if file.FileInfo().IsDir() {
				continue
			}
			if file.UncompressedSize64 > maxData {
				return "", "", errors.New("personalization XML too large")
			}
			reader, err := file.Open()
			if err != nil {
				return "", "", errors.New("cannot read personalization ZIP")
			}
			b, err := io.ReadAll(io.LimitReader(reader, int64(maxData-total+1)))
			reader.Close()
			total += len(b)
			if err != nil || total > maxData {
				return "", "", errors.New("personalization ZIP content invalid or oversized")
			}
			root, err := parseXML(b)
			if err != nil || root.XMLName.Local != "provider" || root.XMLName.Space != piasNS {
				return "", "", errors.New("expected PIAS provider XML in personalization ZIP")
			}
			files++
		}
		if files != 1 {
			return "", "", errors.New("expected one personalization XML")
		}
		return "personalizare-PARA.zip", "application/zip", nil
	}
	root, err := parseXML(data)
	if err != nil || root.XMLName.Local != "provider" || root.XMLName.Space != piasNS {
		return "", "", errors.New("expected PIAS provider XML")
	}
	return "personalizare-PARA.xml", "application/xml", nil
}
func (c *client) personalization(ctx context.Context, start, stop string) (map[string]any, error) {
	uic, _, ok := strings.Cut(c.cfg.Username, "_")
	if !ok || uic == "" {
		return nil, errors.New("CNAS username must be CUI_CAS-CODE")
	}
	h, err := c.sessionHeaders(ctx)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	status, _, body, err := c.backend.Do(ctx, "POST", base+"/svapntws/services/SiuiWS", h, providerEnvelope(uic, start, stop))
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("CNAS personalization HTTP %d", status)
	}
	address, err := providerDownloadURL(body, base)
	if err != nil {
		return nil, err
	}
	h.Del("SOAPAction")
	h.Del("Content-Type")
	status, _, data, err := c.backend.Do(ctx, "GET", address, h, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("CNAS personalization download HTTP %d", status)
	}
	filename, mime, err := checkPersonalizationFile(data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"filename": filename, "content_type": mime, "data_base64": base64.StdEncoding.EncodeToString(data), "size": len(data)}, nil
}
func (m *Module) personalizationHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start string `json:"start"`
		Stop  string `json:"stop"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err.Error())
		return
	}
	start, e1 := time.Parse("2006-01-02", in.Start)
	stop, e2 := time.Parse("2006-01-02", in.Stop)
	if e1 != nil || e2 != nil || stop.Before(start) {
		fail(w, 400, "select a valid start/stop date range (YYYY-MM-DD)")
		return
	}
	c, release, ok := m.passiveClient(w)
	if !ok {
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	file, err := c.personalization(ctx, in.Start, in.Stop)
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	respond(w, 200, map[string]any{"ok": true, "cnas_contacted": true, "file": file})
}
