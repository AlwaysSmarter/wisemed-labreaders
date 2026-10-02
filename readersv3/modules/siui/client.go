package siui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type backend interface {
	Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error)
	Validate(context.Context, string, []byte) error
}
type settings struct {
	Username, Licence, Thumbprint, Store, BaseURL, Database string
	AllowInvalidServerCertificateDate                       bool
}
type client struct {
	cfg     settings
	backend backend
}

func validateBase(s string) error {
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("CNAS base URL must be an HTTPS origin")
	}
	return nil
}
func (c *client) credentials() (http.Header, error) {
	if c.cfg.Username == "" || strings.ContainsAny(c.cfg.Username, ":\r\n") {
		return nil, errors.New("configure the CNAS username")
	}
	secret := strings.TrimSpace(c.cfg.Licence)
	if secret == "" || len(secret) > 4096 || strings.ContainsAny(secret, "\r\n") {
		return nil, errors.New("configure a valid CNAS licence in settings")
	}
	h := http.Header{}
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.cfg.Username+":"+secret)))
	h.Set("Cache-Control", "no-store")
	return h, nil
}

// submitted indicates that a write may have reached CNAS. Errors after that
// point must never trigger an automatic retry, including authentication errors.
func (c *client) validate(ctx context.Context, report string) (result ValidationResult, submitted bool, err error) {
	ids, e := requestIDs(report)
	if e != nil {
		return result, false, e
	}
	if e = c.backend.Validate(ctx, "ParaclinicServicesValidateRequest.xsd", []byte(report)); e != nil {
		return result, false, e
	}
	h, e := c.sessionHeaders(ctx)
	if e != nil {
		return result, false, e
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if e = ctx.Err(); e != nil {
		return result, false, e
	}
	status, _, b, e := c.backend.Do(ctx, "POST", base+"/svapntws/services/SiuiValidateWS", h, envelope(report))
	if e != nil {
		return result, true, e
	}
	if status != 200 && status != 500 {
		return result, true, fmt.Errorf("CNAS validation HTTP %d", status)
	}
	raw, e := decodeSOAP(b)
	if e != nil {
		return result, true, e
	}
	if status != 200 {
		return result, true, fmt.Errorf("CNAS validation HTTP %d", status)
	}
	if e = c.backend.Validate(ctx, "ParaclinicServicesValidateResponse.xsd", []byte(raw)); e != nil {
		return result, true, e
	}
	result, e = parseResult(raw, ids)
	return result, true, e
}

func (c *client) sessionHeaders(ctx context.Context) (http.Header, error) {
	h, e := c.credentials()
	if e != nil {
		return nil, e
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	// Obtain a fresh session for each batch; its expiry is not documented.
	status, headers, _, e := c.backend.Do(ctx, "GET", base+"/OCSP/validator?username="+url.QueryEscape(c.cfg.Username), h, nil)
	if e != nil {
		return nil, e
	}
	if status != 200 {
		return nil, fmt.Errorf("CNAS authentication HTTP %d", status)
	}
	token := headers.Get("OSCP_RESPONSE") // Protocol spelling, deliberately not OCSP.
	if token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("CNAS did not return a valid OSCP_RESPONSE session")
	}
	h.Set("OSCP_RESPONSE", token)
	h.Set("SessionID", token)
	h.Set("Content-Type", "text/xml; charset=utf-8")
	h.Set("SOAPAction", `""`)

	return h, nil
}
