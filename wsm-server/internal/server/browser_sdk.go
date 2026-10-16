package server

import (
	"embed"
	"net/http"
)

//go:embed sdk/wsm.js sdk/siui-bridge.js
var browserSDK embed.FS

func serveBrowserSDK(w http.ResponseWriter, r *http.Request) {
	asset := "sdk/wsm.js"
	if r.URL.Path == "/sdk/siui-bridge.js" {
		asset = "sdk/siui-bridge.js"
	}
	data, err := browserSDK.ReadFile(asset)
	if err != nil {
		http.Error(w, "SDK unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Public static code only; no credentials or tenant data.
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	_, _ = w.Write(data)
}
