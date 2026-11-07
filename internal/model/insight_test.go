// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"strconv"
	"testing"
)

func run(name string, st RunStatus, sha string, secs int64) RunSummary {
	r := RunSummary{Name: name, Status: st, Git: GitInfo{SHA: sha}}
	if secs >= 0 {
		r.DurationSeconds = &secs
	}

	return r
}

func TestTypicalDuration(t *testing.T) {
	for name, tt := range map[string]struct {
		runs []RunSummary
		want int64 // -1 = nil
	}{
		"none":      {nil, -1},
		"none ok":   {[]RunSummary{run("a", StatusFailed, "", 10), run("b", StatusRunning, "", 5)}, -1},
		"odd":       {[]RunSummary{run("a", StatusSucceeded, "", 30), run("b", StatusSucceeded, "", 10), run("c", StatusSucceeded, "", 20), run("d", StatusFailed, "", 999)}, 20},
		"even":      {[]RunSummary{run("a", StatusSucceeded, "", 10), run("b", StatusSucceeded, "", 21), run("c", StatusSucceeded, "", 40), run("d", StatusSucceeded, "", 90)}, 30},
		"no length": {[]RunSummary{run("a", StatusSucceeded, "", -1), run("b", StatusSucceeded, "", 8)}, 8},
	} {
		t.Run(name, func(t *testing.T) {
			got := TypicalDuration(tt.runs)

			switch {
			case tt.want < 0 && got != nil:
				t.Fatalf("got %d, want nil", *got)
			case tt.want >= 0 && (got == nil || *got != tt.want):
				t.Fatalf("got %v, want %d", got, tt.want)
			}
		})
	}
}

func TestIntermittent(t *testing.T) {
	for name, tt := range map[string]struct {
		runs []RunSummary
		want bool
	}{
		"empty":                {nil, false},
		"same sha both":        {[]RunSummary{run("a", StatusFailed, "abc", 1), run("b", StatusSucceeded, "abc", 1)}, true},
		"different shas":       {[]RunSummary{run("a", StatusFailed, "abc", 1), run("b", StatusSucceeded, "def", 1)}, false},
		"only failures":        {[]RunSummary{run("a", StatusFailed, "abc", 1), run("b", StatusFailed, "abc", 1)}, false},
		"running does not tip": {[]RunSummary{run("a", StatusFailed, "abc", 1), run("b", StatusRunning, "abc", 1)}, false},
		"no sha":               {[]RunSummary{run("a", StatusFailed, "", 1), run("b", StatusSucceeded, "", 1)}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Intermittent(tt.runs); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecent(t *testing.T) {
	if got := Recent(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty must be a non-nil empty list, got %#v", got)
	}

	var runs []RunSummary
	for i := range 14 {
		runs = append(runs, run("r"+strconv.Itoa(i), StatusSucceeded, "", int64(i)))
	}

	got := Recent(runs)
	if len(got) != RecentRuns || got[0].Name != "r0" || got[RecentRuns-1].Name != "r9" {
		t.Fatalf("got %d entries, first %q", len(got), got[0].Name)
	}

	if got[3].Status != StatusSucceeded || *got[3].DurationSeconds != 3 {
		t.Fatalf("brief = %+v", got[3])
	}
}
