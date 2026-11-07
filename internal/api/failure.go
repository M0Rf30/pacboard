// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/internal/model"
)

const (
	// failureLogBytes bounds how much of a step log is read to find the error message.
	failureLogBytes = 2 << 20
	failureTimeout  = 10 * time.Second
	failureCacheMax = 256
)

// failureErrorTTL is how long the fallback result of a transient log read error is remembered, so
// a slow or unavailable log API does not cost every refetch of a failed run its full timeout.
const failureErrorTTL = 10 * time.Second

type failureEntry struct {
	f       *model.Failure
	expires time.Time // zero: final result, never expires
}

// failureFlight is one computation in progress; waiters block on done and then read f.
type failureFlight struct {
	done chan struct{}
	f    *model.Failure
}

// failureCache keeps computed failures per run name and resourceVersion, so the refetches an SSE
// event triggers do not read the logs again. Oldest entries are evicted first. Concurrent misses
// for one key share a single computation.
type failureCache struct {
	mu       sync.Mutex
	m        map[string]failureEntry
	order    []string
	inflight map[string]*failureFlight
}

func (c *failureCache) getLocked(key string, now time.Time) (*model.Failure, bool) {
	e, ok := c.m[key]
	if !ok || (!e.expires.IsZero() && !now.Before(e.expires)) {
		return nil, false
	}

	return e.f, true
}

func (c *failureCache) putLocked(key string, f *model.Failure, expires time.Time) {
	if c.m == nil {
		c.m = map[string]failureEntry{}
	}

	if _, ok := c.m[key]; !ok {
		if len(c.order) >= failureCacheMax {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}

		c.order = append(c.order, key)
	}

	c.m[key] = failureEntry{f: f, expires: expires}
}

func (c *failureCache) put(key string, f *model.Failure, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.putLocked(key, f, expires)
}

// acquire returns the cached result, or the flight to wait on, or (leader true) a new flight the
// caller must complete with finish.
func (c *failureCache) acquire(key string, now time.Time) (*model.Failure, *failureFlight, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.getLocked(key, now); ok {
		return hit, nil, false
	}

	if running, ok := c.inflight[key]; ok {
		return nil, running, false
	}

	if c.inflight == nil {
		c.inflight = map[string]*failureFlight{}
	}

	fl := &failureFlight{done: make(chan struct{})}
	c.inflight[key] = fl

	return nil, fl, true
}

// finish stores the result (expires zero = final), removes the flight and wakes its waiters. A nil
// f (the computation panicked) is not stored.
func (c *failureCache) finish(key string, fl *failureFlight, f *model.Failure, expires time.Time) {
	c.mu.Lock()

	if f != nil {
		c.putLocked(key, f, expires)
	}

	delete(c.inflight, key)
	c.mu.Unlock()

	fl.f = f

	close(fl.done)
}

// failure explains a Failed run; nil for any other status. It never fails the request: when the
// log cannot be read the run's condition message stands in.
func (h *handler) failure(
	ctx context.Context, plr *unstructured.Unstructured, summary model.RunSummary,
	nodes []model.GraphNode, taskRuns []*unstructured.Unstructured,
) *model.Failure {
	if summary.Status != model.StatusFailed {
		return nil
	}

	key := plr.GetNamespace() + "/" + plr.GetName() + "@" + plr.GetResourceVersion()

	cached, fl, leader := h.failures.acquire(key, h.Now())
	if cached != nil {
		return cached
	}

	if !leader {
		select {
		case <-fl.done:
			return fl.f
		case <-ctx.Done():
			return nil
		}
	}

	var (
		f       *model.Failure
		expires time.Time
	)

	// finish also runs when the build panics, so the waiters are never stranded.
	defer func() { h.failures.finish(key, fl, f, expires) }()

	// The result is shared with the waiters, so one client giving up must not abort it.
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureTimeout)
	defer cancel()

	f, cacheable := h.buildFailure(bctx, summary, nodes, taskRuns)
	if !cacheable {
		expires = h.Now().Add(failureErrorTTL)
	}

	return f
}

// buildFailure reports whether the result is final: a transient log read error is not cached.
func (h *handler) buildFailure(
	ctx context.Context, summary model.RunSummary, nodes []model.GraphNode, taskRuns []*unstructured.Unstructured,
) (*model.Failure, bool) {
	f := &model.Failure{Message: model.CapMessage(summary.Message), SkippedBecause: model.SkippedAfterFailure(nodes)}

	node := model.FirstFailedNode(nodes)
	if node == nil {
		return f, true
	}

	f.Task = node.ID
	f.TaskRun = failedTaskRun(node, taskRuns)

	var steps []model.Step

	for _, tr := range taskRuns {
		if tr.GetName() == f.TaskRun {
			steps = model.StepsOf(tr)
		}
	}

	step, ok := model.FailedStep(steps)
	if !ok {
		return f, true
	}

	f.Step, f.ExitCode = step.Name, step.ExitCode

	ex, err := h.Logs.Tail(ctx, summary.Namespace, f.TaskRun, step.Name, failureLogBytes)
	if err != nil {
		if !errors.Is(err, logs.ErrGone) {
			h.Log.Warn("failure log unavailable", "run", summary.Name, "taskRun", f.TaskRun, "step", step.Name, "err", err)
		}

		return f, errors.Is(err, logs.ErrGone) || errors.Is(err, logs.ErrNoStep)
	}

	if msg, line, found := model.ErrorFromLog(ex.Text, ex.FromStart); found {
		f.Message, f.Line = msg, line
	}

	return f, true
}

// failedTaskRun picks the first Failed TaskRun of a (matrix) node, else the node's first.
func failedTaskRun(node *model.GraphNode, taskRuns []*unstructured.Unstructured) string {
	for _, name := range node.TaskRunNames {
		for _, tr := range taskRuns {
			if tr.GetName() == name && model.StatusOf(tr.Object).Status == model.StatusFailed {
				return name
			}
		}
	}

	return node.TaskRunName
}
