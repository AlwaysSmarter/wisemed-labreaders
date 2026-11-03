package siui

import "strings"

// Production origins published by CNAS in PIAS 3.7.34 / DocMed 1.0 (20 October 2026).
const (
	DefaultBaseURL       = "https://siui.e-pias.ro"
	DefaultPEBaseURL     = "https://sipe.e-pias.ro"
	DefaultDocMedBaseURL = "https://docmed.e-pias.ro"
)

// DocMed uses document/literal SOAP contracts, not the legacy SIUI RPC routes.
// Keep these destinations separate; configuring them does not submit documents.
func (s settings) docMedEndpoints() map[string]string {
	base := s.DocMedBaseURL
	if base == "" {
		base = DefaultDocMedBaseURL
	}
	base = strings.TrimRight(base, "/")
	return map[string]string{
		"DocMedService":             base + "/loader/ws",
		"DocumentSeries":            base + "/seq/ws",
		"MedicalDocumentDownloader": base + "/download/ws",
		"DocStatusChangerService":   base + "/status/ws",
	}
}
