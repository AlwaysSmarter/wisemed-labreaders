//go:build !linux

package siui

func configuredCertificates(s settings) ([]Certificate, error) { return listCertificates(s.Store) }
