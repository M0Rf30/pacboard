// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package kube

import (
	"context"
	"log/slog"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/M0Rf30/pacboard/internal/model"
)

func obj(
	apiVersion, kind, ns, name string,
	labels map[string]string,
	status map[string]any,
	spec map[string]any,
) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": ns},
	}}
	u.SetLabels(labels)

	if status != nil {
		u.Object["status"] = status
	}

	if spec != nil {
		u.Object["spec"] = spec
	}

	return u
}

func plr(ns, name, repo, pr, start, cond string) *unstructured.Unstructured {
	return obj("tekton.dev/v1", "PipelineRun", ns, name,
		map[string]string{model.LabelRepository: repo, "pipelinesascode.tekton.dev/pull-request": pr},
		map[string]any{
			"startTime":  start,
			"conditions": []any{map[string]any{"type": "Succeeded", "status": cond, "reason": "r"}},
		}, nil)
}

func newTestStore(t *testing.T, objs ...runtime.Object) (*Store, *dynamicfake.FakeDynamicClient) {
	t.Helper()

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		RepositoryGVR:  "RepositoryList",
		PipelineRunGVR: "PipelineRunList",
		TaskRunGVR:     "TaskRunList",
		PipelineGVR:    "PipelineList",
	}, objs...)

	s, err := NewStore(client, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.Start(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for !s.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("store did not sync")
		}

		time.Sleep(5 * time.Millisecond)
	}

	return s, client
}

func TestStoreIndexesAndFilters(t *testing.T) {
	repoA := obj("pipelinesascode.tekton.dev/v1alpha1", "Repository", "ci", "a", nil, nil,
		map[string]any{"url": "https://github.com/o/a", "concurrency_limit": int64(2)})
	repoB := obj("pipelinesascode.tekton.dev/v1alpha1", "Repository", "ci", "b", nil, nil, map[string]any{"url": "https://github.com/o/b"})
	repoC := obj(
		"pipelinesascode.tekton.dev/v1alpha1",
		"Repository",
		"ci",
		"idle",
		nil,
		nil,
		map[string]any{"url": "https://github.com/o/c"},
	)

	run1 := plr("ci", "a-1", "a", "7", "2026-01-01T10:00:00Z", "True")
	run2 := plr("ci", "a-2", "a", "8", "2026-01-01T11:00:00Z", "False")
	run3 := plr("ci", "b-1", "b", "", "2026-01-01T12:00:00Z", "Unknown")
	tr := obj("tekton.dev/v1", "TaskRun", "ci", "a-1-build",
		map[string]string{model.LabelPipelineRun: "a-1", model.LabelPipelineTask: "build"}, nil, nil)

	s, _ := newTestStore(t, repoA, repoB, repoC, run1, run2, run3, tr)

	repos := s.Repositories()
	if len(repos) != 3 || repos[0].Name != "b" || repos[1].Name != "a" || repos[2].Name != "idle" {
		t.Fatalf("repositories order = %+v", repos)
	}

	if repos[1].RunCount != 2 || repos[1].LastRun == nil || repos[1].LastRun.Name != "a-2" ||
		repos[1].ConcurrencyLimit == nil || *repos[1].ConcurrencyLimit != 2 || repos[1].URL != "https://github.com/o/a" {
		t.Fatalf("repo a = %+v", repos[1])
	}

	if repos[2].LastRun != nil || repos[2].RunCount != 0 {
		t.Fatalf("idle repo = %+v", repos[2])
	}

	runs := s.Runs(RunFilter{Namespace: "ci", Repository: "a"})
	if len(runs) != 2 || runs[0].Name != "a-2" {
		t.Fatalf("runs of a = %+v", runs)
	}

	seven := 7
	if got := s.Runs(RunFilter{Namespace: "ci", Repository: "a", PR: &seven}); len(got) != 1 || got[0].Name != "a-1" {
		t.Fatalf("pr filter = %+v", got)
	}

	if got := s.Runs(RunFilter{Status: model.StatusRunning}); len(got) != 1 || got[0].Name != "b-1" {
		t.Fatalf("status filter = %+v", got)
	}

	if got := s.Runs(RunFilter{Limit: 2}); len(got) != 2 || got[0].Name != "b-1" || got[1].Name != "a-2" {
		t.Fatalf("limit = %+v", got)
	}

	got, ok := s.Run("ci", "a-1")
	if !ok {
		t.Fatal("run a-1 missing")
	}

	if trs := s.TaskRunsOf(got); len(trs) != 1 || trs[0].GetName() != "a-1-build" {
		t.Fatalf("taskruns = %+v", trs)
	}

	if _, ok := s.Run("ci", "nope"); ok {
		t.Fatal("unexpected run")
	}
}

func TestStorePublishesChanges(t *testing.T) {
	s, client := newTestStore(t)

	events, cancel := s.Subscribe()
	defer cancel()

	run := plr("ci", "a-1", "a", "7", "2026-01-01T10:00:00Z", "Unknown")
	if _, err := client.Resource(PipelineRunGVR).Namespace("ci").Create(context.Background(), run, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-events:
		want := Event{Kind: "PipelineRun", Namespace: "ci", Name: "a-1", Repository: "a", Run: "a-1"}
		if e != want {
			t.Fatalf("event = %+v, want %+v", e, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
}

func TestPipelineSpecFallsBackToPipelineRef(t *testing.T) {
	pipeline := obj("tekton.dev/v1", "Pipeline", "ci", "p", nil, nil, map[string]any{"tasks": []any{map[string]any{"name": "a"}}})
	s, _ := newTestStore(t, pipeline)

	withRef := obj("tekton.dev/v1", "PipelineRun", "ci", "r", nil, nil, map[string]any{"pipelineRef": map[string]any{"name": "p"}})

	spec, err := s.PipelineSpec(context.Background(), withRef)
	if err != nil || len(spec["tasks"].([]any)) != 1 {
		t.Fatalf("spec = %v, err = %v", spec, err)
	}

	inline := obj("tekton.dev/v1", "PipelineRun", "ci", "r2", nil, map[string]any{"pipelineSpec": map[string]any{"tasks": []any{}}}, nil)
	if _, err := s.PipelineSpec(context.Background(), inline); err != nil {
		t.Fatalf("inline status spec: %v", err)
	}

	if _, err := s.PipelineSpec(context.Background(), obj("tekton.dev/v1", "PipelineRun", "ci", "r3", nil, nil, nil)); err == nil {
		t.Fatal("expected ErrNoPipelineSpec")
	}
}
