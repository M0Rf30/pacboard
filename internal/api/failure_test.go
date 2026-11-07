// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/internal/model"
)

type tailLogs struct {
	fakeLogs

	ex    logs.Excerpt
	calls int
	step  string
}

func (l *tailLogs) Tail(_ context.Context, _, _, step string, _ int) (logs.Excerpt, error) {
	l.calls++
	l.step = step

	return l.ex, l.err
}

func makeTaskRun(name string, exitCodes ...int) *unstructured.Unstructured {
	steps := []any{}
	status, reason := "True", "Succeeded"

	for i, c := range exitCodes {
		if c != 0 {
			status, reason = "False", "Failed"
		}

		steps = append(steps, map[string]any{
			"name": "s" + strconv.Itoa(i), "terminated": map[string]any{"exitCode": int64(c)},
		})
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": name, "namespace": "ci"},
		"status": map[string]any{
			"steps":      steps,
			"conditions": []any{map[string]any{"type": "Succeeded", "status": status, "reason": reason}},
		},
	}}
}

func failureFixture() (model.RunSummary, []model.GraphNode, []*unstructured.Unstructured) {
	early, late := time.Unix(100, 0), time.Unix(200, 0)
	sum := model.RunSummary{Namespace: "ci", Name: "run", Status: model.StatusFailed, Message: "Tasks Completed: 1 (Failed: 1)"}
	nodes := []model.GraphNode{
		{ID: "lint", TaskRunName: "run-lint", Status: model.StatusFailed, CompletionTime: &late, TaskRunNames: []string{"run-lint"}},
		{
			ID: "build", TaskRunName: "run-build-0", Status: model.StatusFailed, CompletionTime: &early,
			TaskRunNames: []string{"run-build-0", "run-build-1"},
		},
		{ID: "deploy", Status: model.NodeSkipped, Reason: "Parent Tasks were skipped"},
		{ID: "docs", Status: model.NodeSkipped, Reason: model.WhenSkipReason},
	}
	trs := []*unstructured.Unstructured{
		makeTaskRun("run-lint", 2),
		makeTaskRun("run-build-0", 0),
		makeTaskRun("run-build-1", 0, 3),
	}

	return sum, nodes, trs
}

func newFailureHandler(l LogStreamer) *handler {
	deps := Deps{Logs: l, Log: slog.New(slog.DiscardHandler), Now: time.Now}

	return &handler{Deps: deps}
}

func runFailure(h *handler) (*model.Failure, *unstructured.Unstructured) {
	sum, nodes, trs := failureFixture()
	plr := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{
		"name": "run", "namespace": "ci", "resourceVersion": "7",
	}}}

	return h.failure(context.Background(), plr, sum, nodes, trs), plr
}

func TestFailureExtraction(t *testing.T) {
	cond := "Tasks Completed: 1 (Failed: 1)"

	for name, tt := range map[string]struct {
		ex       logs.Excerpt
		err      error
		wantMsg  string
		wantLine *int
	}{
		"plain annotation":  {ex: logs.Excerpt{Text: "ok\n::error::boom\nafter\n", FromStart: true}, wantMsg: "boom", wantLine: new(2)},
		"annotation params": {ex: logs.Excerpt{Text: "::error file=x.go,line=3::bad thing\nlast\n", FromStart: true}, wantMsg: "bad thing", wantLine: new(1)},
		"ansi":              {ex: logs.Excerpt{Text: "\x1b[31m::error::\x1b[1mred\x1b[0m\n", FromStart: true}, wantMsg: "red", wantLine: new(1)},
		"last line":         {ex: logs.Excerpt{Text: "a\nmake: *** failed\n\n", FromStart: true}, wantMsg: "make: *** failed", wantLine: new(2)},
		"tail only":         {ex: logs.Excerpt{Text: "x\n::error::late\n"}, wantMsg: "late"},
		"empty log":         {ex: logs.Excerpt{FromStart: true}, wantMsg: cond},
		"pod gone":          {err: logs.ErrGone, wantMsg: cond},
		"read error":        {err: errors.New("boom"), wantMsg: cond},
	} {
		t.Run(name, func(t *testing.T) {
			l := &tailLogs{ex: tt.ex}
			l.err = tt.err

			f, _ := runFailure(newFailureHandler(l))

			if f == nil {
				t.Fatal("nil failure")
			}

			if f.Message != tt.wantMsg || (f.Line == nil) != (tt.wantLine == nil) || (f.Line != nil && *f.Line != *tt.wantLine) {
				t.Fatalf("message %q line %v, want %q %v", f.Message, f.Line, tt.wantMsg, tt.wantLine)
			}

			// Earliest failed node, its first failed matrix TaskRun, first non-zero step.
			if f.Task != "build" || f.TaskRun != "run-build-1" || f.Step != "s1" || f.ExitCode == nil || *f.ExitCode != 3 {
				t.Fatalf("failure = %+v", f)
			}

			if f.SkippedBecause != 1 || l.step != "s1" {
				t.Fatalf("skippedBecause %d, step read %q", f.SkippedBecause, l.step)
			}
		})
	}
}

func TestFailureCache(t *testing.T) {
	l := &tailLogs{ex: logs.Excerpt{Text: "::error::x\n", FromStart: true}}
	h := newFailureHandler(l)

	first, _ := runFailure(h)
	second, _ := runFailure(h)

	if l.calls != 1 || first != second {
		t.Fatalf("calls = %d, same = %v; the second request must come from the cache", l.calls, first == second)
	}

	// A transient read error is remembered only briefly.
	now := time.Unix(1000, 0)
	l = &tailLogs{}
	l.err = errors.New("boom")
	h = newFailureHandler(l)
	h.Now = func() time.Time { return now }

	runFailure(h)
	runFailure(h)

	if l.calls != 1 {
		t.Fatalf("calls = %d, want 1 (errors are negative-cached)", l.calls)
	}

	now = now.Add(failureErrorTTL + time.Second)

	runFailure(h)

	if l.calls != 2 {
		t.Fatalf("calls = %d, want 2 (the negative entry expired)", l.calls)
	}

	// A good result replaces the negative entry for good.
	l.err = nil
	now = now.Add(failureErrorTTL + time.Second)

	runFailure(h)
	now = now.Add(24 * time.Hour)
	runFailure(h)

	if l.calls != 3 {
		t.Fatalf("calls = %d, want 3 (final results do not expire)", l.calls)
	}

	// A gone pod is final.
	l = &tailLogs{}
	l.err = logs.ErrGone
	h = newFailureHandler(l)
	runFailure(h)
	runFailure(h)

	if l.calls != 1 {
		t.Fatalf("calls = %d, want 1", l.calls)
	}
}

// gateLogs blocks every Tail until release is closed and counts the calls.
type gateLogs struct {
	fakeLogs

	release chan struct{}
	calls   atomic.Int32
}

func (g *gateLogs) Tail(context.Context, string, string, string, int) (logs.Excerpt, error) {
	g.calls.Add(1)
	<-g.release

	return logs.Excerpt{Text: "::error::shared\n", FromStart: true}, nil
}

func TestFailureConcurrentMissesShareOneRead(t *testing.T) {
	g := &gateLogs{release: make(chan struct{})}
	h := newFailureHandler(g)

	const waiters = 8

	results := make(chan *model.Failure, waiters)

	for range waiters {
		go func() {
			f, _ := runFailure(h)
			results <- f
		}()
	}

	// Wait until the leader is inside Tail and every other request is parked on its flight.
	deadline := time.Now().Add(5 * time.Second)

	for g.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	close(g.release)

	var first *model.Failure

	for range waiters {
		f := <-results
		if f == nil || f.Message != "shared" {
			t.Fatalf("failure = %+v", f)
		}

		if first == nil {
			first = f
		}
	}

	if n := g.calls.Load(); n != 1 {
		t.Fatalf("Tail called %d times, want 1", n)
	}
}

func TestFailureCacheBounded(t *testing.T) {
	var c failureCache

	for i := range failureCacheMax + 10 {
		c.put(strconv.Itoa(i), &model.Failure{}, time.Time{})
	}

	if len(c.m) != failureCacheMax || len(c.order) != failureCacheMax {
		t.Fatalf("size = %d", len(c.m))
	}

	if _, ok := c.getLocked("0", time.Time{}); ok {
		t.Fatal("oldest entry must be evicted")
	}

	if _, ok := c.getLocked(strconv.Itoa(failureCacheMax+9), time.Time{}); !ok {
		t.Fatal("newest entry missing")
	}
}

func TestFailureNotFailedAndNoTask(t *testing.T) {
	l := &tailLogs{}
	h := newFailureHandler(l)
	plr := &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "r", "namespace": "ci"}}}

	if f := h.failure(context.Background(), plr, model.RunSummary{Status: model.StatusSucceeded}, nil, nil); f != nil {
		t.Fatalf("succeeded run has failure %+v", f)
	}

	long := strings.Repeat("é", 800)

	f := h.failure(context.Background(), plr, model.RunSummary{Status: model.StatusFailed, Message: long}, nil, nil)
	if f == nil || f.Task != "" || f.Step != "" || f.Line != nil || len([]rune(f.Message)) != model.MaxFailureMessage || l.calls != 0 {
		t.Fatalf("failure = %+v, calls %d", f, l.calls)
	}
}
