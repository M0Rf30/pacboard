// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api serves the HTTP/JSON contract of docs/api.md.
package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/auth"
	"github.com/M0Rf30/pacboard/internal/github"
	"github.com/M0Rf30/pacboard/internal/httpx"
	"github.com/M0Rf30/pacboard/internal/kube"
	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/internal/model"
)

// Store is the read-only view of the cluster the handlers need; kube.Store implements it.
type Store interface {
	Ready() bool
	Repositories() []model.RepositorySummary
	Runs(f kube.RunFilter) []model.RunSummary
	Run(namespace, name string) (*unstructured.Unstructured, bool)
	TaskRunsOf(plr *unstructured.Unstructured) []*unstructured.Unstructured
	PipelineSpec(ctx context.Context, plr *unstructured.Unstructured) (map[string]any, error)
	Subscribe() (<-chan kube.Event, func())
}

// LogStreamer writes step logs; logs.Streamer implements it.
type LogStreamer interface {
	Stream(ctx context.Context, w io.Writer, namespace, taskRun, step string, follow bool) error
	// Tail reads at most limit bytes of one step's log, without following.
	Tail(ctx context.Context, namespace, taskRun, step string, limit int) (logs.Excerpt, error)
}

// Deps are the collaborators of the API.
type Deps struct {
	Store  Store
	Logs   LogStreamer
	Auth   *auth.Service
	GitHub *github.Client
	// UI serves everything that is not /api, /auth, /healthz or /readyz (the SPA).
	UI  http.Handler
	Log *slog.Logger
	Now func() time.Time
}

type handler struct {
	Deps

	failures failureCache
}

// heartbeat is the interval of SSE comments that keep proxies from closing an idle stream; a variable
// so that tests can shorten it.
var heartbeat = 20 * time.Second

// New builds the HTTP handler: routes, auth gates, security headers, request logging.
func New(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}

	h := &handler{Deps: d}
	mux := http.NewServeMux()

	d.Auth.Routes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /readyz", h.readyz)

	// /me is public (always 200); every other API route sits behind the auth gate and,
	// for unsafe methods, the CSRF check.
	mux.HandleFunc("GET /api/v1/me", h.me)

	guarded := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, d.Auth.Authenticate(d.Auth.RequireCSRF(fn)))
	}

	guarded("GET /api/v1/repositories", h.ready(h.repositories))
	guarded("GET /api/v1/repositories/{namespace}/{name}/runs", h.ready(h.repositoryRuns))
	guarded("GET /api/v1/runs", h.ready(h.runs))
	guarded("GET /api/v1/runs/{namespace}/{name}", h.ready(h.run))
	guarded("GET /api/v1/runs/{namespace}/{name}/tasks/{task}/logs", h.ready(h.logs))
	guarded("GET /api/v1/events", h.ready(h.events))
	guarded("POST /api/v1/runs/{namespace}/{name}/rerun", h.ready(h.rerun))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { httpx.Error(w, http.StatusNotFound, "not found") })
	mux.HandleFunc("/auth/", func(w http.ResponseWriter, _ *http.Request) { httpx.Error(w, http.StatusNotFound, "not found") })

	if d.UI != nil {
		mux.Handle("/", d.UI)
	}

	return recoverer(d.Log, securityHeaders(requestLog(d.Log, d.Auth.EnsureCSRFCookie(mux))))
}

func (h *handler) readyz(w http.ResponseWriter, _ *http.Request) {
	if !h.Store.Ready() {
		httpx.Error(w, http.StatusServiceUnavailable, "informers not synced yet")

		return
	}

	_, _ = io.WriteString(w, "ok\n")
}

// ready answers 503 until the informer caches hold the cluster state.
func (h *handler) ready(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.Store.Ready() {
			httpx.Error(w, http.StatusServiceUnavailable, "informers not synced yet")

			return
		}

		next(w, r)
	}
}
