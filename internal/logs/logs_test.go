// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package logs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakePods struct {
	mu   sync.Mutex
	logs map[string]string // container -> log
	// waiting lists containers that answer 400 "waiting to start" this many more times.
	waiting map[string]int
	gone    bool
	opened  []string
}

func (f *fakePods) Open(_ context.Context, _, pod, container string, o OpenOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.gone {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, pod)
	}

	if f.waiting[container] > 0 {
		f.waiting[container]--

		return nil, apierrors.NewBadRequest("container is waiting to start: PodInitializing")
	}

	f.opened = append(f.opened, container)

	text := f.logs[container]

	if o.TailLines > 0 {
		lines := strings.SplitAfter(text, "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		text = strings.Join(lines[max(len(lines)-int(o.TailLines), 0):], "")
	}

	if o.LimitBytes > 0 && int64(len(text)) > o.LimitBytes {
		text = text[:o.LimitBytes]
	}

	return io.NopCloser(strings.NewReader(text)), nil
}

type fakeTaskRuns struct {
	mu sync.Mutex
	tr *unstructured.Unstructured
}

func (f *fakeTaskRuns) TaskRun(_, _ string) (*unstructured.Unstructured, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.tr, f.tr != nil
}

func taskRun(podName, cond string) *unstructured.Unstructured {
	status := map[string]any{
		"steps": []any{
			map[string]any{"name": "clone", "container": "step-clone"},
			map[string]any{"name": "build", "container": "step-build"},
		},
		"conditions": []any{map[string]any{"type": "Succeeded", "status": cond, "reason": "x"}},
	}
	if podName != "" {
		status["podName"] = podName
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "tr", "namespace": "ns"},
		"status":   status,
	}}
}

func TestStreamAllStepsAndSingleStep(t *testing.T) {
	pods := &fakePods{logs: map[string]string{"step-clone": "cloning\n", "step-build": "building"}}
	s := &Streamer{Pods: pods, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "True")}}

	var all bytes.Buffer
	if err := s.Stream(context.Background(), &all, "ns", "tr", "", false); err != nil {
		t.Fatal(err)
	}

	if want := "==> step: clone <==\ncloning\n==> step: build <==\nbuilding"; all.String() != want {
		t.Fatalf("all = %q, want %q", all.String(), want)
	}

	var one bytes.Buffer
	if err := s.Stream(context.Background(), &one, "ns", "tr", "build", false); err != nil || one.String() != "building" {
		t.Fatalf("single = %q, err = %v", one.String(), err)
	}

	if err := s.Stream(context.Background(), io.Discard, "ns", "tr", "nope", false); !errors.Is(err, ErrNoStep) {
		t.Fatalf("err = %v, want ErrNoStep", err)
	}
}

func TestStreamPodGone(t *testing.T) {
	for _, name := range []string{"pod deleted", "no pod name"} {
		t.Run(name, func(t *testing.T) {
			pod := "pod"
			pods := &fakePods{gone: true}

			if name == "no pod name" {
				pod, pods = "", &fakePods{}
			}

			s := &Streamer{Pods: pods, TaskRuns: &fakeTaskRuns{tr: taskRun(pod, "True")}}

			var out bytes.Buffer

			err := s.Stream(context.Background(), &out, "ns", "tr", "", true)
			if !errors.Is(err, ErrGone) {
				t.Fatalf("err = %v, want ErrGone", err)
			}

			// A 404 can only be sent while nothing was written; the step separator must not leak.
			if out.Len() != 0 {
				t.Fatalf("wrote %q before reporting the pod gone", out.String())
			}
		})
	}
}

func TestFollowWaitsForWaitingStep(t *testing.T) {
	pods := &fakePods{
		logs:    map[string]string{"step-clone": "a\n", "step-build": "b\n"},
		waiting: map[string]int{"step-build": 2},
	}
	s := &Streamer{Pods: pods, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "Unknown")}, Poll: time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out bytes.Buffer
	if err := s.Stream(ctx, &out, "ns", "tr", "", true); err != nil {
		t.Fatal(err)
	}

	if want := "==> step: clone <==\na\n==> step: build <==\nb\n"; out.String() != want {
		t.Fatalf("out = %q, want %q", out.String(), want)
	}
}

func TestFollowWaitsForPod(t *testing.T) {
	trs := &fakeTaskRuns{tr: taskRun("", "Unknown")}
	s := &Streamer{Pods: &fakePods{logs: map[string]string{"step-clone": "a"}}, TaskRuns: trs, Poll: time.Millisecond}

	go func() {
		time.Sleep(20 * time.Millisecond)
		trs.mu.Lock()
		trs.tr = taskRun("pod", "Unknown")
		trs.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out bytes.Buffer
	if err := s.Stream(ctx, &out, "ns", "tr", "clone", true); err != nil || out.String() != "a" {
		t.Fatalf("out = %q, err = %v", out.String(), err)
	}
}

func TestNotFollowingSkipsStepsThatDidNotStart(t *testing.T) {
	pods := &fakePods{logs: map[string]string{"step-clone": "a\n"}, waiting: map[string]int{"step-build": 1}}
	s := &Streamer{Pods: pods, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "Unknown")}}

	var out bytes.Buffer
	if err := s.Stream(context.Background(), &out, "ns", "tr", "", false); err != nil {
		t.Fatal(err)
	}

	if want := "==> step: clone <==\na\n==> step: build <==\n(step did not start)\n"; out.String() != want {
		t.Fatalf("out = %q, want %q", out.String(), want)
	}
}

func TestTail(t *testing.T) {
	long := strings.Repeat("x", 99) + "\n"
	big := strings.Repeat(long, tailLines+50) // many more than tailLines lines, 200 KB

	pods := &fakePods{logs: map[string]string{"step-clone": "a\nb\n", "step-build": big}}
	s := &Streamer{Pods: pods, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "True")}}
	ctx := context.Background()

	small, err := s.Tail(ctx, "ns", "tr", "clone", 1024)
	if err != nil || small.Text != "a\nb\n" || !small.FromStart {
		t.Fatalf("small = %+v, err = %v", small, err)
	}

	whole, err := s.Tail(ctx, "ns", "tr", "build", len(big))
	if err != nil || whole.Text != big || !whole.FromStart {
		t.Fatalf("whole: len %d fromStart %v err %v", len(whole.Text), whole.FromStart, err)
	}

	cut, err := s.Tail(ctx, "ns", "tr", "build", 1000)
	if err != nil || cut.FromStart || len(cut.Text) > 1000 || !strings.HasSuffix(cut.Text, long) || !strings.HasPrefix(cut.Text, "xx") {
		t.Fatalf("cut: len %d fromStart %v err %v", len(cut.Text), cut.FromStart, err)
	}

	if len(cut.Text)%100 != 0 {
		t.Fatalf("tail does not start at a line boundary: len %d", len(cut.Text))
	}

	if _, err := s.Tail(ctx, "ns", "tr", "nope", 10); !errors.Is(err, ErrNoStep) {
		t.Fatalf("err = %v, want ErrNoStep", err)
	}
}

func TestTailGone(t *testing.T) {
	s := &Streamer{Pods: &fakePods{gone: true}, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "True")}}
	if _, err := s.Tail(context.Background(), "ns", "tr", "clone", 10); !errors.Is(err, ErrGone) {
		t.Fatalf("err = %v, want ErrGone", err)
	}

	s = &Streamer{Pods: &fakePods{}, TaskRuns: &fakeTaskRuns{tr: taskRun("", "True")}}
	if _, err := s.Tail(context.Background(), "ns", "tr", "clone", 10); !errors.Is(err, ErrGone) {
		t.Fatalf("no pod name: err = %v, want ErrGone", err)
	}
}

func TestTailStepNotStarted(t *testing.T) {
	s := &Streamer{Pods: &fakePods{waiting: map[string]int{"step-clone": 1}}, TaskRuns: &fakeTaskRuns{tr: taskRun("pod", "False")}}

	ex, err := s.Tail(context.Background(), "ns", "tr", "clone", 10)
	if err != nil || ex.Text != "" || !ex.FromStart {
		t.Fatalf("ex = %+v, err = %v", ex, err)
	}
}
