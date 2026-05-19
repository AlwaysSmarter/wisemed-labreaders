package appmeta

import "strings"

var (
	Version       = "0.0.0-dev"
	Commit        = "unknown"
	BuildTime     = ""
	VersionMarker = "WISEMED_APP_VERSION=0.0.0-dev"
)

func CurrentVersion() string {
	value := strings.TrimSpace(Version)
	if value == "" {
		return "0.0.0-dev"
	}
	return strings.TrimPrefix(value, "v")
}

func CurrentCommit() string {
	value := strings.TrimSpace(Commit)
	if value == "" {
		return "unknown"
	}
	return value
}

func CurrentBuildTime() string {
	return strings.TrimSpace(BuildTime)
}
