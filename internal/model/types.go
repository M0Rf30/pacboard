// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package model holds the types of the pacboard HTTP contract (docs/api.md)
// and the mapping from Tekton / Pipelines-as-Code objects to them.
package model

import "time"

// RunStatus is the status of a run, task run or step.
type RunStatus string

// Run statuses of the contract. NodeSkipped only applies to graph nodes.
const (
	StatusSucceeded RunStatus = "Succeeded"
	StatusFailed    RunStatus = "Failed"
	StatusRunning   RunStatus = "Running"
	StatusPending   RunStatus = "Pending"
	StatusCancelled RunStatus = "Cancelled"
	StatusUnknown   RunStatus = "Unknown"
	NodeSkipped     RunStatus = "Skipped"
)

// Valid reports whether s is a status a client may filter by.
func (s RunStatus) Valid() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusRunning, StatusPending, StatusCancelled, StatusUnknown:
		return true
	case NodeSkipped:
		return false
	}

	return false
}

// GitInfo links a run back to GitHub.
type GitInfo struct {
	Provider     string `json:"provider"`
	RepoURL      string `json:"repoUrl"`
	SHA          string `json:"sha"`
	ShortSHA     string `json:"shortSha"`
	SHAURL       string `json:"shaUrl"`
	SHATitle     string `json:"shaTitle"`
	Branch       string `json:"branch"`
	SourceBranch string `json:"sourceBranch"`
	PRNumber     *int   `json:"prNumber"`
	PRURL        string `json:"prUrl"`
	EventType    string `json:"eventType"`
	Sender       string `json:"sender"`
	CheckRunURL  string `json:"checkRunUrl"`
}

// RunSummary describes one PipelineRun.
type RunSummary struct {
	Namespace       string     `json:"namespace"`
	Name            string     `json:"name"`
	PRName          string     `json:"prName"`
	Repository      string     `json:"repository"`
	Status          RunStatus  `json:"status"`
	Reason          string     `json:"reason"`
	Message         string     `json:"message"`
	StartTime       *time.Time `json:"startTime"`
	CompletionTime  *time.Time `json:"completionTime"`
	DurationSeconds *int64     `json:"durationSeconds"`
	Git             GitInfo    `json:"git"`
}

// RepositorySummary describes one PaC Repository with its latest run.
type RepositorySummary struct {
	Namespace        string      `json:"namespace"`
	Name             string      `json:"name"`
	URL              string      `json:"url"`
	ConcurrencyLimit *int        `json:"concurrencyLimit"`
	LastRun          *RunSummary `json:"lastRun"`
	RunCount         int         `json:"runCount"`
}

// Step is one step container of a TaskRun.
type Step struct {
	Name           string     `json:"name"`
	Status         RunStatus  `json:"status"`
	ExitCode       *int       `json:"exitCode"`
	Reason         string     `json:"reason"`
	StartTime      *time.Time `json:"startTime"`
	CompletionTime *time.Time `json:"completionTime"`
}

// GraphNode is one pipeline task of a run.
type GraphNode struct {
	ID             string     `json:"id"`
	TaskRunName    string     `json:"taskRunName"`
	Status         RunStatus  `json:"status"`
	Reason         string     `json:"reason"`
	Finally        bool       `json:"finally"`
	MatrixCount    int        `json:"matrixCount"`
	TaskRunNames   []string   `json:"taskRunNames"`
	StartTime      *time.Time `json:"startTime"`
	CompletionTime *time.Time `json:"completionTime"`
	Steps          []Step     `json:"steps"`
}

// Edge kinds.
const (
	EdgeRunAfter = "runAfter"
	EdgeResult   = "result"
	EdgeFinally  = "finally"
)

// GraphEdge is a dependency: To starts after From.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Param is a stringified PipelineRun parameter.
type Param struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RunDetail is a run with its task graph.
type RunDetail struct {
	Summary  RunSummary  `json:"summary"`
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	Params   []Param     `json:"params"`
	CanRerun bool        `json:"canRerun"`
}

// Me describes the current user.
type Me struct {
	Authenticated bool   `json:"authenticated"`
	Login         string `json:"login"`
	AvatarURL     string `json:"avatarUrl"`
	AuthMode      string `json:"authMode"`
	CanRerun      bool   `json:"canRerun"`
}
