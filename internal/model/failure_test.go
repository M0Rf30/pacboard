// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"strings"
	"testing"
	"time"
)

func TestErrorFromLog(t *testing.T) {
	for name, tt := range map[string]struct {
		log       string
		fromStart bool
		msg       string
		line      int // 0 = nil
		ok        bool
	}{
		"plain":            {"a\n::error::boom\n", true, "boom", 2, true},
		"params":           {"::error file=a.go,line=3,col=1::bad\n", true, "bad", 1, true},
		"first wins":       {"::error::one\n::error::two\n", true, "one", 1, true},
		"not anchored":     {"+ echo \"::error::x\"\n", true, "+ echo \"::error::x\"", 1, true},
		"unterminated osc": {"a\x1b]0;title\nb\n::error::boom\nc\x07\n", true, "boom", 3, true},
		"endgroup last":    {"::group::g\nfailed here\n::endgroup::\n", true, "failed here", 2, true},
		"group only":       {"::group::g\n::endgroup::\n", true, "", 0, false},
		"cr rewrite":       {"10%\r50%\r100% done\n", true, "100% done", 1, true},
		"cr rewrite error": {"::error::x\rprogress\r::error::real\n", true, "real", 1, true},
		"ansi":             {"\x1b[31;1m::error::\x1b[0mred\x1b[K\n", true, "red", 1, true},
		"warning ignore":   {"::warning::w\nlast line\n\n", true, "last line", 2, true},
		"not from start":   {"x\n::error::boom\n", false, "boom", 0, true},
		"empty":            {"", true, "", 0, false},
		"blank":            {"\n \n", true, "", 0, false},
		"crlf":             {"x\r\ndone\r\n", true, "done", 2, true},
	} {
		t.Run(name, func(t *testing.T) {
			msg, line, ok := ErrorFromLog(tt.log, tt.fromStart)
			if msg != tt.msg || ok != tt.ok || (line == nil) != (tt.line == 0) || (line != nil && *line != tt.line) {
				t.Fatalf("got %q %v %v", msg, line, ok)
			}
		})
	}
}

func TestCapMessage(t *testing.T) {
	got := CapMessage(strings.Repeat("x", 700))
	if n := len([]rune(got)); n != MaxFailureMessage || !strings.HasSuffix(got, "…") {
		t.Fatalf("len %d", n)
	}
}

func TestFirstFailedNodeAndSkips(t *testing.T) {
	a, b := time.Unix(1, 0), time.Unix(2, 0)
	nodes := []GraphNode{
		{ID: "ok", Status: StatusSucceeded},
		{ID: "late", Status: StatusFailed, CompletionTime: &b},
		{ID: "none", Status: StatusFailed},
		{ID: "early", Status: StatusFailed, CompletionTime: &a},
		{ID: "s1", Status: NodeSkipped, Reason: WhenSkipReason},
		{ID: "s2", Status: NodeSkipped, Reason: "Parent Tasks were skipped"},
		{ID: "s3", Status: NodeSkipped, Reason: "Not run"},
	}

	if n := FirstFailedNode(nodes); n == nil || n.ID != "early" {
		t.Fatalf("first failed = %+v", n)
	}

	if FirstFailedNode(nodes[:1]) != nil {
		t.Fatal("no failed node expected")
	}

	if got := SkippedAfterFailure(nodes); got != 2 {
		t.Fatalf("skipped = %d", got)
	}
}

func TestFailedStep(t *testing.T) {
	zero, two := 0, 2
	steps := []Step{{Name: "a", ExitCode: &zero}, {Name: "b"}, {Name: "c", ExitCode: &two}}

	if s, ok := FailedStep(steps); !ok || s.Name != "c" {
		t.Fatalf("got %+v %v", s, ok)
	}

	if _, ok := FailedStep(steps[:2]); ok {
		t.Fatal("no failed step expected")
	}
}
