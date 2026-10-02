package server

import (
	"embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed adminui/index.html adminui/app.js adminui/styles.css
var adminUI embed.FS

func registerAdminUI(r chi.Router) {
	r.Get("/admin", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/admin/", http.StatusTemporaryRedirect)
	})
	for route, asset := range map[string]string{"/admin/": "index.html", "/admin/app.js": "app.js", "/admin/styles.css": "styles.css"} {
		r.Get(route, func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			contentType := "text/html; charset=utf-8"
			if asset == "app.js" {
				contentType = "text/javascript; charset=utf-8"
			}
			if asset == "styles.css" {
				contentType = "text/css; charset=utf-8"
			}
			w.Header().Set("Content-Type", contentType)
			data, err := adminUI.ReadFile("adminui/" + asset)
			if err != nil {
				http.Error(w, "asset unavailable", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(data)
		})
	}
}
