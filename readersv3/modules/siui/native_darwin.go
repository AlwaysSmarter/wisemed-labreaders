//go:build darwin

package siui

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"
)

type nativeBackend struct {
	cfg  settings
	logf func(string, ...interface{})
}

func listCertificates(store string) ([]Certificate, error) {
	a, err := securityFramework()
	if err != nil {
		return nil, err
	}
	identities, err := a.identities(store)
	if err != nil {
		return nil, err
	}
	result := []Certificate{}
	if identities == 0 {
		return result, nil
	}
	defer a.release(identities)
	for i := int64(0); i < a.arrayCount(identities); i++ {
		identity := a.arrayValue(identities, i)
		der, err := a.identityDER(identity)
		if err != nil {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		var key uintptr
		status := a.identityKey(identity, &key)
		hasKey := status == 0 && key != 0
		if key != 0 {
			a.release(key)
		}
		now := time.Now()
		result = append(result, Certificate{Thumbprint: fmt.Sprintf("%X", sha1.Sum(der)), Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), NotBefore: cert.NotBefore.Format(time.RFC3339), NotAfter: cert.NotAfter.Format(time.RFC3339), Valid: !now.Before(cert.NotBefore) && !now.After(cert.NotAfter), HasPrivateKey: hasKey})
	}
	return result, nil
}

func (a *securityAPI) clientCertificate(cfg settings) (tls.Certificate, func(), error) {
	empty := tls.Certificate{}
	noop := func() {}
	pin, finish, err := cfg.pin.begin(cfg)
	if err != nil {
		return empty, noop, err
	}
	auth, cleanAuth, err := macPINContext(a, pin)
	if err != nil {
		finish(false)
		return empty, noop, err
	}
	var authOnce sync.Once
	endAuth := func(success bool) { authOnce.Do(func() { cleanAuth(); finish(success) }) }
	handedOff := false
	defer func() {
		if !handedOff {
			endAuth(false)
		}
	}()
	identities, err := a.identitiesWithContext(cfg.Store, auth)
	if err != nil {
		return empty, noop, err
	}
	if identities == 0 {
		return empty, noop, errors.New("macOS: no client identities found; connect the token and enable its CryptoTokenKit driver")
	}
	defer a.release(identities)
	for i := int64(0); i < a.arrayCount(identities); i++ {
		identity := a.arrayValue(identities, i)
		der, err := a.identityDER(identity)
		if err != nil {
			continue
		}
		if !strings.EqualFold(fmt.Sprintf("%X", sha1.Sum(der)), cfg.Thumbprint) {
			continue
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return empty, noop, err
		}
		if now := time.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			return empty, noop, errors.New("macOS: selected client certificate is expired or not yet valid")
		}
		var key uintptr
		if status := a.identityKey(identity, &key); status != 0 || key == 0 {
			return empty, noop, fmt.Errorf("macOS: token private key unavailable (OSStatus %d)", status)
		}
		signer := &macSigner{api: a, key: key, public: cert.PublicKey}
		if len(pin) > 0 {
			signer.pinFinish = endAuth
		}
		signatureAlgorithms := []tls.SignatureScheme{}
		for _, candidate := range []struct {
			scheme    tls.SignatureScheme
			algorithm string
		}{
			{tls.PKCS1WithSHA256, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256"},
			{tls.PKCS1WithSHA384, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA384"},
			{tls.PKCS1WithSHA512, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA512"},
			{tls.PSSWithSHA256, "kSecKeyAlgorithmRSASignatureDigestPSSSHA256"},
			{tls.PSSWithSHA384, "kSecKeyAlgorithmRSASignatureDigestPSSSHA384"},
			{tls.PSSWithSHA512, "kSecKeyAlgorithmRSASignatureDigestPSSSHA512"},
			{tls.ECDSAWithP256AndSHA256, "kSecKeyAlgorithmECDSASignatureDigestX962SHA256"},
			{tls.ECDSAWithP384AndSHA384, "kSecKeyAlgorithmECDSASignatureDigestX962SHA384"},
			{tls.ECDSAWithP521AndSHA512, "kSecKeyAlgorithmECDSASignatureDigestX962SHA512"},
		} {
			if a.supports(key, secKeyOperationSign, a.constant(candidate.algorithm)) {
				signatureAlgorithms = append(signatureAlgorithms, candidate.scheme)
			}
		}
		if len(signatureAlgorithms) == 0 {
			a.release(key)
			return empty, noop, errors.New("macOS: token driver offers no supported TLS signing algorithm")
		}
		handedOff = true
		return tls.Certificate{Certificate: a.clientChain(der), PrivateKey: signer, Leaf: cert, SupportedSignatureAlgorithms: signatureAlgorithms}, func() { endAuth(false); a.release(key) }, nil
	}
	return empty, noop, errors.New("macOS: selected certificate not found in Keychain / CryptoTokenKit; reconnect token and reload certificates")
}

func (n nativeBackend) Do(ctx context.Context, method, raw string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, raw, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	if request.URL.Scheme != "https" || request.URL.Hostname() == "" || request.URL.User != nil {
		return 0, nil, nil, errors.New("CNAS requires an HTTPS URL without embedded credentials")
	}
	a, err := securityFramework()
	if err != nil {
		return 0, nil, nil, err
	}
	cert, release, err := a.clientCertificate(n.cfg)
	if err != nil {
		return 0, nil, nil, err
	}
	defer release()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert},
		// Native Security.framework performs full hostname and chain validation below.
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			rawCerts := make([][]byte, len(state.PeerCertificates))
			for i, c := range state.PeerCertificates {
				rawCerts[i] = c.Raw
			}
			return a.verifyServer(rawCerts, request.URL.Hostname(), n.cfg.AllowInvalidServerCertificateDate, n.cfg.AllowRevokedServerCertificate)
		},
	}
	for _, suite := range tls.CipherSuites() {
		if suite.ID != tls.TLS_AES_128_GCM_SHA256 && suite.ID != tls.TLS_AES_256_GCM_SHA384 && suite.ID != tls.TLS_CHACHA20_POLY1305_SHA256 {
			cfg.CipherSuites = append(cfg.CipherSuites, suite.ID)
		}
	}
	cfg.CipherSuites = append(cfg.CipherSuites, tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_AES_256_CBC_SHA)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialCNAS(ctx, network, address, dialer.DialContext, n.logf)
	}, Proxy: http.ProxyFromEnvironment, TLSClientConfig: cfg, TLSHandshakeTimeout: 60 * time.Second, ResponseHeaderTimeout: 90 * time.Second, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request.Header = headers.Clone()
	if n.logf != nil {
		var tlsStarted time.Time
		trace := &httptrace.ClientTrace{
			TLSHandshakeStart: func() { tlsStarted = time.Now(); n.logf("SIUI TLS: negociere server=%s", request.URL.Host) },
			TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
				if err != nil {
					n.logf("[ERROR] SIUI TLS: server=%s durata=%.1fs eroare=%v", request.URL.Host, time.Since(tlsStarted).Seconds(), err)
				} else {
					n.logf("SIUI TLS: negociere reusita server=%s durata=%.1fs", request.URL.Host, time.Since(tlsStarted).Seconds())
				}
			},
		}
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("macOS CNAS request failed: %w", err)
	}
	defer response.Body.Close()
	limit := maxData
	if method == http.MethodGet && !strings.HasPrefix(request.URL.Path, "/OCSP/") {
		limit = maxDownload
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return 0, nil, nil, err
	}
	if len(data) > limit {
		return 0, nil, nil, errors.New("CNAS response exceeds size limit")
	}
	return response.StatusCode, response.Header, data, nil
}
func (n nativeBackend) Validate(ctx context.Context, schema string, data []byte) error {
	return validateSchema(ctx, schema, data)
}
