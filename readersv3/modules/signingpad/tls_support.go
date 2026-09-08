package signingpad

import "wisemed-labreaders/readersv3/shared/localtls"

func ensureLocalHTTPSMaterial(configDir, addr string) (string, string, error) {
	return localtls.EnsureMaterial(configDir, addr)
}
