//go:build darwin || linux

package runner

import (
	"wisemed-labreaders/readersv3/core/config"
	"wisemed-labreaders/readersv3/core/runtime"
)

func runServiceManager(_ *config.Config, _ *runtime.App) (bool, error) {
	return false, nil
}
