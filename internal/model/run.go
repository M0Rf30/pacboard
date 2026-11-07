// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PaC label and annotation keys and the Tekton labels pacboard reads.
const (
	pacPrefix = "pipelinesascode.tekton.dev/"

	LabelRepository   = pacPrefix + "repository"
	LabelPipelineRun  = "tekton.dev/pipelineRun"
	LabelPipelineTask = "tekton.dev/pipelineTask"

	defaultGitHost = "https://github.com"
	shortSHALen    = 7
)

// Meta reads a PaC key: the annotation first, then the label. key is the part after
// "pipelinesascode.tekton.dev/".
func Meta(u *unstructured.Unstructured, key string) string {
	full := pacPrefix + key
	if v := u.GetAnnotations()[full]; v != "" {
		return v
	}

	return u.GetLabels()[full]
}

// RepoRef returns the GitHub org and repository of a run.
func RepoRef(u *unstructured.Unstructured) (string, string) {
	return Meta(u, "url-org"), Meta(u, "url-repository")
}

// SortTime orders runs: start time, else creation time while the run is still pending.
func SortTime(u *unstructured.Unstructured) time.Time {
	if t := Time(u.Object, "status", "startTime"); t != nil {
		return *t
	}

	return u.GetCreationTimestamp().Time
}

func stripRef(branch string) string {
	return strings.TrimPrefix(branch, "refs/heads/")
}

// PRNumber returns the pull request number of a run, nil when it has none.
func PRNumber(u *unstructured.Unstructured) *int {
	n, err := strconv.Atoi(Meta(u, "pull-request"))
	if err != nil || n <= 0 {
		return nil
	}

	return &n
}

// SafeURL returns raw when it is an absolute http(s) URL with a host, else "". Run
// annotations can be influenced by a pull request's .tekton templates, so no other scheme
// (javascript:, data:) or relative form ("//evil") may reach a link.
func SafeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return ""
	}

	return raw
}

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	hexRe  = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	numRe  = regexp.MustCompile(`^[0-9]+$`)
)

// repoURLOf is the validated repository URL: the repo-url annotation, else built from
// url-org / url-repository on github.com (or the ghe-url host).
func repoURLOf(u *unstructured.Unstructured) string {
	if v := SafeURL(Meta(u, "repo-url")); v != "" {
		return strings.TrimSuffix(v, "/")
	}

	org, repo := RepoRef(u)
	if !nameRe.MatchString(org) || !nameRe.MatchString(repo) {
		return ""
	}

	host := SafeURL(Meta(u, "ghe-url"))
	if host == "" {
		host = defaultGitHost
	}

	return SafeURL(strings.TrimSuffix(host, "/") + "/" + org + "/" + repo)
}

// GitInfoOf maps the PaC labels and annotations of a PipelineRun. Every URL it returns is
// an absolute http(s) URL or "".
func GitInfoOf(u *unstructured.Unstructured) GitInfo {
	g := GitInfo{
		Provider:     Meta(u, "git-provider"),
		RepoURL:      repoURLOf(u),
		SHA:          Meta(u, "sha"),
		SHAURL:       SafeURL(Meta(u, "sha-url")),
		SHATitle:     Meta(u, "sha-title"),
		Branch:       stripRef(Meta(u, "branch")),
		SourceBranch: stripRef(Meta(u, "source-branch")),
		PRNumber:     PRNumber(u),
		EventType:    Meta(u, "event-type"),
		Sender:       Meta(u, "sender"),
	}

	g.ShortSHA = g.SHA
	if len(g.ShortSHA) > shortSHALen {
		g.ShortSHA = g.ShortSHA[:shortSHALen]
	}

	if g.RepoURL == "" {
		return g
	}

	if g.SHAURL == "" && hexRe.MatchString(g.SHA) {
		g.SHAURL = g.RepoURL + "/commit/" + g.SHA
	}

	if g.PRNumber != nil {
		g.PRURL = g.RepoURL + "/pull/" + strconv.Itoa(*g.PRNumber)
	}

	if id := Meta(u, "check-run-id"); numRe.MatchString(id) {
		g.CheckRunURL = g.RepoURL + "/runs/" + id
	}

	return g
}

// RunSummaryOf maps a PipelineRun. now is used for the duration of a run in progress.
func RunSummaryOf(u *unstructured.Unstructured, now time.Time) RunSummary {
	info := StatusOf(u.Object)
	start := Time(u.Object, "status", "startTime")
	end := Time(u.Object, "status", "completionTime")

	return RunSummary{
		Namespace:       u.GetNamespace(),
		Name:            u.GetName(),
		PRName:          Meta(u, "original-prname"),
		Repository:      Meta(u, "repository"),
		Status:          info.Status,
		Reason:          info.Reason,
		Message:         info.Message,
		StartTime:       start,
		CompletionTime:  end,
		DurationSeconds: Duration(start, end, info.Status, now),
		Git:             GitInfoOf(u),
	}
}

// Duration is completion-start, or now-start while running.
func Duration(start, end *time.Time, status RunStatus, now time.Time) *int64 {
	var d time.Duration

	switch {
	case start == nil:
		return nil
	case end != nil:
		d = end.Sub(*start)
	case status == StatusRunning:
		d = now.Sub(*start)
	default:
		return nil
	}

	secs := max(int64(d/time.Second), 0)

	return &secs
}

// RepositoryOf maps a PaC Repository CR; LastRun and RunCount are filled by the caller.
func RepositoryOf(u *unstructured.Unstructured) RepositorySummary {
	r := RepositorySummary{
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
		URL:       Str(u.Object, "spec", "url"),
	}

	v, found, _ := unstructured.NestedFieldNoCopy(u.Object, "spec", "concurrency_limit")
	if found {
		if n, ok := toInt(v); ok {
			r.ConcurrencyLimit = &n
		}
	}

	return r
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
