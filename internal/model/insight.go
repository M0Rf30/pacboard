// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import "slices"

// RecentRuns is how many runs a RepositorySummary lists in Recent.
const RecentRuns = 10

// BriefOf reduces a run to its history strip entry.
func BriefOf(r RunSummary) RunBrief {
	return RunBrief{Name: r.Name, Status: r.Status, StartTime: r.StartTime, DurationSeconds: r.DurationSeconds}
}

// Recent returns the first RecentRuns of runs (newest first) as briefs; never nil.
func Recent(runs []RunSummary) []RunBrief {
	out := make([]RunBrief, 0, min(len(runs), RecentRuns))

	for _, r := range runs[:min(len(runs), RecentRuns)] {
		out = append(out, BriefOf(r))
	}

	return out
}

// TypicalDuration is the median duration of the Succeeded runs that have one; nil when there is none.
// An even count averages the two middle values (rounded down).
func TypicalDuration(runs []RunSummary) *int64 {
	var ds []int64

	for _, r := range runs {
		if r.Status == StatusSucceeded && r.DurationSeconds != nil {
			ds = append(ds, *r.DurationSeconds)
		}
	}

	if len(ds) == 0 {
		return nil
	}

	slices.Sort(ds)

	mid := len(ds) / 2
	med := ds[mid]

	if len(ds)%2 == 0 {
		med = (ds[mid-1] + ds[mid]) / 2
	}

	return &med
}

// Intermittent reports whether some commit has both a Succeeded and a Failed run.
func Intermittent(runs []RunSummary) bool {
	const (
		succeeded = 1 << iota
		failed
	)

	seen := map[string]int{}

	for _, r := range runs {
		if r.Git.SHA == "" {
			continue
		}

		switch r.Status {
		case StatusSucceeded:
			seen[r.Git.SHA] |= succeeded
		case StatusFailed:
			seen[r.Git.SHA] |= failed
		case StatusRunning, StatusPending, StatusCancelled, StatusUnknown, NodeSkipped:
		}

		if seen[r.Git.SHA] == succeeded|failed {
			return true
		}
	}

	return false
}

// Insight fills Recent, TypicalDurationSeconds and Intermittent from the repository's runs, newest first.
func (r *RepositorySummary) Insight(runs []RunSummary) {
	r.Recent = Recent(runs)
	r.TypicalDurationSeconds = TypicalDuration(runs)
	r.Intermittent = Intermittent(runs)
}
