package siui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ocspTimeout = 120 * time.Second

type backend interface {
	Do(context.Context, string, string, http.Header, []byte) (int, http.Header, []byte, error)
	Validate(context.Context, string, []byte) error
}
type settings struct {
	pin                                                     *pinVault
	PKCS11Module                                            string
	Username, Licence, Thumbprint, Store, BaseURL, Database string
	AllowInvalidServerCertificateDate                       bool
	AllowRevokedServerCertificate                           bool
	PEBaseURL                                               string
	EInvoiceBaseURL                                         string
	Profiles                                                []certificateProfile
}
type client struct {
	session       *ocspSession
	sessionReused bool
	sessionAge    int
	sessionID     string
	cfg           settings
	backend       backend
	logf          func(string, ...interface{})
	operation     string
	callID        string
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
	address := ""
	defer func() { err = withConnectionError(address, err) }()
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
	address = base + "/svapntws/services/SiuiValidateWS"
	status, _, b, e := c.backend.Do(ctx, "POST", address, h, envelope(report))
	if e != nil {
		return result, !connectionFailedBeforeSend(e), e
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

func (c *client) sessionHeaders(ctx context.Context) (resultHeaders http.Header, err error) {
	address := strings.TrimRight(c.cfg.BaseURL, "/") + "/OCSP/validator"
	started := time.Now()
	defer func() {
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("OCSP: timpul de așteptare a expirat după %.1f secunde; autentificarea nu s-a încheiat: %w", time.Since(started).Seconds(), err)
		}
		err = withConnectionError(address, err)
	}()
	h, e := c.credentials()
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	c.sessionReused = false
	c.sessionAge = 0
	if c.session != nil {
		c.session.mu.Lock()
		defer c.session.mu.Unlock()
		age := time.Since(c.session.acquired)
		if c.session.token != "" && c.session.key == ocspConfigKey(c.cfg) && age >= 0 && age <= ocspMaxAge {
			c.sessionReused = true
			c.sessionAge = int(age.Seconds())
			digest := sha256.Sum256([]byte(c.session.token))
			c.sessionID = fmt.Sprintf("%x", digest[:6])
			if c.logf != nil {
				c.logf("SIUI OCSP: folosesc token-ul OCSP %s cu vechime de %d secunde operation=%s id=%s", c.sessionID, c.sessionAge, c.operation, c.callID)
			}
			return ocspHeaders(h, c.session.token), nil
		}
		c.session.token = ""
	}
	httpStatus := 0
	if deadline, ok := ctx.Deadline(); ok && c.logf != nil {
		c.logf("SIUI OCSP: timp disponibil %.1f secunde", time.Until(deadline).Seconds())
	}
	if c.logf != nil {
		d := c.ocspDebug()
		c.logf("SIUI OCSP: conectare operation=%s id=%s method=GET url=%s port=%s username=%s certificate_store=%s certificate_thumbprint=%s licence_configured=%t; providerCategory nu este parametru OCSP", c.operation, c.callID, d["url"], d["port"], c.cfg.Username, c.cfg.Store, c.cfg.Thumbprint, d["licence_configured"])
	}
	defer func() {
		if c.logf != nil {
			c.logf("SIUI OCSP: durata %.1f secunde operation=%s id=%s", time.Since(started).Seconds(), c.operation, c.callID)
		}
		if err != nil && c.logf != nil {
			if httpStatus != 0 {
				c.logf("[ERROR] SIUI OCSP: autentificare esuata HTTP=%d operation=%s id=%s; token neobtinut", httpStatus, c.operation, c.callID)
			} else {
				c.logf("[ERROR] SIUI OCSP: autentificare esuata operation=%s id=%s; token neobtinut", c.operation, c.callID)
			}
		}
	}()
	// Re-enumerate token identities after USB disconnect/reconnect, as in Settings.
	if native, ok := c.backend.(interface{ refreshCertificates() error }); ok {
		if err := native.refreshCertificates(); err != nil {
			return nil, fmt.Errorf("reload certificates: %w", err)
		}
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	// Acquire only when no matching session remains within the requested 150-second reuse window.
	status, headers, _, e := c.backend.Do(ctx, "GET", base+"/OCSP/validator?username="+url.QueryEscape(c.cfg.Username), h, nil)
	if e != nil {
		return nil, e
	}
	httpStatus = status
	if status != 200 {
		return nil, fmt.Errorf("CNAS authentication HTTP %d", status)
	}
	token := headers.Get("OSCP_RESPONSE") // Protocol spelling, deliberately not OCSP.
	if token == "" || len(token) > 8192 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("CNAS did not return a valid OSCP_RESPONSE session")
	}
	// Log successful authentication before SOAP, without persisting session credentials.
	if c.logf != nil {
		c.logf("SIUI OCSP: token obtinut cu succes operation=%s id=%s", c.operation, c.callID)
	}
	if c.session != nil {
		c.session.token = token
		c.session.acquired = time.Now()
		c.session.key = ocspConfigKey(c.cfg)
	}
	digest := sha256.Sum256([]byte(token))
	c.sessionID = fmt.Sprintf("%x", digest[:6])
	if c.logf != nil {
		c.logf("SIUI OCSP: folosesc token-ul OCSP %s cu vechime de 0 secunde (nou) operation=%s id=%s", c.sessionID, c.operation, c.callID)
	}
	return ocspHeaders(h, token), nil
}

func (n nativeBackend) refreshCertificates() error {
	_, err := configuredCertificates(n.cfg)
	return err
}

// Diagnostic metadata describes the actual connection, never Basic credentials or tokens.
func (c *client) ocspDebug() map[string]any {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	u, _ := url.Parse(base)
	port := "443"
	if u != nil && u.Port() != "" {
		port = u.Port()
	}
	fiscal, cas := splitUsername(c.cfg.Username)
	return map[string]any{"timeout_seconds": int(ocspTimeout / time.Second), "method": "GET", "url": base + "/OCSP/validator?username=" + url.QueryEscape(c.cfg.Username), "port": port, "username": c.cfg.Username, "fiscal_code": fiscal, "cas_code": cas, "certificate_store": c.cfg.Store, "certificate_thumbprint": c.cfg.Thumbprint, "licence_configured": strings.TrimSpace(c.cfg.Licence) != "", "providerCategory_sent_to_ocsp": false}
}

type connectionError struct {
	address string
	cause   error
}

func (e *connectionError) Error() string {
	u, err := url.Parse(e.address)
	if err != nil || u.Host == "" {
		return "Server: conexiunea nu a fost inițiată\n" + e.cause.Error()
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	// Download query strings may contain access tokens; the server and path suffice.
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return fmt.Sprintf("Server: %s\nPort: %s\n%s", u.String(), port, e.cause.Error())
}
func (e *connectionError) Unwrap() error { return e.cause }
func withConnectionError(address string, err error) error {
	if err == nil {
		return nil
	}
	var existing *connectionError
	if errors.As(err, &existing) {
		return err
	}
	return &connectionError{address: address, cause: err}
}

// A typed dial failure proves that this request never established its connection.
// Timeouts during write/read remain uncertain; never classify by error text.
func connectionFailedBeforeSend(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}
