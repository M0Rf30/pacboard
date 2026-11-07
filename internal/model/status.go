// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// StatusInfo is the outcome of a PipelineRun or TaskRun.
type StatusInfo struct {
	Status  RunStatus
	Reason  string
	Message string
}

// Terminal reports whether the run can no longer change.
func (s StatusInfo) Terminal() bool {
	switch s.Status {
	case StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	case StatusRunning, StatusPending, StatusUnknown, NodeSkipped:
		return false
	}

	return false
}

// StatusOf maps the Succeeded condition of a PipelineRun or TaskRun to a RunStatus.
// A missing condition is Pending.
func StatusOf(obj map[string]any) StatusInfo {
	for _, c := range Slice(obj, "status", "conditions") {
		if Str(c, "type") != "Succeeded" {
			continue
		}

		return mapCondition(Str(c, "status"), Str(c, "reason"), Str(c, "message"))
	}

	return StatusInfo{Status: StatusPending}
}

func mapCondition(status, reason, message string) StatusInfo {
	info := StatusInfo{Reason: reason, Message: message}

	switch status {
	case "True":
		info.Status = StatusSucceeded
	case "False":
		switch reason {
		case "Cancelled", "CancelledRunFinally", "StoppedRunFinally", "TaskRunCancelled":
			info.Status = StatusCancelled
		default:
			info.Status = StatusFailed
		}
	case "Unknown":
		switch reason {
		case "PipelineRunPending", "Pending", "TaskRunPending":
			info.Status = StatusPending
		default:
			info.Status = StatusRunning
		}
	default:
		info.Status = StatusUnknown
	}

	return info
}

// StepsOf maps status.steps of a TaskRun.
func StepsOf(tr *unstructured.Unstructured) []Step {
	out := []Step{}

	for _, s := range Slice(tr.Object, "status", "steps") {
		step := Step{Name: Str(s, "name"), Status: StatusPending}

		switch {
		case s["terminated"] != nil:
			t, _ := s["terminated"].(map[string]any)
			step.Reason = Str(t, "reason")
			step.StartTime = Time(t, "startedAt")
			step.CompletionTime = Time(t, "finishedAt")

			code := exitCode(t["exitCode"])
			step.ExitCode = &code

			if code == 0 {
				step.Status = StatusSucceeded
			} else {
				step.Status = StatusFailed
			}
		case s["running"] != nil:
			r, _ := s["running"].(map[string]any)
			step.Status = StatusRunning
			step.StartTime = Time(r, "startedAt")
		case s["waiting"] != nil:
			w, _ := s["waiting"].(map[string]any)
			step.Reason = Str(w, "reason")
		}

		out = append(out, step)
	}

	return out
}

func exitCode(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
