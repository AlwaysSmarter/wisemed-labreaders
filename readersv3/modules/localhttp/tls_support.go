package localhttp

import "wisemed-labreaders/readersv3/shared/localtls"

func ensureLocalHTTPSMaterial(configDir, addr string, logf func(string, ...any)) (string, string, error) {
	return localtls.EnsureMaterialWithLogger(configDir, addr, logf)
}
