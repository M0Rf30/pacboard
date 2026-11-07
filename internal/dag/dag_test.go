// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dag

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/M0Rf30/pacboard/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func parse(t *testing.T, s string) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}

	return m
}

func edgeStrings(g Graph) []string {
	out := []string{}
	for _, e := range g.Edges {
		out = append(out, e.From+">"+e.To+":"+e.Kind)
	}

	return out
}

func emptyRun() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "r", "namespace": "ns"},
		"status":   map[string]any{},
	}}
}

func TestEdges(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want []string
	}{
		{
			name: "runAfter",
			spec: `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"]}]}`,
			want: []string{"a>b:runAfter"},
		},
		{
			// The case the OpenShift console misses: no runAfter, only a result reference.
			name: "result reference only",
			spec: `{"tasks":[{"name":"a"},{"name":"b","params":[{"name":"p","value":"$(tasks.a.results.out)"}]}]}`,
			want: []string{"a>b:result"},
		},
		{
			name: "status reference and bracket syntax",
			spec: `{"tasks":[{"name":"a"},{"name":"c"},
				{"name":"b","params":[{"name":"p","value":"x-$(tasks.a.status)"},{"name":"q","value":"$(tasks['c'].results['r'])"}]}]}`,
			want: []string{"a>b:result", "c>b:result"},
		},
		{
			name: "when expression input and values",
			spec: `{"tasks":[{"name":"a"},{"name":"c"},
				{"name":"b","when":[{"input":"$(tasks.a.results.go)","operator":"in","values":["$(tasks.c.results.ok)","true"]}]}]}`,
			want: []string{"a>b:result", "c>b:result"},
		},
		{
			name: "matrix params and include",
			spec: `{"tasks":[{"name":"a"},{"name":"c"},
				{"name":"b","matrix":{"params":[{"name":"os","value":["$(tasks.a.results.list[*])"]}],
					"include":[{"name":"x","params":[{"name":"y","value":"$(tasks.c.results.z)"}]}]}}]}`,
			want: []string{"a>b:result", "c>b:result"},
		},
		{
			name: "workspace subPath",
			spec: `{"tasks":[{"name":"a"},{"name":"b","workspaces":[{"name":"w","workspace":"w","subPath":"$(tasks.a.results.dir)"}]}]}`,
			want: []string{"a>b:result"},
		},
		{
			name: "reference to unknown task and to itself is ignored",
			spec: `{"tasks":[{"name":"a","params":[{"name":"p","value":"$(tasks.a.results.x) $(tasks.nope.results.y)"}]}]}`,
			want: []string{},
		},
		{
			name: "runAfter wins over result for the same pair",
			spec: `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"],"params":[{"name":"p","value":"$(tasks.a.results.x)"}]}]}`,
			want: []string{"a>b:runAfter"},
		},
		{
			name: "finally gets an edge from every leaf",
			spec: `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"]},{"name":"c"}],"finally":[{"name":"f"},{"name":"g"}]}`,
			want: []string{"a>b:runAfter", "b>f:finally", "c>f:finally", "b>g:finally", "c>g:finally"},
		},
		{
			name: "finally referencing a non-leaf result keeps the result edge",
			spec: `{"tasks":[{"name":"a"},{"name":"b"}],"finally":[{"name":"f","params":[{"name":"p","value":"$(tasks.a.results.r)"}]}]}`,
			want: []string{"a>f:result", "b>f:finally"},
		},
		{
			name: "finally referencing a leaf result has one edge",
			spec: `{"tasks":[{"name":"a"}],"finally":[{"name":"f","params":[{"name":"p","value":"$(tasks.a.results.r)"}]}]}`,
			want: []string{"a>f:result"},
		},
		{
			name: "transitive reduction drops the long way round",
			spec: `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"]},
				{"name":"c","runAfter":["b","a"],"params":[{"name":"p","value":"$(tasks.a.results.x)"}]}]}`,
			want: []string{"a>b:runAfter", "b>c:runAfter"},
		},
		{
			name: "transitive reduction across mixed kinds",
			spec: `{"tasks":[{"name":"a"},{"name":"b","params":[{"name":"p","value":"$(tasks.a.results.x)"}]},
				{"name":"c","runAfter":["a"],"params":[{"name":"p","value":"$(tasks.b.results.x)"}]},
				{"name":"d","runAfter":["a","c"]}]}`,
			want: []string{"a>b:result", "b>c:result", "c>d:runAfter"},
		},
		{
			name: "diamond keeps both branches",
			spec: `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"]},{"name":"c","runAfter":["a"]},{"name":"d","runAfter":["b","c"]}]}`,
			want: []string{"a>b:runAfter", "a>c:runAfter", "b>d:runAfter", "c>d:runAfter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := Build(parse(t, tt.spec), emptyRun(), nil)

			got := edgeStrings(g)
			slices.Sort(got)

			want := slices.Clone(tt.want)
			slices.Sort(want)

			if !slices.Equal(got, want) {
				t.Fatalf("edges = %v, want %v", got, want)
			}
		})
	}
}

func taskRun(t *testing.T, name, task, cond, reason string, steps string) *unstructured.Unstructured {
	t.Helper()

	status := `"status":{"startTime":"2026-01-01T00:00:00Z","completionTime":"2026-01-01T00:01:00Z","podName":"` + name + `-pod",` +
		`"conditions":[{"type":"Succeeded","status":"` + cond + `","reason":"` + reason + `"}],"steps":` + steps + `}`

	return &unstructured.Unstructured{Object: parse(t, `{"metadata":{"name":"`+name+`","namespace":"ns",`+
		`"labels":{"tekton.dev/pipelineTask":"`+task+`"}},`+status+`}`)}
}

func TestNodes(t *testing.T) {
	spec := parse(t, `{"tasks":[{"name":"build"},{"name":"test","runAfter":["build"]},{"name":"skip","runAfter":["build"]},
		{"name":"later","runAfter":["test"]}],"finally":[{"name":"notify"}]}`)
	plr := &unstructured.Unstructured{Object: parse(t, `{"metadata":{"name":"r","namespace":"ns"},"status":{
		"conditions":[{"type":"Succeeded","status":"Unknown","reason":"Running"}],
		"skippedTasks":[{"name":"skip","reason":"When Expressions evaluated to false"}],
		"childReferences":[
			{"kind":"TaskRun","name":"r-build-1","pipelineTaskName":"build"},
			{"kind":"TaskRun","name":"r-build-0","pipelineTaskName":"build"},
			{"kind":"TaskRun","name":"r-test","pipelineTaskName":"test"}]}}`)}

	steps := `[{"name":"s1","container":"step-s1","terminated":{"exitCode":0,"reason":"Completed"}}]`
	trs := []*unstructured.Unstructured{
		taskRun(
			t,
			"r-test",
			"test",
			"Unknown",
			"Running",
			`[{"name":"go","container":"step-go","running":{"startedAt":"2026-01-01T00:00:10Z"}}]`,
		),
		taskRun(t, "r-build-0", "build", "True", "Succeeded", steps),
		taskRun(t, "r-build-1", "build", "False", "Failed", steps),
	}

	g := Build(spec, plr, trs)
	nodes := map[string]model.GraphNode{}

	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}

	if len(g.Nodes) != 5 || g.Nodes[4].ID != "notify" || !g.Nodes[4].Finally {
		t.Fatalf("nodes = %+v", g.Nodes)
	}

	build := nodes["build"]
	// The order of status.childReferences (build-1 before build-0) decides the first TaskRun.
	if build.MatrixCount != 2 || build.TaskRunName != "r-build-1" || !slices.Equal(build.TaskRunNames, []string{"r-build-1", "r-build-0"}) {
		t.Fatalf("build node = %+v", build)
	}

	if build.Status != model.StatusFailed || build.Reason != "Failed" || len(build.Steps) != 1 ||
		build.Steps[0].Status != model.StatusSucceeded {
		t.Fatalf("build node = %+v", build)
	}

	if n := nodes["test"]; n.Status != model.StatusRunning || n.Steps[0].Status != model.StatusRunning || n.MatrixCount != 1 {
		t.Fatalf("test node = %+v", n)
	}

	if n := nodes["skip"]; n.Status != model.NodeSkipped || n.Reason != "When Expressions evaluated to false" || n.TaskRunName != "" {
		t.Fatalf("skip node = %+v", n)
	}

	if n := nodes["later"]; n.Status != model.StatusPending || n.TaskRunNames == nil || n.Steps == nil {
		t.Fatalf("later node = %+v", n)
	}
}

func TestNodeWithoutTaskRunAfterRunEnded(t *testing.T) {
	spec := parse(t, `{"tasks":[{"name":"a"},{"name":"b","runAfter":["a"]}]}`)
	plr := &unstructured.Unstructured{Object: parse(t, `{"metadata":{"name":"r"},"status":{
		"conditions":[{"type":"Succeeded","status":"False","reason":"Failed"}]}}`)}

	g := Build(spec, plr, []*unstructured.Unstructured{taskRun(t, "r-a", "a", "False", "Failed", `[]`)})
	if n := g.Nodes[1]; n.Status != model.NodeSkipped || n.Reason != "Not run" {
		t.Fatalf("b = %+v", n)
	}
}

func TestGroupTaskRunsFallsBackToChildReferences(t *testing.T) {
	plr := &unstructured.Unstructured{Object: parse(t, `{"metadata":{"name":"r"},"status":{
		"childReferences":[{"kind":"TaskRun","name":"r-a","pipelineTaskName":"a"}]}}`)}
	tr := &unstructured.Unstructured{Object: parse(t, `{"metadata":{"name":"r-a"}}`)}

	got := GroupTaskRuns(plr, []*unstructured.Unstructured{tr})
	if len(got["a"]) != 1 {
		t.Fatalf("got %v", got)
	}
}

func loadFixture[T any](t *testing.T, name string) T {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}

	return v
}

// TestGoldenCarbonioProxy runs the real carbonio-proxy PipelineRun (secrets stripped).
func TestGoldenCarbonioProxy(t *testing.T) {
	plr := &unstructured.Unstructured{Object: loadFixture[map[string]any](t, "carbonio-proxy-plr.json")}

	var trs []*unstructured.Unstructured

	for _, item := range model.Slice(loadFixture[map[string]any](t, "carbonio-proxy-taskruns.json"), "items") {
		trs = append(trs, &unstructured.Unstructured{Object: item})
	}

	spec, _, _ := unstructured.NestedMap(plr.Object, "status", "pipelineSpec")
	g := Build(spec, plr, trs)

	got, err := json.MarshalIndent(map[string]any{"nodes": g.Nodes, "edges": g.Edges}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	got = append(got, '\n')
	golden := filepath.Join("testdata", "carbonio-proxy.golden.json")

	if *update {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}

	if string(want) != string(got) {
		t.Fatalf("graph differs from %s (run go test ./internal/dag -update):\n%s", golden, got)
	}

	// The facts the golden file pins, spelled out: gitleaks has no runAfter and depends on
	// metadata only through its results; metadata runs after clone.
	edges := edgeStrings(g)
	for _, w := range []string{"clone>metadata:runAfter", "metadata>gitleaks:result", "gitleaks>build-packages:runAfter"} {
		if !slices.Contains(edges, w) {
			t.Errorf("missing edge %s in %v", w, edges)
		}
	}

	for _, n := range g.Nodes {
		if n.ID == "build-packages" && (n.MatrixCount != 4 || len(n.TaskRunNames) != 4) {
			t.Errorf("build-packages matrix = %d / %v", n.MatrixCount, n.TaskRunNames)
		}

		if n.ID == "debuginfod" && (n.Status != model.NodeSkipped || n.Reason == "") {
			t.Errorf("debuginfod = %+v", n)
		}
	}
}
