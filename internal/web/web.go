package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/a-h/templ"
)

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate

//go:embed static/*
var staticFiles embed.FS

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSub(staticFiles, "static")))))
	mux.Handle("GET /_ui/backend-status", templ.Handler(BackendStatus()))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := Page().Render(r.Context(), w); err != nil {
			http.Error(w, "page rendering failed", http.StatusInternalServerError)
		}
	})
	return mux
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
