package siui

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

type certificateProfile struct {
	EInvoiceBaseURL                   string `json:"einvoice_base_url" yaml:"einvoice_base_url"`
	AllowRevokedServerCertificate     bool   `json:"allow_revoked_server_certificate" yaml:"allow_revoked_server_certificate"`
	Store                             string `json:"certificate_store" yaml:"certificate_store"`
	Thumbprint                        string `json:"certificate_thumbprint" yaml:"certificate_thumbprint"`
	BaseURL                           string `json:"base_url" yaml:"base_url"`
	PEBaseURL                         string `json:"pe_base_url" yaml:"pe_base_url"`
	AllowInvalidServerCertificateDate bool   `json:"allow_invalid_server_certificate_date" yaml:"allow_invalid_server_certificate_date"`
}
type insuranceHouse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func insuranceHouses() []insuranceHouse {
	b, _ := assets.ReadFile("assets/insurance_houses.json")
	var out []insuranceHouse
	_ = json.Unmarshal(b, &out)
	return out
}
func splitUsername(s string) (string, string) { a, b, _ := strings.Cut(s, "_"); return a, b }
func cnasUsername(fiscal, cas string) (string, error) {
	fiscal = strings.TrimSpace(fiscal)
	cas = strings.TrimSpace(cas)
	if len(fiscal) == 0 || len(fiscal) > 20 {
		return "", errors.New("invalid fiscal code")
	}
	for _, c := range fiscal {
		if c < '0' || c > '9' {
			return "", errors.New("fiscal code must contain digits only")
		}
	}
	for _, h := range insuranceHouses() {
		if h.Code == cas {
			return fiscal + "_" + cas, nil
		}
	}
	return "", errors.New("select a CAS code from the catalogue")
}
func profileKey(store, thumb string) string {
	return store + ":" + strings.ToUpper(strings.ReplaceAll(thumb, " ", ""))
}
func (s *settings) saveActiveProfile() {
	if s.Thumbprint == "" {
		return
	}
	p := certificateProfile{Store: s.Store, Thumbprint: s.Thumbprint, BaseURL: s.BaseURL, PEBaseURL: s.PEBaseURL, EInvoiceBaseURL: s.EInvoiceBaseURL, AllowInvalidServerCertificateDate: s.AllowInvalidServerCertificateDate, AllowRevokedServerCertificate: s.AllowRevokedServerCertificate}
	for i, v := range s.Profiles {
		if profileKey(v.Store, v.Thumbprint) == profileKey(p.Store, p.Thumbprint) {
			s.Profiles[i] = p
			return
		}
	}
	s.Profiles = append(s.Profiles, p)
}
func knownOrigin(s string) bool {
	u, e := url.Parse(s)
	if e != nil {
		return false
	}
	return (u.Hostname() == "www.siui.ro" || u.Hostname() == "testsiui.siui.ro" || u.Hostname() == "sipe.siui.ro") && (u.Port() == "" || u.Port() == "443" || u.Port() == "444")
}
func (s settings) acceptsOrigin(address string) bool {
	u, _ := url.Parse(address)
	for _, p := range s.Profiles {
		for _, v := range []string{p.BaseURL, p.PEBaseURL, p.EInvoiceBaseURL} {
			o, _ := url.Parse(v)
			if o != nil && u != nil && o.Host == u.Host {
				return true
			}
		}
	}
	for _, v := range []string{s.BaseURL, s.PEBaseURL, s.EInvoiceBaseURL} {
		o, _ := url.Parse(v)
		if o != nil && u != nil && o.Host == u.Host {
			return true
		}
	}
	return knownOrigin(address)
}
