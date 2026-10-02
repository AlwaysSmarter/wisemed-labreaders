//go:build windows

package siui

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type nativeBackend struct{ cfg settings }

var winhttp = windows.NewLazySystemDLL("winhttp.dll")
var whOpen = winhttp.NewProc("WinHttpOpen")
var whConnect = winhttp.NewProc("WinHttpConnect")
var whRequest = winhttp.NewProc("WinHttpOpenRequest")
var whOption = winhttp.NewProc("WinHttpSetOption")
var whTimeout = winhttp.NewProc("WinHttpSetTimeouts")
var whSend = winhttp.NewProc("WinHttpSendRequest")
var whReceive = winhttp.NewProc("WinHttpReceiveResponse")
var whQuery = winhttp.NewProc("WinHttpQueryHeaders")
var whRead = winhttp.NewProc("WinHttpReadData")
var whClose = winhttp.NewProc("WinHttpCloseHandle")

func wide(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }
func nativeError(stage string, e error) error {
	return fmt.Errorf("Windows CNAS %s failed (%v)", stage, e)
}
func option(h uintptr, id, value uint32) error {
	r, _, e := whOption.Call(h, uintptr(id), uintptr(unsafe.Pointer(&value)), 4)
	if r == 0 {
		return nativeError("option", e)
	}
	return nil
}
func (n nativeBackend) certificate() (*windows.CertContext, error) {
	hash, e := hex.DecodeString(strings.ReplaceAll(n.cfg.Thumbprint, " ", ""))
	if e != nil || len(hash) != 20 {
		return nil, errors.New("configure the certificate SHA-1 thumbprint (40 hex digits)")
	}
	flags := uint32(windows.CERT_SYSTEM_STORE_CURRENT_USER | windows.CERT_STORE_OPEN_EXISTING_FLAG | windows.CERT_STORE_READONLY_FLAG)
	if n.cfg.Store == "LocalMachine" {
		flags = windows.CERT_SYSTEM_STORE_LOCAL_MACHINE | windows.CERT_STORE_OPEN_EXISTING_FLAG | windows.CERT_STORE_READONLY_FLAG
	}
	name := wide("MY")
	store, e := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0, flags, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if e != nil {
		return nil, errors.New("Windows certificate store unavailable")
	}
	defer windows.CertCloseStore(store, 0)
	blob := windows.CryptDataBlob{Size: uint32(len(hash)), Data: &hash[0]}
	cert, e := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0, windows.CERT_FIND_HASH, unsafe.Pointer(&blob), nil)
	if e != nil {
		return nil, errors.New("configured CNAS certificate not found in Windows MY store")
	}
	parsed, e := x509.ParseCertificate(unsafe.Slice(cert.EncodedCert, cert.Length))
	if e != nil || time.Now().Before(parsed.NotBefore) || time.Now().After(parsed.NotAfter) {
		windows.CertFreeCertificateContext(cert)
		return nil, errors.New("CNAS certificate invalid or expired")
	}
	return cert, nil
}
func (n nativeBackend) Do(ctx context.Context, method, raw string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	if e := ctx.Err(); e != nil {
		return 0, nil, nil, e
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return 0, nil, nil, errors.New("HTTPS required")
	}
	cert, e := n.certificate()
	if e != nil {
		return 0, nil, nil, e
	}
	defer windows.CertFreeCertificateContext(cert)
	agent := wide("WiseMED-SIUI-Bridge/1")
	session, _, e := whOpen.Call(uintptr(unsafe.Pointer(agent)), 4, 0, 0, 0)
	runtime.KeepAlive(agent) // automatic system proxy
	if session == 0 {
		return 0, nil, nil, nativeError("open", e)
	}
	defer whClose.Call(session)
	r, _, e := whTimeout.Call(session, 10000, 10000, 20000, 20000)
	if r == 0 {
		return 0, nil, nil, nativeError("timeouts", e)
	}
	if e = option(session, 84, 0x800); e != nil {
		return 0, nil, nil, e
	} // TLS 1.2, normal Windows certificate validation
	port := 443
	if u.Port() != "" {
		port, e = strconv.Atoi(u.Port())
		if e != nil || port < 1 || port > 65535 {
			return 0, nil, nil, errors.New("invalid CNAS port")
		}
	}
	host := wide(u.Hostname())
	conn, _, e := whConnect.Call(session, uintptr(unsafe.Pointer(host)), uintptr(port), 0)
	runtime.KeepAlive(host)
	if conn == 0 {
		return 0, nil, nil, nativeError("connect", e)
	}
	defer whClose.Call(conn)
	verb, resource, version := wide(method), wide(u.RequestURI()), wide("HTTP/1.1")
	request, _, e := whRequest.Call(conn, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(resource)), uintptr(unsafe.Pointer(version)), 0, 0, 0x800000)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(resource)
	runtime.KeepAlive(version)
	if request == 0 {
		return 0, nil, nil, nativeError("request", e)
	}
	diagnosticID, secureFlags, cleanupDiagnostics, e := attachSecureDiagnostics(request)
	if e != nil {
		whClose.Call(request)
		return 0, nil, nil, e
	}
	defer cleanupDiagnostics()
	var once sync.Once
	closeRequest := func() { once.Do(func() { whClose.Call(request) }) }
	defer closeRequest()
	stop := context.AfterFunc(ctx, closeRequest)
	defer stop()
	if e = option(request, 63, 2|4); e != nil {
		return 0, nil, nil, e
	} // no redirects or implicit authentication
	if e = option(request, 77, 2); e != nil {
		return 0, nil, nil, e
	} // never use logged-in Windows credentials
	r, _, e = whOption.Call(request, 47, uintptr(unsafe.Pointer(cert)), unsafe.Sizeof(*cert))
	if r == 0 {
		return 0, nil, nil, nativeError("client certificate", e)
	}
	// Scoped to this CNAS request: retain CA, hostname and usage validation.
	if n.cfg.AllowInvalidServerCertificateDate {
		if e = option(request, 31, 0x00002000); e != nil {
			return 0, nil, nil, e
		} // WINHTTP_OPTION_SECURITY_FLAGS / IGNORE_CERT_DATE_INVALID
	}
	var hb strings.Builder
	for k, values := range headers {
		for _, v := range values {
			if strings.ContainsAny(k+v, "\r\n\x00") {
				return 0, nil, nil, errors.New("invalid HTTP header")
			}
			hb.WriteString(k + ": " + v + "\r\n")
		}
	}
	hs := wide(hb.String())
	var bp uintptr
	if len(body) > 0 {
		bp = uintptr(unsafe.Pointer(&body[0]))
	}
	r, _, e = whSend.Call(request, uintptr(unsafe.Pointer(hs)), ^uintptr(0)&0xffffffff, bp, uintptr(len(body)), uintptr(len(body)), diagnosticID)
	runtime.KeepAlive(hs)
	runtime.KeepAlive(body)
	if r == 0 {
		return 0, nil, nil, nativeRequestError("send", u.Host, e, secureFlags)
	}
	r, _, e = whReceive.Call(request, 0)
	if r == 0 {
		return 0, nil, nil, nativeRequestError("receive", u.Host, e, secureFlags)
	}
	var status uint32
	size := uint32(4)
	r, _, e = whQuery.Call(request, 19|0x20000000, 0, uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&size)), 0)
	if r == 0 {
		return 0, nil, nil, nativeError("status", e)
	}
	size = 0
	whQuery.Call(request, 22, 0, 0, uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 || size > 65536 {
		return 0, nil, nil, errors.New("invalid HTTP response headers")
	}
	buffer := make([]uint16, (size+1)/2)
	r, _, e = whQuery.Call(request, 22, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r == 0 {
		return 0, nil, nil, nativeError("headers", e)
	}
	responseHeaders := http.Header{}
	for _, line := range strings.Split(windows.UTF16ToString(buffer), "\r\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			responseHeaders.Add(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	var data []byte
	chunk := make([]byte, 16384)
	for {
		var count uint32
		r, _, e = whRead.Call(request, uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)), uintptr(unsafe.Pointer(&count)))
		if r == 0 {
			return 0, nil, nil, nativeRequestError("read", u.Host, e, secureFlags)
		}
		if count == 0 {
			break
		}
		if len(data)+int(count) > maxData {
			return 0, nil, nil, errors.New("CNAS response too large")
		}
		data = append(data, chunk[:count]...)
	}
	if e = ctx.Err(); e != nil {
		return 0, nil, nil, e
	}
	return int(status), responseHeaders, data, nil
}
func (n nativeBackend) Validate(ctx context.Context, schema string, data []byte) error {
	return validateSchema(ctx, schema, data)
}

func listCertificates(location string) ([]Certificate, error) {
	flags := uint32(windows.CERT_SYSTEM_STORE_CURRENT_USER | windows.CERT_STORE_OPEN_EXISTING_FLAG | windows.CERT_STORE_READONLY_FLAG)
	if location == "LocalMachine" {
		flags = windows.CERT_SYSTEM_STORE_LOCAL_MACHINE | windows.CERT_STORE_OPEN_EXISTING_FLAG | windows.CERT_STORE_READONLY_FLAG
	} else if location != "CurrentUser" {
		return nil, errors.New("invalid certificate store")
	}
	name := wide("MY")
	store, e := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0, flags, uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if e != nil {
		return nil, errors.New("Windows certificate store unavailable")
	}
	defer windows.CertCloseStore(store, 0)
	prop := windows.NewLazySystemDLL("crypt32.dll").NewProc("CertGetCertificateContextProperty")
	list := []Certificate{}
	var previous *windows.CertContext
	for {
		cert, err := windows.CertEnumCertificatesInStore(store, previous)
		previous = cert
		if err != nil || cert == nil {
			break
		}
		der := unsafe.Slice(cert.EncodedCert, cert.Length)
		parsed, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		var size uint32
		r, _, _ := prop.Call(uintptr(unsafe.Pointer(cert)), 2, 0, uintptr(unsafe.Pointer(&size)))
		hasKey := r != 0
		if !hasKey {
			r, _, _ = prop.Call(uintptr(unsafe.Pointer(cert)), 5, 0, uintptr(unsafe.Pointer(&size)))
			hasKey = r != 0
		}
		// SHA-1 is the Windows certificate identifier, not a signing algorithm.
		hash := sha1.Sum(der)
		list = append(list, Certificate{Thumbprint: strings.ToUpper(hex.EncodeToString(hash[:])), Subject: parsed.Subject.String(), Issuer: parsed.Issuer.String(), NotBefore: parsed.NotBefore.Format(time.RFC3339), NotAfter: parsed.NotAfter.Format(time.RFC3339), Valid: !time.Now().Before(parsed.NotBefore) && !time.Now().After(parsed.NotAfter), HasPrivateKey: hasKey})
	}
	return list, nil
}
