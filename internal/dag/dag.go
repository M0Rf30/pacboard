// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package dag builds the task graph of a PipelineRun: nodes from the resolved pipeline
// spec joined with the run's TaskRuns, edges from runAfter, result references and
// finally, with the transitive duplicates removed.
package dag

import (
	"regexp"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/model"
)

// Graph is the drawable task graph of a run.
type Graph struct {
	Nodes []model.GraphNode
	Edges []model.GraphEdge
}

// task is the part of a pipeline task the graph needs.
type task struct {
	name    string
	finally bool
	spec    map[string]any
}

// Build joins the pipeline spec (status.pipelineSpec or the referenced Pipeline) with the
// run's TaskRuns. A pipeline task without TaskRun is Skipped (with the reason Tekton gave)
// or, once the run is over, "Skipped / not run"; while the run is going it is Pending.
func Build(spec map[string]any, plr *unstructured.Unstructured, taskRuns []*unstructured.Unstructured) Graph {
	tasks := tasksOf(spec)
	byTask := GroupTaskRuns(plr, taskRuns)
	skipped := map[string]string{}

	for _, s := range model.Slice(plr.Object, "status", "skippedTasks") {
		skipped[model.Str(s, "name")] = model.Str(s, "reason")
	}

	over := model.StatusOf(plr.Object).Terminal()
	nodes := make([]model.GraphNode, 0, len(tasks))

	for _, t := range tasks {
		nodes = append(nodes, buildNode(t, byTask[t.name], skipped, over))
	}

	return Graph{Nodes: nodes, Edges: edgesOf(tasks)}
}

func tasksOf(spec map[string]any) []task {
	var out []task

	for _, t := range model.Slice(spec, "tasks") {
		out = append(out, task{name: model.Str(t, "name"), spec: t})
	}

	for _, t := range model.Slice(spec, "finally") {
		out = append(out, task{name: model.Str(t, "name"), finally: true, spec: t})
	}

	return out
}

// GroupTaskRuns maps each pipeline task name to its TaskRuns (several with a matrix), in
// the order of status.childReferences, then by name. The pipeline task comes from the
// TaskRun's tekton.dev/pipelineTask label, else from childReferences.
func GroupTaskRuns(plr *unstructured.Unstructured, taskRuns []*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	order := map[string]int{}
	taskOf := map[string]string{}

	for i, c := range model.Slice(plr.Object, "status", "childReferences") {
		if model.Str(c, "kind") != "TaskRun" {
			continue
		}

		order[model.Str(c, "name")] = i
		taskOf[model.Str(c, "name")] = model.Str(c, "pipelineTaskName")
	}

	sorted := slices.Clone(taskRuns)
	slices.SortStableFunc(sorted, func(a, b *unstructured.Unstructured) int {
		ia, oka := order[a.GetName()]
		ib, okb := order[b.GetName()]

		switch {
		case oka && okb && ia != ib:
			return ia - ib
		case oka != okb:
			if oka {
				return -1
			}

			return 1
		}

		return compareStrings(a.GetName(), b.GetName())
	})

	out := map[string][]*unstructured.Unstructured{}

	for _, tr := range sorted {
		name := tr.GetLabels()[model.LabelPipelineTask]
		if name == "" {
			name = taskOf[tr.GetName()]
		}

		if name != "" {
			out[name] = append(out[name], tr)
		}
	}

	return out
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func buildNode(t task, trs []*unstructured.Unstructured, skipped map[string]string, over bool) model.GraphNode {
	n := model.GraphNode{
		ID:           t.name,
		Finally:      t.finally,
		MatrixCount:  1,
		TaskRunNames: []string{},
		Steps:        []model.Step{},
	}

	if len(trs) == 0 {
		switch reason, wasSkipped := skipped[t.name]; {
		case wasSkipped:
			n.Status, n.Reason = model.NodeSkipped, reason
		case over:
			n.Status, n.Reason = model.NodeSkipped, "Not run"
		default:
			n.Status = model.StatusPending
		}

		return n
	}

	n.TaskRunName = trs[0].GetName()
	n.MatrixCount = len(trs)
	n.Steps = model.StepsOf(trs[0])

	for _, tr := range trs {
		n.TaskRunNames = append(n.TaskRunNames, tr.GetName())
	}

	n.Status, n.Reason = aggregate(trs)
	n.StartTime, n.CompletionTime = span(trs)

	return n
}

// aggregate folds the TaskRuns of a matrix into one status: any failure wins, then
// cancellation, then work in progress; Succeeded only when every TaskRun succeeded.
func aggregate(trs []*unstructured.Unstructured) (model.RunStatus, string) {
	rank := map[model.RunStatus]int{
		model.StatusFailed:    5,
		model.StatusCancelled: 4,
		model.StatusRunning:   3,
		model.StatusPending:   2,
		model.StatusUnknown:   1,
		model.StatusSucceeded: 0,
	}

	best := model.StatusOf(trs[0].Object)

	for _, tr := range trs[1:] {
		if info := model.StatusOf(tr.Object); rank[info.Status] > rank[best.Status] {
			best = info
		}
	}

	return best.Status, best.Reason
}

// span is the earliest start and, once every TaskRun finished, the latest completion.
func span(trs []*unstructured.Unstructured) (*time.Time, *time.Time) {
	var start, end *time.Time

	done := true

	for _, tr := range trs {
		if s := model.Time(tr.Object, "status", "startTime"); s != nil && (start == nil || s.Before(*start)) {
			start = s
		}

		e := model.Time(tr.Object, "status", "completionTime")
		if e == nil {
			done = false

			continue
		}

		if end == nil || e.After(*end) {
			end = e
		}
	}

	if !done {
		end = nil
	}

	return start, end
}

var (
	// $(tasks.<name>.results.<r>), $(tasks.<name>.status) and $(tasks.<name>.matrix.…)
	// reference another task and so order the two; names are DNS labels.
	dotRef = regexp.MustCompile(`\$\(tasks\.([a-z0-9][a-z0-9-]*)\.(?:results|status|matrix)\b`)
	// The same with the bracket notation: $(tasks['name'].results['r']).
	bracketRef = regexp.MustCompile(`\$\(tasks\[['"]([^'"\]]+)['"]\]\.(?:results|status|matrix)\b`)
)

// resultRefs lists the tasks referenced by params, when, matrix and workspace subPaths.
func resultRefs(spec map[string]any) []string {
	var refs []string

	collect := func(s string) {
		for _, re := range []*regexp.Regexp{dotRef, bracketRef} {
			for _, m := range re.FindAllStringSubmatch(s, -1) {
				refs = append(refs, m[1])
			}
		}
	}

	for _, key := range []string{"params", "when", "matrix"} {
		walkStrings(spec[key], collect)
	}

	for _, w := range model.Slice(spec, "workspaces") {
		if s, ok := w["subPath"].(string); ok {
			collect(s)
		}
	}

	return refs
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case []any:
		for _, item := range t {
			walkStrings(item, fn)
		}
	case map[string]any:
		for _, item := range t {
			walkStrings(item, fn)
		}
	}
}

type pair struct{ from, to string }

// edgesOf derives the edges of the graph. One edge per pair of tasks, the strongest kind
// winning (runAfter, then result, then finally), then transitive reduction.
func edgesOf(tasks []task) []model.GraphEdge {
	known := map[string]bool{}
	for _, t := range tasks {
		known[t.name] = true
	}

	kinds := map[pair]string{}
	add := func(from, to, kind string) {
		if from == to || !known[from] || !known[to] {
			return
		}

		p := pair{from, to}
		if cur, ok := kinds[p]; !ok || strength(kind) > strength(cur) {
			kinds[p] = kind
		}
	}

	hasOutgoing := map[string]bool{}

	for _, t := range tasks {
		for _, after := range stringList(t.spec["runAfter"]) {
			add(after, t.name, model.EdgeRunAfter)
		}

		for _, ref := range resultRefs(t.spec) {
			add(ref, t.name, model.EdgeResult)
		}
	}

	finallyTask := map[string]bool{}
	for _, t := range tasks {
		finallyTask[t.name] = t.finally
	}

	for p := range kinds {
		if !finallyTask[p.to] {
			hasOutgoing[p.from] = true
		}
	}

	for _, leaf := range tasks {
		if leaf.finally || hasOutgoing[leaf.name] {
			continue
		}

		for _, fin := range tasks {
			if fin.finally {
				add(leaf.name, fin.name, model.EdgeFinally)
			}
		}
	}

	reduce(kinds)

	return ordered(tasks, kinds)
}

func strength(kind string) int {
	switch kind {
	case model.EdgeRunAfter:
		return 3
	case model.EdgeResult:
		return 2
	default:
		return 1
	}
}

func stringList(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))

	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

// reduce removes every edge A→C for which another path A→…→C exists.
func reduce(kinds map[pair]string) {
	next := map[string][]string{}
	for p := range kinds {
		next[p.from] = append(next[p.from], p.to)
	}

	for p := range kinds {
		if reachable(next, p.from, p.to, p) {
			delete(kinds, p)
		}
	}
}

// reachable reports whether to is reachable from the source without using the edge skip.
// The graph is a DAG (Tekton rejects cycles), but visited guards against bad input.
func reachable(next map[string][]string, from, to string, skip pair) bool {
	visited := map[string]bool{from: true}
	stack := []string{from}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for _, n := range next[cur] {
			if (pair{cur, n}) == skip {
				continue
			}

			if n == to {
				return true
			}

			if !visited[n] {
				visited[n] = true
				stack = append(stack, n)
			}
		}
	}

	return false
}

// ordered lists the edges by target task then by source task, in spec order.
func ordered(tasks []task, kinds map[pair]string) []model.GraphEdge {
	out := make([]model.GraphEdge, 0, len(kinds))

	for _, to := range tasks {
		for _, from := range tasks {
			if kind, ok := kinds[pair{from.name, to.name}]; ok {
				out = append(out, model.GraphEdge{From: from.name, To: to.name, Kind: kind})
			}
		}
	}

	return out
}
