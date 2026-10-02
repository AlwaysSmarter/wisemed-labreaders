//go:build windows

package main

import (
	"context"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

// Windows operators restart the process after changing credentials/certificates.
func watchReload(context.Context, *server.Server, string) func() { return func() {} }
