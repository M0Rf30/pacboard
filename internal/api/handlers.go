// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/auth"
	"github.com/M0Rf30/pacboard/internal/dag"
	"github.com/M0Rf30/pacboard/internal/github"
	"github.com/M0Rf30/pacboard/internal/httpx"
	"github.com/M0Rf30/pacboard/internal/kube"
	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/internal/model"
)

const (
	defaultRepoRuns = 50
	defaultAllRuns  = 100
	maxLimit        = 500
	maxBodyBytes    = 4 << 10
)

// safeName limits what is put into a GitHub URL path or a PR comment: org, repository and
// pipeline names are DNS-ish, never whitespace or slashes, and never "." / ".." / a leading dot
// (a path segment that a client or proxy may normalise away).
var safeName = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, h.Auth.Me(r))
}

func (h *handler) repositories(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, h.Store.Repositories())
}

// filter reads limit, pr, branch and status; it writes the 400 itself and reports false on bad input.
func filter(w http.ResponseWriter, r *http.Request, defaultLimit int) (kube.RunFilter, bool) {
	q := r.URL.Query()
	f := kube.RunFilter{Limit: defaultLimit, Branch: q.Get("branch"), Status: model.RunStatus(q.Get("status"))}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("limit must be between 1 and %d", maxLimit))

			return f, false
		}

		f.Limit = n
	}

	if v := q.Get("pr"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.Error(w, http.StatusBadRequest, "pr must be a pull request number")

			return f, false
		}

		f.PR = &n
	}

	if f.Status != "" && !f.Status.Valid() {
		httpx.Error(w, http.StatusBadRequest, "unknown status")

		return f, false
	}

	return f, true
}

func (h *handler) repositoryRuns(w http.ResponseWriter, r *http.Request) {
	f, ok := filter(w, r, defaultRepoRuns)
	if !ok {
		return
	}

	f.Namespace, f.Repository = r.PathValue("namespace"), r.PathValue("name")
	httpx.JSON(w, http.StatusOK, h.Store.Runs(f))
}

func (h *handler) runs(w http.ResponseWriter, r *http.Request) {
	f, ok := filter(w, r, defaultAllRuns)
	if !ok {
		return
	}

	httpx.JSON(w, http.StatusOK, h.Store.Runs(f))
}

func (h *handler) run(w http.ResponseWriter, r *http.Request) {
	plr, ok := h.Store.Run(r.PathValue("namespace"), r.PathValue("name"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "run not found")

		return
	}

	taskRuns := h.Store.TaskRunsOf(plr)

	spec, err := h.Store.PipelineSpec(r.Context(), plr)
	if err != nil {
		// Still show what ran, just without the declared order.
		h.Log.Warn("pipeline spec unavailable, listing task runs only", "run", plr.GetName(), "err", err)
		spec = specFromTaskRuns(dag.GroupTaskRuns(plr, taskRuns), plr)
	}

	graph := dag.Build(spec, plr, taskRuns)
	summary := model.RunSummaryOf(plr, h.Now())

	var siblings []model.RunSummary
	if summary.Repository != "" {
		siblings = h.Store.Runs(kube.RunFilter{Namespace: summary.Namespace, Repository: summary.Repository})
	}

	httpx.JSON(w, http.StatusOK, model.RunDetail{
		Summary:                summary,
		Nodes:                  graph.Nodes,
		Edges:                  graph.Edges,
		Params:                 model.Params(plr.Object),
		CanRerun:               h.Auth.Me(r).CanRerun && canRerun(summary),
		TypicalDurationSeconds: model.TypicalDuration(siblings),
		Failure:                h.failure(r.Context(), plr, summary, graph.Nodes, taskRuns),
	})
}

func canRerun(s model.RunSummary) bool {
	return s.Git.PRNumber != nil && s.PRName != ""
}

// specFromTaskRuns lists the pipeline tasks that have TaskRuns, in childReferences order.
func specFromTaskRuns(groups map[string][]*unstructured.Unstructured, plr *unstructured.Unstructured) map[string]any {
	tasks := []any{}
	seen := map[string]bool{}

	for _, c := range model.Slice(plr.Object, "status", "childReferences") {
		name := model.Str(c, "pipelineTaskName")
		if name != "" && groups[name] != nil && !seen[name] {
			seen[name] = true
			tasks = append(tasks, map[string]any{"name": name})
		}
	}

	return map[string]any{"tasks": tasks}
}

func (h *handler) logs(w http.ResponseWriter, r *http.Request) {
	ns, name, task := r.PathValue("namespace"), r.PathValue("name"), r.PathValue("task")

	plr, ok := h.Store.Run(ns, name)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "run not found")

		return
	}

	trs := dag.GroupTaskRuns(plr, h.Store.TaskRunsOf(plr))[task]
	if len(trs) == 0 {
		httpx.Error(w, http.StatusNotFound, "task has no task run")

		return
	}

	q := r.URL.Query()
	taskRun := trs[0].GetName()

	if v := q.Get("taskRun"); v != "" {
		taskRun = ""

		for _, tr := range trs {
			if tr.GetName() == v {
				taskRun = v
			}
		}

		if taskRun == "" {
			httpx.Error(w, http.StatusNotFound, "task run not found")

			return
		}
	}

	follow := q.Get("follow") == "1" || q.Get("follow") == "true"
	out := &lazyWriter{w: w, rc: http.NewResponseController(w)}

	err := h.Logs.Stream(r.Context(), out, ns, taskRun, q.Get("step"), follow)

	switch {
	case err == nil || r.Context().Err() != nil:
	case out.started:
		// The status line is gone; the stream just ends.
		h.Log.Warn("log stream interrupted", "taskRun", taskRun, "err", err)
	case errors.Is(err, logs.ErrGone):
		httpx.Error(w, http.StatusNotFound, "logs no longer available")
	case errors.Is(err, logs.ErrNoStep):
		httpx.Error(w, http.StatusNotFound, "step not found")
	default:
		h.Log.Warn("log stream failed", "taskRun", taskRun, "err", err)
		httpx.Error(w, http.StatusBadGateway, "could not read logs")
	}
}

// lazyWriter sends the 200 and flushes with the first byte, so an error before any output
// can still become a JSON 404.
type lazyWriter struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	started bool
}

func (l *lazyWriter) Write(p []byte) (int, error) {
	if !l.started {
		l.started = true
		l.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		l.w.Header().Set("X-Accel-Buffering", "no")
		l.w.WriteHeader(http.StatusOK)
	}

	defer setWriteDeadline(l.rc)()

	n, err := l.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}

	// A flush error means the client went away or stalled; the next write reports it.
	_ = l.rc.Flush()

	return n, nil
}

// writeTimeout bounds one write+flush to a streaming client. The server has no WriteTimeout (the
// streams are long-lived), so a client that stops reading would otherwise pin its handler.
var writeTimeout = 30 * time.Second

// setWriteDeadline arms the per-write deadline and returns the function that clears it. Writers
// that do not support deadlines (recorders in tests) are left alone.
func setWriteDeadline(rc *http.ResponseController) func() {
	_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))

	return func() { _ = rc.SetWriteDeadline(time.Time{}) }
}

// sendEvent writes one SSE message and flushes it under the write deadline.
func sendEvent(w http.ResponseWriter, rc *http.ResponseController, msg string) error {
	defer setWriteDeadline(rc)()

	if _, err := io.WriteString(w, msg); err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	if err := rc.Flush(); err != nil {
		return fmt.Errorf("flush event: %w", err)
	}

	return nil
}

func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if sendEvent(w, rc, ": connected\n\n") != nil {
		return
	}

	events, cancel := h.Store.Subscribe()
	defer cancel()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()

	// The stream outlives the request's authentication: it ends with the session, and each
	// heartbeat re-checks membership (cached, so GitHub is not asked more than any request would).
	var expired <-chan time.Time

	if remaining, ok := h.Auth.SessionRemaining(r); ok {
		t := time.NewTimer(remaining)
		defer t.Stop()

		expired = t.C
	}

	for {
		var msg string

		select {
		case <-r.Context().Done():
			return
		case <-expired:
			return
		case <-tick.C:
			if !h.Auth.Revalidate(r) {
				return
			}

			msg = ": heartbeat\n\n"
		case e := <-events:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}

			msg = "event: change\ndata: " + string(data) + "\n\n"
		}

		if sendEvent(w, rc, msg) != nil {
			return
		}
	}
}

type rerunRequest struct {
	Mode string `json:"mode"`
}

func (h *handler) rerun(w http.ResponseWriter, r *http.Request) {
	actor, err := h.Auth.Actor(r)

	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		httpx.Error(w, http.StatusUnauthorized, "authentication required")

		return
	case err != nil:
		httpx.Error(w, http.StatusForbidden, "reruns need a logged-in GitHub user")

		return
	}

	var req rerunRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).
		Decode(&req); err != nil ||
		(req.Mode != "retest" && req.Mode != "test") {
		httpx.Error(w, http.StatusBadRequest, `mode must be "retest" or "test"`)

		return
	}

	plr, ok := h.Store.Run(r.PathValue("namespace"), r.PathValue("name"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "run not found")

		return
	}

	pr := model.PRNumber(plr)
	org, repo := model.RepoRef(plr)
	prName := model.Meta(plr, "original-prname")

	switch {
	case pr == nil:
		httpx.Error(w, http.StatusBadRequest, "run has no pull request to comment on")

		return
	case !safeName.MatchString(org) || !safeName.MatchString(repo):
		httpx.Error(w, http.StatusBadRequest, "run has no usable GitHub repository")

		return
	case !safeName.MatchString(prName):
		httpx.Error(w, http.StatusBadRequest, "run has no usable pipeline name")

		return
	}

	url, err := h.GitHub.PostComment(r.Context(), actor.Token, org, repo, *pr, "/"+req.Mode+" "+prName)
	if err != nil {
		h.rerunFailed(w, actor.Login, plr.GetName(), err)

		return
	}

	h.Log.Info("rerun requested", "login", actor.Login, "run", plr.GetNamespace()+"/"+plr.GetName(), "mode", req.Mode, "comment", url)
	httpx.JSON(w, http.StatusCreated, map[string]string{"commentUrl": url})
}

func (h *handler) rerunFailed(w http.ResponseWriter, login, run string, err error) {
	h.Log.Warn("rerun comment failed", "login", login, "run", run, "err", err)

	var apiErr *github.APIError
	if !errors.As(err, &apiErr) {
		httpx.Error(w, http.StatusBadGateway, "could not reach GitHub")

		return
	}

	switch apiErr.Status {
	case http.StatusUnauthorized:
		httpx.Error(w, http.StatusUnauthorized, "GitHub rejected your login, sign in again")
	case http.StatusForbidden, http.StatusNotFound:
		httpx.Error(w, http.StatusForbidden, "GitHub does not let you comment on this pull request")
	default:
		httpx.Error(w, http.StatusBadGateway, "GitHub refused the comment")
	}
}
