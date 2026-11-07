// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { Failure, GraphNode, RunStatus } from '@/api/types'

/** What the task panel shows: a task, one of its steps ("" = all steps) and a matrix TaskRun ("" = the default one). */
export interface Selection {
  task: string
  step: string
  taskRun: string
}

const NONE: Selection = { task: '', step: '', taskRun: '' }

/** Tasks that ran (or are running), as opposed to Skipped and not yet started. */
function isExecuted(n: GraphNode): boolean {
  return n.status !== 'Skipped' && n.status !== 'Pending'
}

function firstFailedStep(n: GraphNode): string {
  return n.steps.find((s) => s.status === 'Failed')?.name ?? ''
}

/**
 * The step to open when a task is picked: its failed step, else its running step while it runs, else its
 * only step; a task with several quiet steps opens on all of them.
 */
export function defaultStep(n: GraphNode): string {
  const failed = firstFailedStep(n)
  if (failed) return failed
  if (n.status === 'Running') {
    const running = n.steps.find((s) => s.status === 'Running')
    if (running) return running.name
  }
  return n.steps.length === 1 ? n.steps[0]!.name : ''
}

/** The TaskRun a selection shows: the chosen one, else the node's first (the "" default resolved). */
export function resolveTaskRun(n: GraphNode, taskRun: string): string {
  return taskRun || n.taskRunNames[0] || n.taskRunName
}

/** Selection for a task the user clicked; on the failed task of a matrix it is the failing TaskRun. */
export function pickTask(n: GraphNode, failure: Failure | null = null): Selection {
  const taskRun = failure && failure.task === n.id && n.taskRunNames.includes(failure.taskRun) ? failure.taskRun : ''
  return { task: n.id, step: defaultStep(n), taskRun }
}

/** The task that ended last among the executed ones (by start time; API order breaks ties). */
export function lastExecuted(nodes: readonly GraphNode[]): GraphNode | undefined {
  let best: GraphNode | undefined
  let bestStart = -Infinity
  for (const n of nodes) {
    if (!isExecuted(n)) continue
    const t = n.startTime ? Date.parse(n.startTime) : Number.NaN
    const start = Number.isNaN(t) ? -Infinity : t
    if (best === undefined || start >= bestStart) {
      best = n
      bestStart = start
    }
  }
  return best
}

/**
 * Where a run page lands when the link names nothing: the failure (its task and step, the TaskRun for
 * a matrix), else the first failed task, else the last executed one.
 */
export function landingSelection(nodes: readonly GraphNode[], status: RunStatus, failure: Failure | null): Selection {
  if (status === 'Failed') {
    const byFailure = failure?.task ? nodes.find((n) => n.id === failure.task) : undefined
    const node = byFailure ?? nodes.find((n) => n.status === 'Failed')
    if (node) {
      const step = byFailure && failure?.step && node.steps.some((s) => s.name === failure.step) ? failure.step : defaultStep(node)
      const taskRun = byFailure && failure?.taskRun && node.taskRunNames.includes(failure.taskRun) ? failure.taskRun : ''
      return { task: node.id, step, taskRun }
    }
  }
  const last = lastExecuted(nodes)
  return last ? pickTask(last) : NONE
}

/** The steps the panel can select, "" (all steps) first when there is more than one step. */
export function stepChoices(n: GraphNode): string[] {
  const names = n.steps.map((s) => s.name)
  return names.length > 1 ? ['', ...names] : names
}

/** Next (+1) or previous (-1) step choice, null at the ends. */
export function neighbourStep(n: GraphNode, current: string, delta: 1 | -1): string | null {
  const choices = stepChoices(n)
  const i = choices.indexOf(current)
  if (i < 0) return choices.length > 0 ? (choices[delta === 1 ? 0 : choices.length - 1] ?? null) : null
  const next = choices[i + delta]
  return next === undefined ? null : next
}

/**
 * Whether a run counts as having failed in the selected step: the "Go to error line" target. The
 * failure names task and step; a step-less failure (timeout) only has a task.
 */
export function failureTargets(failure: Failure | null, sel: Selection, node?: GraphNode): boolean {
  if (failure === null || failure.task === '' || failure.task !== sel.task) return false
  if (failure.step !== '' && failure.step !== sel.step) return false
  // A matrix task: the failure belongs to one TaskRun, and "" means the first one.
  if (node && failure.taskRun && node.taskRunNames.includes(failure.taskRun)) return resolveTaskRun(node, sel.taskRun) === failure.taskRun
  return true
}
