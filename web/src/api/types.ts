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
