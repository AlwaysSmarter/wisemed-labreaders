package siui

type Certificate struct {
	Thumbprint    string `json:"thumbprint"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	NotBefore     string `json:"not_before"`
	NotAfter      string `json:"not_after"`
	Valid         bool   `json:"valid"`
	HasPrivateKey bool   `json:"has_private_key"`
}
