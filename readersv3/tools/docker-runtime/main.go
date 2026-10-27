// docker-runtime checks config readability and the local UI without credentials.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/shared/appmeta"
)

func main() {
	if len(os.Args) != 3 {
		fail(fmt.Errorf("usage: docker-runtime check-config|health CONFIG"))
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func run(mode, path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	expected := map[string]string{"siui-bridge": "siui", "siui-bridge-history": "siui-history", "wisemed-siui-data": "siui-data"}[os.Getenv("WISEMED_DOCKER_APP")]
	if expected == "" || cfg.Analyzer.Protocol != expected {
		return fmt.Errorf("configuration protocol does not match Docker application")
	}
	host, port, err := net.SplitHostPort(cfg.LocalHTTP.Address)
	if err != nil {
		return err
	}
	// Host networking retains EC2 loopback database access without publishing the UI.
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("Docker local_http.address must use a numeric loopback address")
	}
	if mode == "check-config" {
		return nil
	}
	if mode != "health" {
		return fmt.Errorf("unknown mode %q", mode)
	}
	scheme := "http"
	transport := &http.Transport{Proxy: nil}
	if cfg.LocalHTTP.TLS {
		scheme = "https"
		pem, err := os.ReadFile(filepath.Join(filepath.Dir(path), "tls", "wisemed-local-root-ca.pem"))
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid local TLS CA")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(scheme + "://" + net.JoinHostPort(host, port) + "/api/session")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("local UI returned %s", response.Status)
	}
	var payload struct {
		OK     bool `json:"ok"`
		Reader struct {
			Protocol string `json:"protocol"`
			Version  string `json:"app_version"`
		} `json:"reader"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return err
	}
	if !payload.OK || payload.Reader.Protocol != expected || payload.Reader.Version != appmeta.CurrentVersion() {
		return fmt.Errorf("local UI application/version mismatch")
	}
	return nil
}
