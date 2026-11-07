// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package kube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/M0Rf30/pacboard/internal/model"
)

// Resources read from the cluster.
var (
	RepositoryGVR  = schema.GroupVersionResource{Group: "pipelinesascode.tekton.dev", Version: "v1alpha1", Resource: "repositories"}
	PipelineRunGVR = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	TaskRunGVR     = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}
	PipelineGVR    = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelines"}
)

// Index names. Both key on "<namespace>/<value>".
const (
	byRepository  = "byRepository"
	byPipelineRun = "byPipelineRun"
)

// ErrNoPipelineSpec means neither the run nor its Pipeline reference carries a spec.
var ErrNoPipelineSpec = errors.New("pipeline spec not available")

// RunFilter selects PipelineRuns; zero fields match everything.
type RunFilter struct {
	Namespace  string
	Repository string
	PR         *int
	Branch     string
	Status     model.RunStatus
	Limit      int
}

// Store serves Repository, PipelineRun and TaskRun objects from informer caches.
type Store struct {
	dyn    dynamic.Interface
	log    *slog.Logger
	broker *Broker
	now    func() time.Time

	factories []interface{ Start(<-chan struct{}) }
	repos     []cache.SharedIndexInformer
	runs      []cache.SharedIndexInformer
	taskRuns  []cache.SharedIndexInformer
	ready     atomic.Bool
}

// NewStore prepares the informers for the namespaces (all when empty); call Start to run them.
func NewStore(dyn dynamic.Interface, namespaces []string, log *slog.Logger) (*Store, error) {
	s := &Store{dyn: dyn, log: log, broker: NewBroker(), now: time.Now}

	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	for _, ns := range namespaces {
		factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dyn, 0, ns, nil)
		s.factories = append(s.factories, factory)

		repo := factory.ForResource(RepositoryGVR).Informer()
		run := factory.ForResource(PipelineRunGVR).Informer()
		task := factory.ForResource(TaskRunGVR).Informer()

		if err := run.AddIndexers(cache.Indexers{byRepository: labelIndex(model.LabelRepository)}); err != nil {
			return nil, fmt.Errorf("index pipelineruns: %w", err)
		}

		if err := task.AddIndexers(cache.Indexers{byPipelineRun: labelIndex(model.LabelPipelineRun)}); err != nil {
			return nil, fmt.Errorf("index taskruns: %w", err)
		}

		for kind, inf := range map[string]cache.SharedIndexInformer{"Repository": repo, "PipelineRun": run, "TaskRun": task} {
			if err := s.watch(kind, inf); err != nil {
				return nil, err
			}
		}

		s.repos = append(s.repos, repo)
		s.runs = append(s.runs, run)
		s.taskRuns = append(s.taskRuns, task)
	}

	return s, nil
}

// labelIndex indexes objects by "<namespace>/<label value>".
func labelIndex(label string) cache.IndexFunc {
	return func(obj any) ([]string, error) {
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return nil, nil
		}

		v := u.GetLabels()[label]
		if v == "" {
			return nil, nil
		}

		return []string{u.GetNamespace() + "/" + v}, nil
	}
}

func (s *Store) watch(kind string, inf cache.SharedIndexInformer) error {
	// Drop managedFields: large and never read.
	if err := inf.SetTransform(func(obj any) (any, error) {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			u.SetManagedFields(nil)
		}

		return obj, nil
	}); err != nil {
		return fmt.Errorf("transform %s: %w", kind, err)
	}

	if err := inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
		s.log.Warn("watch failed, retrying", "kind", kind, "err", err)
	}); err != nil {
		return fmt.Errorf("watch error handler %s: %w", kind, err)
	}

	_, err := inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) { s.notify(kind, obj) },
		UpdateFunc: func(oldObj, newObj any) {
			o, _ := oldObj.(*unstructured.Unstructured)
			n, _ := newObj.(*unstructured.Unstructured)

			if o != nil && n != nil && o.GetResourceVersion() == n.GetResourceVersion() {
				return
			}

			s.notify(kind, newObj)
		},
		DeleteFunc: func(obj any) {
			if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tomb.Obj
			}

			s.notify(kind, obj)
		},
	})
	if err != nil {
		return fmt.Errorf("event handler %s: %w", kind, err)
	}

	return nil
}

func (s *Store) notify(kind string, obj any) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || !s.ready.Load() {
		return
	}

	e := Event{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}

	switch kind {
	case "PipelineRun":
		e.Repository = u.GetLabels()[model.LabelRepository]
		e.Run = u.GetName()
	case "TaskRun":
		e.Repository = u.GetLabels()[model.LabelRepository]
		e.Run = u.GetLabels()[model.LabelPipelineRun]
	default:
		e.Repository = u.GetName()
	}

	s.broker.Publish(e)
}

// Start runs the informers until ctx ends and returns at once; Ready turns true when they synced.
func (s *Store) Start(ctx context.Context) {
	for _, f := range s.factories {
		f.Start(ctx.Done())
	}

	var synced []cache.InformerSynced

	for _, group := range [][]cache.SharedIndexInformer{s.repos, s.runs, s.taskRuns} {
		for _, inf := range group {
			synced = append(synced, inf.HasSynced)
		}
	}

	go func() {
		if cache.WaitForCacheSync(ctx.Done(), synced...) {
			s.ready.Store(true)
			s.log.Info("informers synced")
		}
	}()
}

// Ready reports whether every informer completed its initial list.
func (s *Store) Ready() bool { return s.ready.Load() }

// Subscribe returns a channel of change events and the function that ends the subscription.
func (s *Store) Subscribe() (<-chan Event, func()) { return s.broker.Subscribe() }

func get(infs []cache.SharedIndexInformer, ns, name string) *unstructured.Unstructured {
	for _, inf := range infs {
		obj, exists, err := inf.GetIndexer().GetByKey(ns + "/" + name)
		if err != nil || !exists {
			continue
		}

		if u, ok := obj.(*unstructured.Unstructured); ok {
			return u
		}
	}

	return nil
}

func byIndex(infs []cache.SharedIndexInformer, index, key string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured

	for _, inf := range infs {
		objs, err := inf.GetIndexer().ByIndex(index, key)
		if err != nil {
			continue
		}

		for _, obj := range objs {
			if u, ok := obj.(*unstructured.Unstructured); ok {
				out = append(out, u)
			}
		}
	}

	return out
}

// Run returns a PipelineRun; callers must not modify it.
func (s *Store) Run(ns, name string) (*unstructured.Unstructured, bool) {
	u := get(s.runs, ns, name)

	return u, u != nil
}

// TaskRun returns a TaskRun; callers must not modify it.
func (s *Store) TaskRun(ns, name string) (*unstructured.Unstructured, bool) {
	u := get(s.taskRuns, ns, name)

	return u, u != nil
}

// TaskRunsOf lists the TaskRuns of a PipelineRun.
func (s *Store) TaskRunsOf(plr *unstructured.Unstructured) []*unstructured.Unstructured {
	return byIndex(s.taskRuns, byPipelineRun, plr.GetNamespace()+"/"+plr.GetName())
}

// Runs lists PipelineRun summaries newest first.
func (s *Store) Runs(f RunFilter) []model.RunSummary {
	var objs []*unstructured.Unstructured

	if f.Repository != "" {
		objs = byIndex(s.runs, byRepository, f.Namespace+"/"+f.Repository)
	} else {
		for _, inf := range s.runs {
			for _, o := range inf.GetStore().List() {
				if u, ok := o.(*unstructured.Unstructured); ok && (f.Namespace == "" || u.GetNamespace() == f.Namespace) {
					objs = append(objs, u)
				}
			}
		}
	}

	sortNewestFirst(objs)

	now := s.now()
	out := make([]model.RunSummary, 0, min(len(objs), max(f.Limit, 0)))

	for _, u := range objs {
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}

		r := model.RunSummaryOf(u, now)
		if matches(r, f) {
			out = append(out, r)
		}
	}

	return out
}

func matches(r model.RunSummary, f RunFilter) bool {
	if f.PR != nil && (r.Git.PRNumber == nil || *r.Git.PRNumber != *f.PR) {
		return false
	}

	if f.Branch != "" {
		b := strings.TrimPrefix(f.Branch, "refs/heads/")
		if r.Git.Branch != b && r.Git.SourceBranch != b {
			return false
		}
	}

	return f.Status == "" || r.Status == f.Status
}

func sortNewestFirst(objs []*unstructured.Unstructured) {
	slices.SortStableFunc(objs, func(a, b *unstructured.Unstructured) int {
		if c := model.SortTime(b).Compare(model.SortTime(a)); c != 0 {
			return c
		}

		return strings.Compare(a.GetName(), b.GetName())
	})
}

// Repositories lists the PaC Repositories with their latest run, newest activity first.
func (s *Store) Repositories() []model.RepositorySummary {
	now := s.now()
	type entry struct {
		repo model.RepositorySummary
		at   time.Time
	}

	var entries []entry

	for _, inf := range s.repos {
		for _, o := range inf.GetStore().List() {
			u, ok := o.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			e := entry{repo: model.RepositoryOf(u)}
			runs := byIndex(s.runs, byRepository, u.GetNamespace()+"/"+u.GetName())
			e.repo.RunCount = len(runs)

			if len(runs) > 0 {
				sortNewestFirst(runs)

				last := model.RunSummaryOf(runs[0], now)
				e.repo.LastRun = &last
				e.at = model.SortTime(runs[0])
			}

			entries = append(entries, e)
		}
	}

	slices.SortStableFunc(entries, func(a, b entry) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}

		return strings.Compare(a.repo.Namespace+"/"+a.repo.Name, b.repo.Namespace+"/"+b.repo.Name)
	})

	out := make([]model.RepositorySummary, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.repo)
	}

	return out
}

// PipelineSpec returns the resolved spec of a run: status.pipelineSpec, else spec.pipelineSpec,
// else the Pipeline named by spec.pipelineRef.
func (s *Store) PipelineSpec(ctx context.Context, plr *unstructured.Unstructured) (map[string]any, error) {
	for _, path := range [][]string{{"status", "pipelineSpec"}, {"spec", "pipelineSpec"}} {
		if v, _, _ := unstructured.NestedFieldNoCopy(plr.Object, path...); v != nil {
			if m, ok := v.(map[string]any); ok && len(m) > 0 {
				return m, nil
			}
		}
	}

	name := model.Str(plr.Object, "spec", "pipelineRef", "name")
	if name == "" {
		return nil, ErrNoPipelineSpec
	}

	p, err := s.dyn.Resource(PipelineGVR).Namespace(plr.GetNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pipeline %s: %w", name, err)
	}

	v, _, _ := unstructured.NestedFieldNoCopy(p.Object, "spec")
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		return m, nil
	}

	return nil, ErrNoPipelineSpec
}
