package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var files embed.FS

func Handler() http.Handler {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || !strings.Contains(path.Base(name), ".") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
