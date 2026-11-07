// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxFailureMessage caps Failure.Message.
const MaxFailureMessage = 500

// WhenSkipReason is the skippedTasks reason of a task whose when-expressions were false; those skips
// are expected and not attributed to a failure.
const WhenSkipReason = "When Expressions evaluated to false"

var (
	// CSI sequences (colors, cursor) and OSC sequences (titles, hyperlinks).
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// GitHub-style annotation: "::error::msg" or "::error file=x,line=3::msg", at the start of the
	// line. Same shape as ANNOTATION in web/src/lib/logparse.ts, so the API and the SPA agree on
	// which lines are annotations.
	errorRe = regexp.MustCompile(`^::error(?: [^:]*)?::(.*)$`)
	// Group markers are not rows of their own in the SPA's log view.
	groupRe = regexp.MustCompile(`^::(?:group::.*|endgroup::\s*)$`)
)

// StripANSI removes terminal escape sequences.
func StripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// FirstFailedNode returns the Failed node that completed first (nodes without a completion time
// last, then by position), or nil.
func FirstFailedNode(nodes []GraphNode) *GraphNode {
	var best *GraphNode

	for i := range nodes {
		n := &nodes[i]
		if n.Status != StatusFailed {
			continue
		}

		if best == nil || earlier(n, best) {
			best = n
		}
	}

	return best
}

func earlier(a, b *GraphNode) bool {
	switch {
	case a.CompletionTime == nil:
		return false
	case b.CompletionTime == nil:
		return true
	default:
		return a.CompletionTime.Before(*b.CompletionTime)
	}
}

// FailedStep returns the first step that exited non-zero.
func FailedStep(steps []Step) (Step, bool) {
	i := slices.IndexFunc(steps, func(s Step) bool { return s.ExitCode != nil && *s.ExitCode != 0 })
	if i < 0 {
		return Step{}, false
	}

	return steps[i], true
}

// SkippedAfterFailure counts Skipped nodes that were not skipped by a when-expression.
func SkippedAfterFailure(nodes []GraphNode) int {
	n := 0

	for _, node := range nodes {
		if node.Status == NodeSkipped && node.Reason != WhenSkipReason {
			n++
		}
	}

	return n
}

// ErrorFromLog picks the message of a step log: the first `::error::` annotation, else the last
// non-empty line. The *int is the 1-based line number of that text, only when fromStart (the log text
// begins at the log's first line); otherwise nil. The bool is false when the log has no text at all.
func ErrorFromLog(log string, fromStart bool) (string, *int, bool) {
	// Per line, like the SPA: an unterminated escape sequence must not swallow the lines after it.
	lines := strings.Split(log, "\n")
	for i, l := range lines {
		lines[i] = StripANSI(lastRewrite(l))
	}

	pick := func(i int, text string) (string, *int, bool) {
		var ln *int

		if fromStart {
			n := i + 1
			ln = &n
		}

		return CapMessage(text), ln, true
	}

	for i, l := range lines {
		if m := errorRe.FindStringSubmatch(l); m != nil {
			if text := strings.TrimSpace(m[1]); text != "" {
				return pick(i, text)
			}
		}
	}

	for i, l := range slices.Backward(lines) {
		if text := strings.TrimSpace(l); text != "" && !groupRe.MatchString(l) {
			return pick(i, text)
		}
	}

	return "", nil, false
}

// lastRewrite returns what a terminal shows for a line: progress bars rewrite it with \r, so only
// the text after the last \r (ignoring one trailing \r from CRLF) remains.
func lastRewrite(l string) string {
	l = strings.TrimSuffix(l, "\r")
	if i := strings.LastIndexByte(l, '\r'); i >= 0 {
		return l[i+1:]
	}

	return l
}

// CapMessage strips ANSI codes and cuts the text to MaxFailureMessage characters.
func CapMessage(s string) string {
	s = strings.TrimSpace(StripANSI(s))
	if utf8.RuneCountInString(s) <= MaxFailureMessage {
		return s
	}

	return string([]rune(s)[:MaxFailureMessage-1]) + "…"
}
