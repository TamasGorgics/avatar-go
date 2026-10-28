package web

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed assets/app.css assets/app.js assets/favicon.svg assets/portrait.jpg
var assetsFS embed.FS

func assetHandler() http.Handler {
	files := http.FileServerFS(assetsFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		files.ServeHTTP(w, r)
	})
}
