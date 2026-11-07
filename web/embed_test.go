// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	files := fstest.MapFS{
		"index.html":        {Data: []byte("<html>app</html>")},
		"assets/app-abc.js": {Data: []byte("console.log(1)")},
		"favicon.svg":       {Data: []byte("<svg/>")},
		".gitkeep":          {Data: nil},
	}
	h := newHandler(files)

	tests := []struct {
		method, target string
		status         int
		body, cache    string
	}{
		{"GET", "/", 200, "app", "no-cache"},
		{"GET", "/repositories/ci/carbonio-proxy", 200, "app", "no-cache"},
		{"GET", "/runs/ci/x/graph", 200, "app", "no-cache"},
		{"GET", "/assets/app-abc.js", 200, "console", "immutable"},
		{"GET", "/favicon.svg", 200, "<svg", "no-cache"},
		{"GET", "/assets/missing.js", 404, "", ""},
		{"GET", "/missing.png", 404, "", ""},
		{"GET", "/.gitkeep", 404, "", ""},
		{"GET", "/api/v1/nope", 404, "", ""},
		{"GET", "/auth/nope", 404, "", ""},
		{"POST", "/", 405, "", ""},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

		if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.body) {
			t.Errorf("%s %s = %d %q", tt.method, tt.target, rec.Code, rec.Body)
		}

		if got := rec.Header().Get("Cache-Control"); tt.cache != "" && !strings.Contains(got, tt.cache) {
			t.Errorf("%s cache-control = %q, want %q", tt.target, got, tt.cache)
		}
	}
}

func TestSPAHandlerWithoutBuild(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler(fstest.MapFS{".gitkeep": {}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}
