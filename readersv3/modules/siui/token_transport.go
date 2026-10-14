package siui

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// No HTTP replay: in particular, a PIN or TLS error must never resend an invoice.
func tokenHTTP(ctx context.Context, cfg settings, cert tls.Certificate, method, address string, headers http.Header, body []byte, logf func(string, ...interface{})) (int, http.Header, []byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if e != nil {
		return 0, nil, nil, e
	}
	if req.URL.Scheme != "https" || req.URL.Hostname() == "" || req.URL.User != nil {
		return 0, nil, nil, errors.New("invalid CNAS HTTPS endpoint")
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	if cfg.AllowInvalidServerCertificateDate {
		tc.InsecureSkipVerify = true
		tc.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("missing server certificate")
			}
			leaf := cs.PeerCertificates[0]
			pool := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				pool.AddCert(c)
			}
			opts := x509.VerifyOptions{DNSName: req.URL.Hostname(), Intermediates: pool}
			_, err := leaf.Verify(opts)
			var invalid x509.CertificateInvalidError
			if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
				opts.CurrentTime = leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) / 2)
				_, err = leaf.Verify(opts)
			}
			return err
		}
	}
	// Legacy CNAS servers still negotiate TLS 1.2 RSA suites.
	for _, s := range tls.CipherSuites() {
		if s.ID != tls.TLS_AES_128_GCM_SHA256 && s.ID != tls.TLS_AES_256_GCM_SHA384 && s.ID != tls.TLS_CHACHA20_POLY1305_SHA256 {
			tc.CipherSuites = append(tc.CipherSuites, s.ID)
		}
	}
	tc.CipherSuites = append(tc.CipherSuites, tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_AES_256_CBC_SHA)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{TLSClientConfig: tc, Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 60 * time.Second, ResponseHeaderTimeout: 90 * time.Second, DisableKeepAlives: true, DialContext: func(c context.Context, n, a string) (net.Conn, error) {
		return dialCNAS(c, n, a, dialer.DialContext, logf)
	}}
	defer tr.CloseIdleConnections()
	req.Header = headers.Clone()
	h := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := h.Do(req)
	if e != nil {
		return 0, nil, nil, e
	}
	defer res.Body.Close()
	data, e := io.ReadAll(io.LimitReader(res.Body, maxDownload+1))
	if len(data) > maxDownload {
		return 0, nil, nil, errors.New("CNAS response too large")
	}
	return res.StatusCode, res.Header, data, e
}
