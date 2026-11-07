// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import type { Failure, GraphNode, Step } from '@/api/types'
import { defaultStep, failureTargets, landingSelection, lastExecuted, neighbourStep, pickTask, stepChoices } from './runview'

const step = (name: string, status: Step['status'] = 'Succeeded'): Step => ({
  name,
  status,
  exitCode: status === 'Failed' ? 1 : null,
  reason: '',
  startTime: null,
  completionTime: null,
})

function node(id: string, status: GraphNode['status'], steps: Step[], extra: Partial<GraphNode> = {}): GraphNode {
  return {
    id,
    taskRunName: `${id}-0`,
    status,
    reason: '',
    finally: false,
    matrixCount: 1,
    taskRunNames: [`${id}-0`],
    startTime: null,
    completionTime: null,
    steps,
    ...extra,
  }
}

const failure = (extra: Partial<Failure> = {}): Failure => ({
  task: 'lint',
  taskRun: '',
  step: 'run',
  exitCode: 1,
  message: 'bad',
  line: 4,
  skippedBecause: 2,
  ...extra,
})

const nodes = [
  node('clone', 'Succeeded', [step('clone')], { startTime: '2025-01-01T00:00:00Z' }),
  node('lint', 'Failed', [step('setup'), step('run', 'Failed'), step('post', 'Pending')], { startTime: '2025-01-01T00:01:00Z' }),
  node('deploy', 'Skipped', []),
]

describe('defaultStep / pickTask', () => {
  it('prefers the failed step, then the running one, then the only one', () => {
    expect(defaultStep(nodes[1]!)).toBe('run')
    expect(defaultStep(node('t', 'Running', [step('a'), step('b', 'Running')]))).toBe('b')
    expect(defaultStep(nodes[0]!)).toBe('clone')
  })

  it('opens a quiet multi-step task on all steps', () => {
    expect(defaultStep(node('t', 'Succeeded', [step('a'), step('b')]))).toBe('')
  })

  it('selects a task with its step and no matrix TaskRun', () => {
    expect(pickTask(nodes[1]!)).toEqual({ task: 'lint', step: 'run', taskRun: '' })
  })
})

describe('lastExecuted', () => {
  it('skips skipped and pending tasks and takes the latest start', () => {
    expect(lastExecuted(nodes)?.id).toBe('lint')
    expect(lastExecuted([node('a', 'Skipped', []), node('b', 'Pending', [])])).toBeUndefined()
  })

  it('lets the API order break ties between tasks without a start time', () => {
    expect(lastExecuted([node('a', 'Succeeded', []), node('b', 'Succeeded', [])])?.id).toBe('b')
  })
})

describe('landingSelection', () => {
  it('lands a failed run on the failure task and step', () => {
    expect(landingSelection(nodes, 'Failed', failure())).toEqual({ task: 'lint', step: 'run', taskRun: '' })
  })

  it('falls back to the first failed step when the failure names an unknown step', () => {
    expect(landingSelection(nodes, 'Failed', failure({ step: 'nope' })).step).toBe('run')
  })

  it('falls back to a failed node when the failure is missing', () => {
    expect(landingSelection(nodes, 'Failed', null)).toEqual({ task: 'lint', step: 'run', taskRun: '' })
  })

  it('keeps the matrix TaskRun of the failure', () => {
    const m = [node('lint', 'Failed', [step('run', 'Failed')], { taskRunNames: ['lint-0', 'lint-1'], matrixCount: 2 })]
    expect(landingSelection(m, 'Failed', failure({ taskRun: 'lint-1' })).taskRun).toBe('lint-1')
    expect(landingSelection(m, 'Failed', failure({ taskRun: 'ghost' })).taskRun).toBe('')
  })

  it('lands anything else on the last executed task', () => {
    expect(landingSelection(nodes, 'Succeeded', null).task).toBe('lint')
    expect(landingSelection([node('a', 'Skipped', [])], 'Succeeded', null)).toEqual({ task: '', step: '', taskRun: '' })
  })

  it('uses the last executed task for a failed run without any failed task (timeout)', () => {
    const ok = [node('clone', 'Succeeded', [step('clone')])]
    expect(landingSelection(ok, 'Failed', failure({ task: '' })).task).toBe('clone')
  })
})

describe('step navigation', () => {
  const n = nodes[1]!

  it('puts "all steps" first for multi-step tasks only', () => {
    expect(stepChoices(n)).toEqual(['', 'setup', 'run', 'post'])
    expect(stepChoices(nodes[0]!)).toEqual(['clone'])
    expect(stepChoices(nodes[2]!)).toEqual([])
  })

  it('moves and stops at the ends', () => {
    expect(neighbourStep(n, '', 1)).toBe('setup')
    expect(neighbourStep(n, 'run', 1)).toBe('post')
    expect(neighbourStep(n, 'post', 1)).toBeNull()
    expect(neighbourStep(n, 'setup', -1)).toBe('')
    expect(neighbourStep(n, '', -1)).toBeNull()
  })

  it('enters the list from the matching end when nothing valid is selected', () => {
    expect(neighbourStep(n, 'ghost', 1)).toBe('')
    expect(neighbourStep(n, 'ghost', -1)).toBe('post')
    expect(neighbourStep(nodes[2]!, '', 1)).toBeNull()
  })
})

describe('failureTargets', () => {
  it('is true only for the failure task and step', () => {
    expect(failureTargets(failure(), { task: 'lint', step: 'run', taskRun: '' })).toBe(true)
    expect(failureTargets(failure(), { task: 'lint', step: 'setup', taskRun: '' })).toBe(false)
    expect(failureTargets(failure(), { task: 'clone', step: 'run', taskRun: '' })).toBe(false)
    expect(failureTargets(failure({ step: '' }), { task: 'lint', step: 'setup', taskRun: '' })).toBe(true)
    expect(failureTargets(failure({ task: '' }), { task: '', step: '', taskRun: '' })).toBe(false)
    expect(failureTargets(null, { task: 'lint', step: 'run', taskRun: '' })).toBe(false)
  })
})

describe('matrix failure TaskRun', () => {
  const m = node('test', 'Failed', [step('run', 'Failed')], { taskRunNames: ['test-0', 'test-1'], taskRunName: 'test-0', matrixCount: 2 })
  const f = failure({ task: 'test', taskRun: 'test-1' })
  it('picks the failing TaskRun and only targets it', () => {
    expect(pickTask(m, f)).toEqual({ task: 'test', step: 'run', taskRun: 'test-1' })
    expect(pickTask(m)).toEqual({ task: 'test', step: 'run', taskRun: '' })
    expect(failureTargets(f, { task: 'test', step: 'run', taskRun: 'test-1' }, m)).toBe(true)
    expect(failureTargets(f, { task: 'test', step: 'run', taskRun: '' }, m)).toBe(false)
    expect(failureTargets(f, { task: 'test', step: 'run', taskRun: 'test-0' }, m)).toBe(false)
    expect(failureTargets(failure({ task: 'test', taskRun: 'test-0' }), { task: 'test', step: 'run', taskRun: '' }, m)).toBe(true)
  })
})
