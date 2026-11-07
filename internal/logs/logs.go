// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package logs streams the step container logs of a TaskRun's pod.
package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/M0Rf30/pacboard/internal/model"
)

var (
	// ErrGone means the pod (or the TaskRun's pod name) no longer exists.
	ErrGone = errors.New("logs no longer available")
	// ErrNoStep means the TaskRun has no step of that name.
	ErrNoStep = errors.New("no such step")
)

const defaultPoll = time.Second

// OpenOptions select what part of a container log to read.
type OpenOptions struct {
	Follow bool
	// LimitBytes stops the stream after this many bytes from the start (0 = no limit).
	LimitBytes int64
	// TailLines starts at that many lines from the end (0 = from the beginning).
	TailLines int64
}

// PodLogs opens the log of one container.
type PodLogs interface {
	Open(ctx context.Context, namespace, pod, container string, o OpenOptions) (io.ReadCloser, error)
}

// TaskRuns looks a TaskRun up; the informer store implements it.
type TaskRuns interface {
	TaskRun(namespace, name string) (*unstructured.Unstructured, bool)
}

// Kubernetes reads container logs from the API server.
type Kubernetes struct{ Client kubernetes.Interface }

// Open implements PodLogs.
func (k Kubernetes) Open(ctx context.Context, namespace, pod, container string, o OpenOptions) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{Container: container, Follow: o.Follow}
	if o.LimitBytes > 0 {
		opts.LimitBytes = &o.LimitBytes
	}

	if o.TailLines > 0 {
		opts.TailLines = &o.TailLines
	}

	rc, err := k.Client.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open log of %s/%s: %w", pod, container, err)
	}

	return rc, nil
}

// Streamer copies step logs of a TaskRun to a writer.
type Streamer struct {
	Pods     PodLogs
	TaskRuns TaskRuns
	// Poll is how often a followed stream re-checks a step or pod that is not up yet (default 1s).
	Poll time.Duration
}

type step struct{ name, container string }

// stepsOf lists the step containers: from status.steps once started, else from the task spec.
func stepsOf(tr *unstructured.Unstructured) []step {
	var out []step

	for _, s := range model.Slice(tr.Object, "status", "steps") {
		name := model.Str(s, "name")

		c := model.Str(s, "container")
		if c == "" {
			c = "step-" + name
		}

		out = append(out, step{name, c})
	}

	if len(out) > 0 {
		return out
	}

	for _, path := range [][]string{{"status", "taskSpec", "steps"}, {"spec", "taskSpec", "steps"}} {
		for _, s := range model.Slice(tr.Object, path...) {
			name := model.Str(s, "name")
			out = append(out, step{name, "step-" + name})
		}

		if len(out) > 0 {
			break
		}
	}

	return out
}

// tracker remembers whether anything was written and how the output ended.
type tracker struct {
	w       io.Writer
	written bool
	last    byte
}

func (t *tracker) Write(p []byte) (int, error) {
	if len(p) > 0 {
		t.written = true
		t.last = p[len(p)-1]
	}

	return t.w.Write(p) //nolint:wrapcheck // plain passthrough
}

// Stream writes the logs of one step (stepName) or of every step with
// "==> step: <name> <==" separators (stepName empty). With follow it waits for the pod and
// for steps that have not started, and returns when the last step ended. ErrGone is returned
// when the pod is gone; nothing has been written to w in that case unless a step was already copied.
func (s *Streamer) Stream(ctx context.Context, w io.Writer, namespace, taskRun, stepName string, follow bool) error {
	tr, ok := s.TaskRuns.TaskRun(namespace, taskRun)
	if !ok {
		return ErrGone
	}

	steps := stepsOf(tr)

	if stepName != "" {
		var found []step

		for _, st := range steps {
			if st.name == stepName {
				found = append(found, st)
			}
		}

		if len(found) == 0 {
			return fmt.Errorf("%w: %q", ErrNoStep, stepName)
		}

		steps = found
	}

	pod, err := s.podName(ctx, namespace, taskRun, follow)
	if err != nil {
		return err
	}

	out := &tracker{w: w}

	for _, st := range steps {
		// The separator goes out once the step's log is open, so a vanished pod still
		// leaves nothing written and the caller can answer 404.
		begin := func() {
			if stepName != "" {
				return
			}

			if out.written && out.last != '\n' {
				_, _ = io.WriteString(out, "\n")
			}

			_, _ = fmt.Fprintf(out, "==> step: %s <==\n", st.name)
		}

		if err := s.copyStep(ctx, out, begin, namespace, taskRun, pod, st, follow); err != nil {
			return err
		}
	}

	return nil
}

func (s *Streamer) poll() time.Duration {
	if s.Poll > 0 {
		return s.Poll
	}

	return defaultPoll
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// podName returns the pod of the TaskRun; while following it waits for a pending TaskRun to get one.
func (s *Streamer) podName(ctx context.Context, namespace, taskRun string, follow bool) (string, error) {
	for {
		tr, ok := s.TaskRuns.TaskRun(namespace, taskRun)
		if !ok {
			return "", ErrGone
		}

		if pod := model.Str(tr.Object, "status", "podName"); pod != "" {
			return pod, nil
		}

		if !follow || model.StatusOf(tr.Object).Terminal() {
			return "", ErrGone
		}

		if err := sleep(ctx, s.poll()); err != nil {
			return "", err
		}
	}
}

func (s *Streamer) copyStep(
	ctx context.Context, out *tracker, begin func(), namespace, taskRun, pod string, st step, follow bool,
) error {
	for {
		rc, err := s.Pods.Open(ctx, namespace, pod, st.container, OpenOptions{Follow: follow})

		switch {
		case err == nil:
			begin()

			err = copyFlushed(ctx, out, rc)
			_ = rc.Close()

			return err
		case apierrors.IsNotFound(err):
			if out.written {
				_, _ = io.WriteString(out, "\n[logs no longer available]\n")

				return nil
			}

			return ErrGone
		case apierrors.IsBadRequest(err):
			// The container is waiting to start (or was never created).
			if !follow || s.finished(namespace, taskRun) {
				begin()

				_, _ = io.WriteString(out, "(step did not start)\n")

				return nil
			}

			if err := sleep(ctx, s.poll()); err != nil {
				return err
			}
		default:
			return err
		}
	}
}

func (s *Streamer) finished(namespace, taskRun string) bool {
	tr, ok := s.TaskRuns.TaskRun(namespace, taskRun)

	return !ok || model.StatusOf(tr.Object).Terminal()
}

func copyFlushed(ctx context.Context, w io.Writer, r io.Reader) error {
	buf := make([]byte, 32*1024)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write log: %w", werr)
			}
		}

		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			if ctx.Err() != nil {
				return fmt.Errorf("read log: %w", ctx.Err())
			}

			return fmt.Errorf("read log: %w", err)
		}
	}
}

// tailLines bounds the second read of a log larger than the excerpt limit.
const tailLines = 2000

// Excerpt is the part of a step log Tail read.
type Excerpt struct {
	Text string
	// FromStart is true when Text begins at the first line of the log.
	FromStart bool
}

// Tail reads at most limit bytes of one step's log without following: the whole log when it is
// that small, else its last lines (cut at a line boundary). ErrGone means the pod is gone.
// A step whose container never started yields an empty excerpt.
func (s *Streamer) Tail(ctx context.Context, namespace, taskRun, stepName string, limit int) (Excerpt, error) {
	tr, ok := s.TaskRuns.TaskRun(namespace, taskRun)
	if !ok {
		return Excerpt{}, ErrGone
	}

	var container string

	for _, st := range stepsOf(tr) {
		if st.name == stepName {
			container = st.container
		}
	}

	pod := model.Str(tr.Object, "status", "podName")

	switch {
	case container == "":
		return Excerpt{}, fmt.Errorf("%w: %q", ErrNoStep, stepName)
	case pod == "":
		return Excerpt{}, ErrGone
	}

	head, err := s.readAll(ctx, namespace, pod, container, OpenOptions{LimitBytes: int64(limit) + 1}, limit+1)
	if err != nil {
		return Excerpt{}, err
	}

	if len(head) <= limit {
		return Excerpt{Text: string(head), FromStart: true}, nil
	}

	tail, err := s.readAll(ctx, namespace, pod, container, OpenOptions{TailLines: tailLines}, limit)
	if err != nil {
		return Excerpt{}, err
	}

	// The tail starts mid-line when it was cut by size; drop the partial first line.
	if len(tail) == limit {
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
	}

	return Excerpt{Text: string(tail)}, nil
}

// readAll returns the last keep bytes of a container log; a container that has not started reads as empty.
func (s *Streamer) readAll(ctx context.Context, namespace, pod, container string, o OpenOptions, keep int) ([]byte, error) {
	rc, err := s.Pods.Open(ctx, namespace, pod, container, o)

	switch {
	case apierrors.IsNotFound(err):
		return nil, ErrGone
	case apierrors.IsBadRequest(err):
		return nil, nil
	case err != nil:
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	var (
		data []byte
		buf  = make([]byte, 32*1024)
	)

	for {
		n, err := rc.Read(buf)
		data = append(data, buf[:n]...)

		if len(data) > 2*keep {
			data = append(data[:0], data[len(data)-keep:]...)
		}

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read log: %w", err)
		}
	}

	return data[max(len(data)-keep, 0):], nil
}
