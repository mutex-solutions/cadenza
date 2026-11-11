// Package webapp serves the built web app (web/apps/web/dist, copied into
// dist/ by `make web`) with a fallback to index.html for app routes such as
// /t/<thread-id>. Without a build it serves a short page saying how to build.
package webapp

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const notBuilt = `<!doctype html><meta charset="utf-8"><title>Cadenza</title>
<body style="font-family:system-ui;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1>Cadenza is running</h1><p>The API is up, but this server was built without the web app.
Build it with <code>make web build</code>, or use a release binary.</p></body>`

// Handler serves the app. Hashed assets are cached for a year; index.html is
// never cached so a new release shows up at once.
func Handler() http.Handler {
	sub, _ := fs.Sub(dist, "dist")
	return handler(sub)
}

func handler(files fs.FS) http.Handler {
	index, err := fs.ReadFile(files, "index.html")
	built := err == nil
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !built {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(notBuilt))
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(files, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			// A missing asset is a 404, not the app.
			if strings.HasPrefix(p, "assets/") || path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
