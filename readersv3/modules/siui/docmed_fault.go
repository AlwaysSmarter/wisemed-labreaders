package siui

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Fault diagnostics are retained only in the owner-scoped call/file. They are
// never written to operational logs and never authorize replaying a mutation.
type docMedSOAPFault struct {
	Code                    string `json:"code,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	Actor                   string `json:"actor,omitempty"`
	Detail                  string `json:"detail,omitempty"`
	HTTPStatus              int    `json:"http_status,omitempty"`
	DetailXML               string `json:"detail_xml,omitempty"`
	RawXML                  []byte `json:"-"`
	WriteMayHaveBeenApplied bool   `json:"write_may_have_been_applied,omitempty"`
}

func (f *docMedSOAPFault) Error() string {
	message := "DocMED SOAP fault"
	if f.Code != "" {
		message += " [" + f.Code + "]"
	}
	if f.Reason != "" {
		message += ": " + f.Reason
	}
	if f.Detail != "" {
		message += "\nDetail: " + f.Detail
	}
	if f.WriteMayHaveBeenApplied {
		message += "\nRezultatul scrierii este incert; verifică seriile existente înainte de repetare."
	}
	return message
}
func faultText(n node) string {
	texts := []string{}
	var visit func(node)
	visit = func(n node) {
		if text := strings.TrimSpace(n.Text); text != "" {
			texts = append(texts, text)
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(n)
	return strings.Join(texts, "\n")
}
func boundedFaultText(text string, limit int) string {
	r := []rune(text)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return text
}
func parseDocMedFault(response node) (*docMedSOAPFault, error) {
	raw, err := standaloneDocMedXML(response)
	if err != nil {
		return nil, err
	}
	return &docMedSOAPFault{Code: faultText(response.child("faultcode")), Reason: faultText(response.child("faultstring")), Actor: faultText(response.child("faultactor")), Detail: faultText(response.child("detail")), RawXML: raw}, nil
}
func (f *docMedSOAPFault) redact(secrets ...string) error {
	root, err := parseXMLLimit(f.RawXML, docMedXMLLimit)
	if err != nil {
		return fmt.Errorf("cannot read DocMED fault diagnostics: %w", err)
	}
	replace := func(text string) string {
		for _, secret := range secrets {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[redacted]")
			}
		}
		return text
	}
	var clean func(*node)
	clean = func(n *node) {
		n.Text = replace(n.Text)
		for i := range n.Attrs {
			if n.Attrs[i].Name.Space != "xmlns" && n.Attrs[i].Name.Local != "xmlns" {
				n.Attrs[i].Value = replace(n.Attrs[i].Value)
			}
		}
		for i := range n.Children {
			clean(&n.Children[i])
		}
	}
	clean(&root)
	f.Code = boundedFaultText(faultText(root.child("faultcode")), 128)
	f.Reason = boundedFaultText(faultText(root.child("faultstring")), 1024)
	f.Actor = boundedFaultText(faultText(root.child("faultactor")), 512)
	f.Detail = boundedFaultText(faultText(root.child("detail")), 2048)
	if detail := root.child("detail"); detail.XMLName.Local != "" {
		if raw, e := standaloneDocMedXML(detail); e == nil {
			f.DetailXML = boundedFaultText(string(raw), 4096)
		}
	}
	f.RawXML, err = standaloneDocMedXML(root)
	return err
}
func isDocMedFaultRoot(root node) bool {
	return root.XMLName == (xml.Name{Space: soapNS, Local: "Fault"})
}
