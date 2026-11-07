// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/auth"
	"github.com/M0Rf30/pacboard/internal/github"
	"github.com/M0Rf30/pacboard/internal/kube"
	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/internal/model"
)

const pac = "pipelinesascode.tekton.dev/"

type fakeStore struct {
	ready  bool
	runs   map[string]*unstructured.Unstructured
	events chan kube.Event
	last   kube.RunFilter
}

func (f *fakeStore) Ready() bool                             { return f.ready }
func (f *fakeStore) Repositories() []model.RepositorySummary { return []model.RepositorySummary{} }
func (f *fakeStore) TaskRunsOf(*unstructured.Unstructured) []*unstructured.Unstructured {
	return nil
}

func (f *fakeStore) Runs(fl kube.RunFilter) []model.RunSummary {
	f.last = fl

	return []model.RunSummary{}
}

func (f *fakeStore) Run(ns, name string) (*unstructured.Unstructured, bool) {
	r, ok := f.runs[ns+"/"+name]

	return r, ok
}

func (f *fakeStore) PipelineSpec(context.Context, *unstructured.Unstructured) (map[string]any, error) {
	return map[string]any{"tasks": []any{map[string]any{"name": "clone"}, map[string]any{"name": "build", "runAfter": []any{"clone"}}}}, nil
}

func (f *fakeStore) Subscribe() (<-chan kube.Event, func()) { return f.events, func() {} }

type fakeLogs struct{ err error }

func (f fakeLogs) Stream(_ context.Context, w io.Writer, _, _, step string, _ bool) error {
	if f.err != nil {
		return f.err
	}

	_, _ = io.WriteString(w, "log of "+step)

	return nil
}

func (f fakeLogs) Tail(context.Context, string, string, string, int) (logs.Excerpt, error) {
	return logs.Excerpt{}, f.err
}

func plr(name string, meta map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "ci"},
		"status":   map[string]any{"conditions": []any{map[string]any{"type": "Succeeded", "status": "True", "reason": "Completed"}}},
	}}
	u.SetAnnotations(meta)

	return u
}

func prRun() *unstructured.Unstructured {
	return plr("with-pr", map[string]string{
		pac + "pull-request": "323", pac + "url-org": "zextras", pac + "url-repository": "carbonio-proxy",
		pac + "original-prname": "carbonio-proxy", pac + "repository": "carbonio-proxy",
	})
}

// fakeGitHub records the comments it receives.
type fakeGitHub struct {
	srv *httptest.Server

	mu       sync.Mutex
	comments []recorded
	status   int // status for the comment POST, 0 = 201
}

type recorded struct{ path, auth, body string }

func newGitHub(t *testing.T) *fakeGitHub {
	t.Helper()

	f := &fakeGitHub{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v3/user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "devuser", "avatar_url": "https://avatars.example/devuser"})
	})
	mux.HandleFunc("POST /api/v3/repos/{org}/{repo}/issues/{n}/comments", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}

		_ = json.NewDecoder(r.Body).Decode(&body)

		f.mu.Lock()
		f.comments = append(f.comments, recorded{r.URL.Path, r.Header.Get("Authorization"), body.Body})
		status := f.status
		f.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"message":"nope"}`)

			return
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"html_url":"https://github.com/zextras/carbonio-proxy/pull/323#issuecomment-1"}`)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	return f
}

type env struct {
	h     http.Handler
	store *fakeStore
	gh    *fakeGitHub
}

func newEnv(t *testing.T, mode string, devToken bool, lg LogStreamer) *env {
	t.Helper()

	gh := newGitHub(t)

	client, err := github.New(gh.srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	cfg := auth.Config{Mode: mode, GitHub: client, Logger: slog.New(slog.DiscardHandler)}

	switch {
	case mode == auth.ModeGitHub:
		cfg.ClientID, cfg.ClientSecret, cfg.AllowedOrgs = "cid", "secret", []string{"acme"}
		cfg.SessionKey, cfg.PublicURL = "pacboard-api-test-session-key-0123456789", "https://pacboard.example.com"
	case devToken:
		cfg.DevToken = "dev-token"
	}

	a, err := auth.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	store := &fakeStore{ready: true, events: make(chan kube.Event, 4), runs: map[string]*unstructured.Unstructured{
		"ci/with-pr": prRun(),
		"ci/without-pr": plr(
			"without-pr",
			map[string]string{pac + "original-prname": "carbonio-proxy", pac + "url-org": "o", pac + "url-repository": "r"},
		),
		"ci/no-name": plr("no-name", map[string]string{pac + "pull-request": "1", pac + "url-org": "o", pac + "url-repository": "r"}),
		"ci/dotdot-org": plr("dotdot-org", map[string]string{
			pac + "pull-request": "1", pac + "url-org": "..", pac + "url-repository": "r", pac + "original-prname": "p",
		}),
		"ci/dot-repo": plr("dot-repo", map[string]string{
			pac + "pull-request": "1", pac + "url-org": "o", pac + "url-repository": ".", pac + "original-prname": "p",
		}),
		"ci/hidden-repo": plr("hidden-repo", map[string]string{
			pac + "pull-request": "1", pac + "url-org": "o", pac + "url-repository": ".git", pac + "original-prname": "p",
		}),
		"ci/dotdot-name": plr("dotdot-name", map[string]string{
			pac + "pull-request": "1", pac + "url-org": "o", pac + "url-repository": "r", pac + "original-prname": "..",
		}),
		"ci/bad-name": plr("bad-name", map[string]string{
			pac + "pull-request": "1", pac + "url-org": "o", pac + "url-repository": "r", pac + "original-prname": "x\n/approve",
		}),
	}}

	h := New(Deps{Store: store, Logs: lg, Auth: a, GitHub: client, Log: slog.New(slog.DiscardHandler),
		UI: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "spa") })})

	return &env{h: h, store: store, gh: gh}
}

func (e *env) do(method, target, body string, headers map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	for _, c := range cookies {
		req.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)

	return rec
}

var csrf = []*http.Cookie{{Name: auth.CookieCSRF, Value: "tok"}}

func (e *env) post(target, body string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, target, body, map[string]string{auth.HeaderCSRF: "tok"}, csrf...)
}

func TestRerunPostsExactComment(t *testing.T) {
	for mode, want := range map[string]string{"retest": "/retest carbonio-proxy", "test": "/test carbonio-proxy"} {
		e := newEnv(t, auth.ModeNone, true, nil)

		rec := e.post("/api/v1/runs/ci/with-pr/rerun", `{"mode":"`+mode+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s: status %d: %s", mode, rec.Code, rec.Body)
		}

		var out map[string]string
		if err := json.Unmarshal(
			rec.Body.Bytes(),
			&out,
		); err != nil ||
			out["commentUrl"] != "https://github.com/zextras/carbonio-proxy/pull/323#issuecomment-1" {
			t.Fatalf("%s: body %s", mode, rec.Body)
		}

		if len(e.gh.comments) != 1 {
			t.Fatalf("%s: %d comments", mode, len(e.gh.comments))
		}

		c := e.gh.comments[0]
		if c.body != want || c.path != "/api/v3/repos/zextras/carbonio-proxy/issues/323/comments" || c.auth != "Bearer dev-token" {
			t.Fatalf("%s: comment = %+v", mode, c)
		}
	}
}

func TestRerunRejections(t *testing.T) {
	tests := []struct {
		name   string
		target string
		body   string
		want   int
	}{
		{"run without PR", "/api/v1/runs/ci/without-pr/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"run without pipeline name", "/api/v1/runs/ci/no-name/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"pipeline name that is not a name", "/api/v1/runs/ci/bad-name/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"org is ..", "/api/v1/runs/ci/dotdot-org/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"repository is .", "/api/v1/runs/ci/dot-repo/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"leading dot repository", "/api/v1/runs/ci/hidden-repo/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"pipeline name ..", "/api/v1/runs/ci/dotdot-name/rerun", `{"mode":"retest"}`, http.StatusBadRequest},
		{"unknown run", "/api/v1/runs/ci/nope/rerun", `{"mode":"retest"}`, http.StatusNotFound},
		{"bad mode", "/api/v1/runs/ci/with-pr/rerun", `{"mode":"approve"}`, http.StatusBadRequest},
		{"no mode", "/api/v1/runs/ci/with-pr/rerun", `{}`, http.StatusBadRequest},
		{"not json", "/api/v1/runs/ci/with-pr/rerun", `nope`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		e := newEnv(t, auth.ModeNone, true, nil)

		if rec := e.post(tt.target, tt.body); rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (%s)", tt.name, rec.Code, tt.want, rec.Body)
		}

		if len(e.gh.comments) != 0 {
			t.Errorf("%s: a comment was posted", tt.name)
		}
	}
}

func TestRerunAuthorization(t *testing.T) {
	body := `{"mode":"retest"}`

	// None mode without a dev token: reruns are not allowed.
	e := newEnv(t, auth.ModeNone, false, nil)
	if rec := e.post("/api/v1/runs/ci/with-pr/rerun", body); rec.Code != http.StatusForbidden {
		t.Errorf("none mode = %d", rec.Code)
	}

	// CSRF header is required, even in none mode.
	e = newEnv(t, auth.ModeNone, true, nil)
	if rec := e.do(http.MethodPost, "/api/v1/runs/ci/with-pr/rerun", body, nil, csrf...); rec.Code != http.StatusForbidden {
		t.Errorf("without csrf header = %d", rec.Code)
	}

	if rec := e.do(
		http.MethodPost,
		"/api/v1/runs/ci/with-pr/rerun",
		body,
		map[string]string{auth.HeaderCSRF: "other"},
		csrf...); rec.Code != http.StatusForbidden {
		t.Errorf("wrong csrf header = %d", rec.Code)
	}

	// Anonymous in github mode: 401 before anything else.
	e = newEnv(t, auth.ModeGitHub, false, nil)
	if rec := e.post("/api/v1/runs/ci/with-pr/rerun", body); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous github = %d", rec.Code)
	}

	if rec := e.do(http.MethodGet, "/api/v1/runs", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous GET github = %d", rec.Code)
	}

	// /me stays public.
	if rec := e.do(
		http.MethodGet,
		"/api/v1/me",
		"",
		nil,
	); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"authMode":"github"`) {
		t.Errorf("me = %d %s", rec.Code, rec.Body)
	}
}

func TestRerunGitHubFailures(t *testing.T) {
	for upstream, want := range map[int]int{
		http.StatusUnauthorized:        http.StatusUnauthorized,
		http.StatusForbidden:           http.StatusForbidden,
		http.StatusNotFound:            http.StatusForbidden,
		http.StatusUnprocessableEntity: http.StatusBadGateway,
		http.StatusInternalServerError: http.StatusBadGateway,
	} {
		e := newEnv(t, auth.ModeNone, true, nil)
		e.gh.status = upstream

		rec := e.post("/api/v1/runs/ci/with-pr/rerun", `{"mode":"test"}`)
		if rec.Code != want || strings.Contains(rec.Body.String(), "nope") {
			t.Errorf("upstream %d: status %d, want %d (%s)", upstream, rec.Code, want, rec.Body)
		}
	}
}

func TestRunDetail(t *testing.T) {
	e := newEnv(t, auth.ModeNone, true, nil)

	rec := e.do(http.MethodGet, "/api/v1/runs/ci/with-pr", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}

	var d model.RunDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}

	if !d.CanRerun || d.Summary.Name != "with-pr" || len(d.Nodes) != 2 || len(d.Edges) != 1 || d.Edges[0].Kind != "runAfter" {
		t.Fatalf("detail = %+v", d)
	}

	if !strings.Contains(rec.Body.String(), `"params":[]`) {
		t.Fatalf("params must be an empty list: %s", rec.Body)
	}

	// A run without a PR cannot be rerun.
	var d2 model.RunDetail
	rec = e.do(http.MethodGet, "/api/v1/runs/ci/without-pr", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &d2)

	if d2.CanRerun {
		t.Fatal("run without PR must not be rerunnable")
	}

	// Without a dev token nobody may rerun anything.
	e = newEnv(t, auth.ModeNone, false, nil)
	_ = json.Unmarshal(e.do(http.MethodGet, "/api/v1/runs/ci/with-pr", "", nil).Body.Bytes(), &d2)

	if d2.CanRerun {
		t.Fatal("canRerun without capability")
	}

	if rec := e.do(
		http.MethodGet,
		"/api/v1/runs/ci/nope",
		"",
		nil,
	); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("missing run = %d %s", rec.Code, rec.Body)
	}
}

func TestListFilters(t *testing.T) {
	e := newEnv(t, auth.ModeNone, false, nil)

	if rec := e.do(
		http.MethodGet,
		"/api/v1/repositories/ci/r/runs?limit=7&pr=12&branch=main&status=Failed",
		"",
		nil,
	); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	pr := 12
	if f := e.store.last; f.Namespace != "ci" || f.Repository != "r" || f.Limit != 7 || f.PR == nil || *f.PR != pr || f.Branch != "main" ||
		f.Status != model.StatusFailed {
		t.Fatalf("filter = %+v", f)
	}

	e.do(http.MethodGet, "/api/v1/runs", "", nil)

	if e.store.last.Limit != 100 || e.store.last.Repository != "" {
		t.Fatalf("default filter = %+v", e.store.last)
	}

	for _, q := range []string{"limit=0", "limit=abc", "limit=100000", "pr=x", "pr=0", "status=Exploded"} {
		if rec := e.do(http.MethodGet, "/api/v1/runs?"+q, "", nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", q, rec.Code)
		}
	}
}

func TestReadinessAndRouting(t *testing.T) {
	e := newEnv(t, auth.ModeNone, false, nil)
	e.store.ready = false

	if rec := e.do(http.MethodGet, "/readyz", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz = %d", rec.Code)
	}

	if rec := e.do(http.MethodGet, "/healthz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}

	if rec := e.do(http.MethodGet, "/api/v1/repositories", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("repositories before sync = %d", rec.Code)
	}

	// A stream opened before sync would receive nothing (the store is silent until ready) and
	// the SPA would sit on its 503 forever; it must see the 503 itself and retry.
	if rec := e.do(http.MethodGet, "/api/v1/events", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("events before sync = %d", rec.Code)
	}

	e.store.ready = true

	if rec := e.do(http.MethodGet, "/readyz", "", nil); rec.Code != http.StatusOK {
		t.Errorf("readyz after sync = %d", rec.Code)
	}

	if rec := e.do(
		http.MethodGet,
		"/api/v1/repositories",
		"",
		nil,
	); rec.Code != http.StatusOK ||
		strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("repositories = %d %s", rec.Code, rec.Body)
	}

	if rec := e.do(
		http.MethodGet,
		"/api/v1/unknown",
		"",
		nil,
	); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("unknown api = %d %s", rec.Code, rec.Body)
	}

	if rec := e.do(http.MethodGet, "/runs/ci/x", "", nil); rec.Body.String() != "spa" {
		t.Errorf("spa fallback = %q", rec.Body)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t, auth.ModeNone, false, nil)

	for _, target := range []string{"/", "/api/v1/me", "/healthz"} {
		h := e.do(http.MethodGet, target, "", nil).Header()

		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") ||
			h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s headers = %v", target, h)
		}
	}

	if cc := e.do(http.MethodGet, "/api/v1/me", "", nil).Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("api cache-control = %q", cc)
	}
}

func TestLogs(t *testing.T) {
	e := newEnv(t, auth.ModeNone, false, fakeLogs{})
	// the fake store has no TaskRuns, so the task is unknown
	if rec := e.do(http.MethodGet, "/api/v1/runs/ci/with-pr/tasks/clone/logs", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("task without taskrun = %d", rec.Code)
	}

	if rec := e.do(http.MethodGet, "/api/v1/runs/ci/nope/tasks/clone/logs", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run = %d", rec.Code)
	}
}

type logStore struct{ *fakeStore }

func (l logStore) TaskRunsOf(plr *unstructured.Unstructured) []*unstructured.Unstructured {
	tr := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{
		"name": plr.GetName() + "-clone", "namespace": "ci", "labels": map[string]any{model.LabelPipelineTask: "clone"},
	}}}

	return []*unstructured.Unstructured{tr}
}

func TestLogsStreamAndGone(t *testing.T) {
	for name, tt := range map[string]struct {
		err      error
		status   int
		body     string
		wantType string
	}{
		"ok":       {nil, 200, "log of build", "text/plain; charset=utf-8"},
		"gone":     {logs.ErrGone, 404, `{"error":"logs no longer available"}`, "application/json"},
		"no step":  {logs.ErrNoStep, 404, `step not found`, "application/json"},
		"upstream": {errors.New("boom"), 502, `could not read logs`, "application/json"},
	} {
		e := newEnv(t, auth.ModeNone, false, fakeLogs{err: tt.err})
		h := New(Deps{Store: logStore{e.store}, Logs: fakeLogs{err: tt.err}, Auth: authNone(t), GitHub: nil,
			Log: slog.New(slog.DiscardHandler)})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/ci/with-pr/tasks/clone/logs?step=build&follow=1", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.body) || rec.Header().Get("Content-Type") != tt.wantType {
			t.Errorf("%s: %d %q (%s)", name, rec.Code, rec.Body, rec.Header().Get("Content-Type"))
		}
	}

	// An unknown taskRun is a 404 before streaming.
	e := newEnv(t, auth.ModeNone, false, fakeLogs{})
	h := New(Deps{Store: logStore{e.store}, Logs: fakeLogs{}, Auth: authNone(t), Log: slog.New(slog.DiscardHandler)})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/runs/ci/with-pr/tasks/clone/logs?taskRun=other", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown taskRun = %d", rec.Code)
	}
}

// floodLogs writes until the response write fails, then reports it.
type floodLogs struct {
	fakeLogs

	stopped chan error
}

func (f floodLogs) Stream(ctx context.Context, w io.Writer, _, _, _ string, _ bool) error {
	chunk := make([]byte, 64<<10)

	for ctx.Err() == nil {
		if _, err := w.Write(chunk); err != nil {
			f.stopped <- err

			return err
		}
	}

	f.stopped <- ctx.Err()

	return ctx.Err()
}

// A client that stops reading must not pin the log handler: the per-write deadline ends it. The
// SSE path shares the same deadline helper (sendEvent); filling the socket buffers with 40-byte
// events would take too long to test.
func TestLogsStalledClientDoesNotPinHandler(t *testing.T) {
	old := writeTimeout
	writeTimeout = 200 * time.Millisecond

	t.Cleanup(func() { writeTimeout = old })

	l := floodLogs{stopped: make(chan error, 1)}
	e := newEnv(t, auth.ModeNone, false, l)
	srv := httptest.NewServer(New(Deps{
		Store: logStore{e.store}, Logs: l, Auth: authNone(t), Log: slog.New(slog.DiscardHandler),
	}))
	t.Cleanup(srv.Close)

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	const request = "GET /api/v1/runs/ci/with-pr/tasks/clone/logs?step=build&follow=1 HTTP/1.1\r\nHost: x\r\n\r\n"

	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}

	// The client never reads. The handler must give up on its own.
	select {
	case err := <-l.stopped:
		if errors.Is(err, context.Canceled) {
			t.Fatalf("handler ended by cancellation, not by the write deadline: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler still blocked on a client that does not read")
	}
}

func authNone(t *testing.T) *auth.Service {
	t.Helper()

	a, err := auth.New(context.Background(), auth.Config{Mode: auth.ModeNone})
	if err != nil {
		t.Fatal(err)
	}

	return a
}

func TestEventsStream(t *testing.T) {
	e := newEnv(t, auth.ModeNone, false, nil)
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/events", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", resp.Header.Get("Content-Type"))
	}

	e.store.events <- kube.Event{Kind: "TaskRun", Namespace: "ci", Name: "r-clone", Repository: "r", Run: "r"}

	sc := bufio.NewScanner(resp.Body)

	var lines []string

	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) == 4 {
			break
		}
	}

	got := strings.Join(lines, "|")
	if want := `: connected||event: change|data: {"kind":"TaskRun","namespace":"ci","name":"r-clone","repository":"r","run":"r"}`; got != want {
		t.Fatalf("stream = %q, want %q", got, want)
	}
}
