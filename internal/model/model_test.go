// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func obj(t *testing.T, s string) *unstructured.Unstructured {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}

	return &unstructured.Unstructured{Object: m}
}

func cond(status, reason string) string {
	return `{"status":{"conditions":[{"type":"Succeeded","status":"` + status + `","reason":"` + reason + `","message":"m"}]}}`
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		json string
		want RunStatus
	}{
		{"no condition", `{"status":{}}`, StatusPending},
		{"no status at all", `{}`, StatusPending},
		{"other condition type only", `{"status":{"conditions":[{"type":"Ready","status":"True"}]}}`, StatusPending},
		{"succeeded", cond("True", "Completed"), StatusSucceeded},
		{"succeeded with failures ignored", cond("True", "Succeeded"), StatusSucceeded},
		{"failed", cond("False", "Failed"), StatusFailed},
		{"timeout", cond("False", "PipelineRunTimeout"), StatusFailed},
		{"cancelled", cond("False", "Cancelled"), StatusCancelled},
		{"cancelled run finally", cond("False", "CancelledRunFinally"), StatusCancelled},
		{"stopped run finally", cond("False", "StoppedRunFinally"), StatusCancelled},
		{"taskrun cancelled", cond("False", "TaskRunCancelled"), StatusCancelled},
		{"running", cond("Unknown", "Running"), StatusRunning},
		{"started", cond("Unknown", "Started"), StatusRunning},
		{"pending reason", cond("Unknown", "PipelineRunPending"), StatusPending},
		{"weird status", cond("Maybe", "x"), StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusOf(obj(t, tt.json).Object).Status; got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
		})
	}

	info := StatusOf(obj(t, cond("False", "Failed")).Object)
	if info.Reason != "Failed" || info.Message != "m" || !info.Terminal() {
		t.Fatalf("info = %+v", info)
	}

	if StatusOf(obj(t, cond("Unknown", "Running")).Object).Terminal() {
		t.Fatal("running must not be terminal")
	}
}

func TestGitInfo(t *testing.T) {
	const prefix = "pipelinesascode.tekton.dev/"

	run := obj(t, `{"metadata":{"name":"r","namespace":"ci",
		"labels":{"`+prefix+`pull-request":"323","`+prefix+`check-run-id":"128482194628","`+prefix+`sha":"41cd53e92af56d6f708133e56fe4297a9217b35b",
			"`+prefix+`url-org":"zextras","`+prefix+`url-repository":"carbonio-proxy","`+prefix+`event-type":"label-event"},
		"annotations":{"`+prefix+`repo-url":"https://github.com/zextras/carbonio-proxy","`+prefix+`branch":"refs/heads/main",
			"`+prefix+`source-branch":"refs/heads/ci/tekton-eval","`+prefix+`sha-title":"ci: pin","`+prefix+`sender":"M0Rf30",
			"`+prefix+`git-provider":"github","`+prefix+`event-type":"test-all-comment",
			"`+prefix+`sha-url":"https://github.com/zextras/carbonio-proxy/commit/41cd53e92af56d6f708133e56fe4297a9217b35b"}}}`)

	g := GitInfoOf(run)

	if g.RepoURL != "https://github.com/zextras/carbonio-proxy" || g.Provider != "github" || g.SHATitle != "ci: pin" ||
		g.Sender != "M0Rf30" {
		t.Fatalf("git = %+v", g)
	}

	if g.PRNumber == nil || *g.PRNumber != 323 || g.PRURL != "https://github.com/zextras/carbonio-proxy/pull/323" {
		t.Fatalf("pr = %v %q", g.PRNumber, g.PRURL)
	}

	if g.CheckRunURL != "https://github.com/zextras/carbonio-proxy/runs/128482194628" {
		t.Fatalf("check run url = %q", g.CheckRunURL)
	}

	if g.Branch != "main" || g.SourceBranch != "ci/tekton-eval" {
		t.Fatalf("branches = %q / %q (refs/heads/ must be stripped)", g.Branch, g.SourceBranch)
	}

	if g.SHA != "41cd53e92af56d6f708133e56fe4297a9217b35b" || g.ShortSHA != "41cd53e" || !strings.HasSuffix(g.SHAURL, "/commit/"+g.SHA) {
		t.Fatalf("sha = %q %q %q", g.SHA, g.ShortSHA, g.SHAURL)
	}

	// Annotation first, then label.
	if g.EventType != "test-all-comment" {
		t.Fatalf("event type = %q, want the annotation's", g.EventType)
	}
}

func TestGitInfoFallbacks(t *testing.T) {
	const prefix = "pipelinesascode.tekton.dev/"

	// No repo-url, sha-url, PR or check run: derived from org/repo, no PR links.
	g := GitInfoOf(obj(t, `{"metadata":{"labels":{"`+prefix+`url-org":"zextras","`+prefix+`url-repository":"x","`+prefix+`sha":"abc1234def",
		"`+prefix+`branch":"refs/tags/v1"}}}`))

	if g.RepoURL != "https://github.com/zextras/x" || g.SHAURL != "https://github.com/zextras/x/commit/abc1234def" ||
		g.PRNumber != nil || g.PRURL != "" || g.CheckRunURL != "" || g.Branch != "refs/tags/v1" {
		t.Fatalf("git = %+v", g)
	}

	// Short SHA stays as is; empty everything is all empty.
	if g := GitInfoOf(obj(t, `{"metadata":{"labels":{"`+prefix+`sha":"abc"}}}`)); g.ShortSHA != "abc" || g.RepoURL != "" || g.SHAURL != "" {
		t.Fatalf("git = %+v", g)
	}

	if g := GitInfoOf(obj(t, `{"metadata":{"labels":{"`+prefix+`pull-request":"0"}}}`)); g.PRNumber != nil {
		t.Fatalf("pull-request=0 must be no PR: %+v", g)
	}
}

// URLs come from annotations a pull request's .tekton template can influence.
func TestGitInfoNeverEmitsUnsafeURLs(t *testing.T) {
	const prefix = "pipelinesascode.tekton.dev/"

	bad := []string{
		"javascript:alert(1)",
		"//evil",
		"//evil.com/x",
		"data:text/html,x",
		"/relative",
		"https:///nohost",
		"ftp://x/y",
		"https://",
		"not a url",
		"  https://a.b",
	}

	for _, v := range bad {
		run := obj(t, `{"metadata":{"annotations":{"`+prefix+`repo-url":`+quote(v)+`,"`+prefix+`sha-url":`+quote(v)+`,
			"`+prefix+`pull-request":"1","`+prefix+`check-run-id":"2","`+prefix+`sha":"abc"},
			"labels":{"`+prefix+`url-org":"o","`+prefix+`url-repository":"r"}}}`)

		g := GitInfoOf(run)
		for name, got := range map[string]string{"repo": g.RepoURL, "sha": g.SHAURL, "pr": g.PRURL, "check": g.CheckRunURL} {
			// The unusable repo-url falls back to the validated org/repo form; the bad value itself never shows.
			if strings.Contains(got, "evil") || strings.Contains(got, "javascript") || strings.Contains(got, "data:") ||
				strings.Contains(got, "not a url") {
				t.Errorf("%q leaked into %s url %q", v, name, got)
			}

			if got != "" && !strings.HasPrefix(got, "https://github.com/o/r") {
				t.Errorf("%q: %s url = %q, want the derived github.com/o/r form or empty", v, name, got)
			}
		}
	}

	// Without a usable org/repo to fall back on, everything is empty.
	run := obj(t, `{"metadata":{"annotations":{"`+prefix+`repo-url":"javascript:alert(1)","`+prefix+`sha-url":"//evil",
		"`+prefix+`pull-request":"1","`+prefix+`check-run-id":"2"}}}`)
	if g := GitInfoOf(run); g.RepoURL != "" || g.SHAURL != "" || g.PRURL != "" || g.CheckRunURL != "" {
		t.Fatalf("git = %+v", g)
	}

	// A hostile org/repo or check run id never builds a URL.
	run = obj(
		t,
		`{"metadata":{"labels":{"`+prefix+`url-org":"../evil","`+prefix+`url-repository":"r?x=1","`+prefix+`check-run-id":"1/../2"}}}`,
	)
	if g := GitInfoOf(run); g.RepoURL != "" || g.CheckRunURL != "" {
		t.Fatalf("git = %+v", g)
	}

	// A good https and http URL pass through.
	for _, v := range []string{"https://github.com/o/r", "http://ghe.local:8080/o/r"} {
		run := obj(t, `{"metadata":{"annotations":{"`+prefix+`repo-url":`+quote(v)+`,"`+prefix+`pull-request":"5"}}}`)
		if g := GitInfoOf(run); g.RepoURL != v || g.PRURL != v+"/pull/5" {
			t.Errorf("good url %q: %+v", v, g)
		}
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}

func TestRunSummaryDuration(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	done := RunSummaryOf(
		obj(
			t,
			`{"metadata":{"name":"r","namespace":"ci"},"status":{"startTime":"2026-01-01T10:00:00Z","completionTime":"2026-01-01T10:01:30Z",
		"conditions":[{"type":"Succeeded","status":"True","reason":"Completed"}]}}`,
		),
		now,
	)
	if done.DurationSeconds == nil || *done.DurationSeconds != 90 || done.Status != StatusSucceeded || done.Reason != "Completed" {
		t.Fatalf("done = %+v", done)
	}

	running := RunSummaryOf(
		obj(t, `{"status":{"startTime":"2026-01-01T11:59:00Z","conditions":[{"type":"Succeeded","status":"Unknown","reason":"Running"}]}}`),
		now,
	)
	if running.DurationSeconds == nil || *running.DurationSeconds != 60 || running.CompletionTime != nil {
		t.Fatalf("running = %+v", running)
	}

	pending := RunSummaryOf(obj(t, `{"status":{}}`), now)
	if pending.DurationSeconds != nil || pending.StartTime != nil || pending.Status != StatusPending {
		t.Fatalf("pending = %+v", pending)
	}

	// JSON shape: null timestamps, camelCase keys.
	b, err := json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{`"startTime":null`, `"durationSeconds":null`, `"prNumber":null`, `"shortSha":""`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json %s lacks %s", b, want)
		}
	}
}

func TestStepsOf(t *testing.T) {
	tr := obj(t, `{"status":{"steps":[
		{"name":"ok","terminated":{"exitCode":0,"reason":"Completed","startedAt":"2026-01-01T10:00:00Z","finishedAt":"2026-01-01T10:00:05Z"}},
		{"name":"bad","terminated":{"exitCode":2,"reason":"Error"}},
		{"name":"run","running":{"startedAt":"2026-01-01T10:00:06Z"}},
		{"name":"wait","waiting":{"reason":"PodInitializing"}},
		{"name":"none"}]}}`)

	got := StepsOf(tr)
	want := []struct {
		status RunStatus
		code   *int
		reason string
	}{
		{StatusSucceeded, new(0), "Completed"},
		{StatusFailed, new(2), "Error"},
		{StatusRunning, nil, ""},
		{StatusPending, nil, "PodInitializing"},
		{StatusPending, nil, ""},
	}

	if len(got) != len(want) {
		t.Fatalf("steps = %+v", got)
	}

	for i, w := range want {
		g := got[i]
		if g.Status != w.status || g.Reason != w.reason || (g.ExitCode == nil) != (w.code == nil) ||
			(w.code != nil && *g.ExitCode != *w.code) {
			t.Errorf("step %d = %+v, want %+v", i, g, w)
		}
	}

	if got[0].StartTime == nil || got[0].CompletionTime == nil || got[2].StartTime == nil {
		t.Errorf("times missing: %+v", got)
	}
}

func TestParamsAndRepository(t *testing.T) {
	plr := obj(t, `{"spec":{"params":[{"name":"a","value":"x"},{"name":"b","value":["1","2"]},{"name":"c","value":{"k":"v"}}]}}`)

	got := Params(plr.Object)
	if len(got) != 3 || got[0].Value != "x" || got[1].Value != `["1","2"]` || got[2].Value != `{"k":"v"}` {
		t.Fatalf("params = %+v", got)
	}

	if p := Params(obj(t, `{}`).Object); p == nil || len(p) != 0 {
		t.Fatalf("empty params must be an empty list, got %#v", p)
	}

	repo := RepositoryOf(obj(t, `{"metadata":{"name":"r","namespace":"ci"},"spec":{"url":"https://github.com/o/r","concurrency_limit":3}}`))
	if repo.URL != "https://github.com/o/r" || repo.ConcurrencyLimit == nil || *repo.ConcurrencyLimit != 3 {
		t.Fatalf("repo = %+v", repo)
	}

	if u := RepositoryOf(obj(t, `{"spec":{"url":"javascript:alert(1)"}}`)).URL; u != "" {
		t.Fatalf("unsafe repository url kept: %q", u)
	}

	if RepositoryOf(obj(t, `{"spec":{}}`)).ConcurrencyLimit != nil {
		t.Fatal("concurrency limit must be null when unset")
	}
}
