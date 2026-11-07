// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { LogParser, lineText, type LogLine } from './logparse'
import {
  closedGroups,
  errorTargetIndex,
  filterLines,
  firstErrorIndex,
  focusMatches,
  hiddenLineCount,
  lineMatches,
  visibleFrom,
  visibleTail,
} from './logview'

function parseLog(text: string): LogLine[] {
  const p = new LogParser()
  p.push(text)
  p.flush()
  return p.lines
}

const failing = parseLog(['::group::setup', 'ok', '::endgroup::', '::group::lint', 'running', '::error file=a.go,line=3::bad', '::endgroup::', 'done'].join('\n'))
const quiet = parseLog(['::group::one', 'a', '::endgroup::', '::group::two', 'b', '::endgroup::'].join('\n'))

describe('folded groups', () => {
  const p = new LogParser()
  p.push(['before', '::group::Install', 'a', '::group::Nested', 'b', '::endgroup::', 'c', '::endgroup::', 'after'].join('\n'))
  p.flush()
  const closedOf = (...ids: number[]) => closedGroups(p.groups, (id) => !ids.includes(id))

  it('hides the closed group with its children only', () => {
    expect(visibleTail(p.lines, closedOf(1), 100).map(lineText)).toEqual(['before', 'Install', 'a', 'Nested', 'c', 'after'])
    expect(visibleTail(p.lines, closedOf(0), 100).map(lineText)).toEqual(['before', 'Install', 'after'])
    expect(visibleTail(p.lines, closedOf(), 100)).toHaveLength(p.lines.length)
  })

  it('limits the window to the last visible lines', () => {
    expect(visibleTail(p.lines, closedOf(0), 2).map(lineText)).toEqual(['Install', 'after'])
  })

  it('counts hidden lines once, nested closed groups included', () => {
    const count = (id: number) => p.groupLines(id)
    expect(hiddenLineCount(p.groups, closedOf(1), count)).toBe(1)
    expect(hiddenLineCount(p.groups, closedOf(0), count)).toBe(4)
    expect(hiddenLineCount(p.groups, closedOf(0, 1), count)).toBe(4)
    expect(hiddenLineCount(p.groups, closedOf(), count)).toBe(0)
  })

  it('counts the visible lines from a given line', () => {
    const a = p.lines.find((l) => lineText(l) === 'a')!
    expect(visibleFrom(p.lines, closedOf(1), a)).toBe(4)
    expect(visibleFrom(p.lines, closedOf(), a)).toBe(5)
  })
})

describe('error target', () => {
  it('finds the first error annotation', () => {
    expect(firstErrorIndex(failing)).toBe(failing.findIndex((l) => l.level === 'error'))
    expect(firstErrorIndex(quiet)).toBe(-1)
  })

  it('prefers the API line number, which counts raw lines', () => {
    const idx = errorTargetIndex(failing, 5)
    expect(failing[idx]!.no).toBe(5)
    expect(failing[idx]!.level).toBe('')
  })

  it('falls back to the first error when the line is unknown or absent', () => {
    expect(failing[errorTargetIndex(failing, null)]!.level).toBe('error')
    expect(failing[errorTargetIndex(failing, 999)]!.level).toBe('error')
    expect(errorTargetIndex(quiet, null)).toBe(-1)
  })
})

describe('filtering', () => {
  it('matches text and the annotation file, ignoring case', () => {
    const [line] = parseLog('::error file=Main.go,line=2::Boom')
    expect(lineMatches(line!, 'boom')).toBe(true)
    expect(lineMatches(line!, 'main.go:2')).toBe(true)
    expect(lineMatches(line!, 'nope')).toBe(false)
    expect(lineMatches(line!, '  ')).toBe(true)
  })

  it('lists the matching lines flat, groups included only when their title matches', () => {
    expect(filterLines(failing, 'lint', false).map((l) => l.kind)).toEqual(['group'])
    expect(filterLines(failing, 'run', false).map((l) => l.segments[0]!.text)).toEqual(['running'])
  })

  it('keeps errors and warnings when showing annotations only', () => {
    const lines = parseLog('x\n::warning::w\n::notice::n\n::error::e')
    expect(filterLines(lines, '', true).map((l) => l.level)).toEqual(['warning', 'error'])
    expect(filterLines(lines, 'e', true).map((l) => l.level)).toEqual(['error'])
    expect(filterLines(lines, '', false)).toHaveLength(4)
  })
})

describe('focusMatches', () => {
  const req = { seq: 1, task: 'lint', taskRun: 'run-lint', step: 'go', line: 4, error: true }
  it('matches only the log the request names', () => {
    expect(focusMatches(req, { task: 'lint', taskRun: 'run-lint', step: 'go' })).toBe(true)
    expect(focusMatches(req, { task: 'build', taskRun: 'run-lint', step: 'go' })).toBe(false)
    expect(focusMatches(req, { task: 'lint', taskRun: 'run-lint-2', step: 'go' })).toBe(false)
    expect(focusMatches(req, { task: 'lint', taskRun: 'run-lint', step: '' })).toBe(false)
  })
})
