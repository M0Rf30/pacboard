// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Hand-written copy of the types in docs/api.md, the contract with the Go backend.

export type RunStatus = 'Succeeded' | 'Failed' | 'Running' | 'Pending' | 'Cancelled' | 'Unknown'

export type NodeStatus = RunStatus | 'Skipped'

export interface GitInfo {
  provider: string
  repoUrl: string
  sha: string
  shortSha: string
  shaUrl: string
  shaTitle: string
  branch: string
  sourceBranch: string
  prNumber: number | null
  prUrl: string
  eventType: string
  sender: string
  checkRunUrl: string
}

export interface RunSummary {
  namespace: string
  name: string
  prName: string
  repository: string
  status: RunStatus
  reason: string
  message: string
  startTime: string | null
  completionTime: string | null
  durationSeconds: number | null
  git: GitInfo
}

export interface RepositorySummary {
  namespace: string
  name: string
  url: string
  concurrencyLimit: number | null
  lastRun: RunSummary | null
  runCount: number
  /** Up to 10 retained runs, newest first (history strip). */
  recent: RunBrief[]
  /** Median duration of the retained Succeeded runs; null when there are none. */
  typicalDurationSeconds: number | null
  /** Some commit has both a Succeeded and a Failed run among the retained runs. */
  intermittent: boolean
}

export interface RunBrief {
  name: string
  status: RunStatus
  startTime: string | null
  durationSeconds: number | null
}

/** Why a run failed, for the banner above the graph. The API sends null unless the run Failed. */
export interface Failure {
  /** First failed GraphNode.id; "" when the run failed without a failed task (timeout, validation). */
  task: string
  taskRun: string
  /** First step with a non-zero exit code; "" when unknown. */
  step: string
  exitCode: number | null
  /** First `::error::` text of that step's log, else its last non-empty line, else the run's condition message. */
  message: string
  /** 1-based line of that text in the step log; null when the message did not come from the log. */
  line: number | null
  /** Tasks Skipped after the failure (not by a false `when`). */
  skippedBecause: number
}

export interface Step {
  name: string
  status: RunStatus
  exitCode: number | null
  reason: string
  startTime: string | null
  completionTime: string | null
}

export interface GraphNode {
  id: string
  taskRunName: string
  status: NodeStatus
  reason: string
  finally: boolean
  matrixCount: number
  taskRunNames: string[]
  startTime: string | null
  completionTime: string | null
  steps: Step[]
}

export type EdgeKind = 'runAfter' | 'result' | 'finally'

export interface GraphEdge {
  from: string
  to: string
  kind: EdgeKind
}

export interface RunParam {
  name: string
  value: string
}

export interface RunDetail {
  summary: RunSummary
  nodes: GraphNode[]
  edges: GraphEdge[]
  params: RunParam[]
  canRerun: boolean
  typicalDurationSeconds: number | null
  failure: Failure | null
}

export interface Me {
  authenticated: boolean
  login: string
  avatarUrl: string
  authMode: 'github' | 'none'
  canRerun: boolean
}

export type RerunMode = 'retest' | 'test'

export interface RerunResult {
  commentUrl: string
}

export interface ChangeEvent {
  kind: 'PipelineRun' | 'TaskRun' | 'Repository'
  namespace: string
  name: string
  repository: string
  run: string
}
