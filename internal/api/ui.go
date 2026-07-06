package api

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// The production frontend (web/ Vite build, copied into webdist by `make
// web`) serves the two surfaces from the design handoff:
//
//	/         — Surface B, the paper control center
//	/ui/touch — Surface A · Round 2, the rack instrument face
//
// If webdist is empty (backend-only build), the Phase 0 static shells under
// static/ keep the same URLs alive.
//
//go:embed static/*.html
var legacyFS embed.FS

//go:embed all:webdist
var webdistFS embed.FS

func (s *Server) uiRoutes(r chi.Router) {
	dist, err := fs.Sub(webdistFS, "webdist")
	hasDist := err == nil
	if hasDist {
		if _, err := fs.Stat(dist, "index.html"); err != nil {
			hasDist = false
		}
	}

	if hasDist {
		files := http.FileServer(http.FS(dist))
		r.Get("/", servePage(dist, "index.html"))
		r.Get("/ui/touch", servePage(dist, "touch.html"))
		r.Get("/assets/*", files.ServeHTTP)
		r.Get("/fonts/*", files.ServeHTTP)
		r.Get("/seal.png", files.ServeHTTP)
		return
	}

	r.Get("/", serveLegacy("static/index.html"))
	r.Get("/ui/touch", serveLegacy("static/touch.html"))
}

func servePage(dist fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := fs.ReadFile(dist, name)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	}
}

func serveLegacy(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := legacyFS.ReadFile(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	}
}
