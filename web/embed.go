// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package web embeds the built SPA (web/dist) and serves it with history-mode fallback.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// dist is the Vite build output (untracked). The tracked dist/.gitkeep keeps the pattern valid
// before the UI is built; until then the handler answers 503 with a hint.
//
//go:embed all:dist
var distFS embed.FS

const (
	immutable  = "public, max-age=31536000, immutable"
	revalidate = "no-cache"
)

// Handler serves the embedded SPA: files from dist, index.html for every other path
// without a file extension. /api/ and /auth/ never fall back to the SPA.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: " + err.Error())
	}

	return newHandler(sub)
}

func newHandler(files fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

			return
		}

		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean == "" {
			clean = "index.html"
		}

		if strings.HasPrefix(clean, "api/") || strings.HasPrefix(clean, "auth/") {
			http.NotFound(w, r)

			return
		}

		if serve(w, r, files, clean) {
			return
		}

		// A missing asset or other file must 404, not return HTML with a 200.
		if clean != "index.html" && (strings.HasPrefix(clean, "assets/") || path.Ext(clean) != "") {
			http.NotFound(w, r)

			return
		}

		if !serve(w, r, files, "index.html") {
			http.Error(w, "UI not built: run `make web-build`", http.StatusServiceUnavailable)
		}
	})
}

func serve(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	if path.Base(name) == ".gitkeep" {
		return false
	}

	data, err := fs.ReadFile(files, name)
	if err != nil {
		return false
	}

	cache := revalidate
	if strings.HasPrefix(name, "assets/") {
		cache = immutable
	}

	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))

	return true
}
