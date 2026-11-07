// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { formatLocation, lineText, type LogLine } from './logparse'

/** Ask the viewer to reveal a line of one specific log once the log has it: the error line, or a given line number. */
export interface FocusRequest {
  /** Changes for every request, so asking twice for the same line scrolls twice. */
  seq: number
  /** The log the line belongs to; the viewer ignores the request while it shows another one. */
  task: string
  /** Resolved TaskRun name (never the "" default). */
  taskRun: string
  /** Step name, "" for all steps. */
  step: string
  /** 1-based raw line number, or null. */
  line: number | null
  /** True: go to the error (`line` when the log has it, else the first `::error::`). */
  error: boolean
}

/** Whether a request targets the log the viewer currently shows. `taskRun` is the resolved name. */
export function focusMatches(f: FocusRequest, shown: { task: string; taskRun: string; step: string }): boolean {
  return f.task === shown.task && f.taskRun === shown.taskRun && f.step === shown.step
}

function hiddenBy(l: LogLine, closed: ReadonlySet<number>): boolean {
  return closed.size > 0 && l.path.some((id) => closed.has(id))
}

/** Groups the viewer shows folded. */
export function closedGroups(groups: ReadonlyMap<number, LogLine>, isOpen: (id: number) => boolean): Set<number> {
  const closed = new Set<number>()
  for (const id of groups.keys()) if (!isOpen(id)) closed.add(id)
  return closed
}

/**
 * Number of lines hidden inside closed groups. Counts each outermost closed group once (its count
 * already includes the nested groups), so the cost follows the number of groups, not of lines.
 */
export function hiddenLineCount(
  groups: ReadonlyMap<number, LogLine>,
  closed: ReadonlySet<number>,
  linesIn: (id: number) => number,
): number {
  let n = 0
  for (const id of closed) {
    const header = groups.get(id)
    if (header && !hiddenBy(header, closed)) n += linesIn(id)
  }
  return n
}

/**
 * The last `limit` lines that are not hidden inside a closed group, in order. Walks back from the
 * end, so the cost is the window plus the hidden lines it skips, not the whole log.
 */
export function visibleTail(lines: readonly LogLine[], closed: ReadonlySet<number>, limit: number): LogLine[] {
  const out: LogLine[] = []
  for (let i = lines.length - 1; i >= 0 && out.length < limit; i--) {
    const l = lines[i]!
    if (!hiddenBy(l, closed)) out.push(l)
  }
  return out.reverse()
}

/** How many visible lines there are from `line` (inclusive) to the end; 0 when the line is not retained. */
export function visibleFrom(lines: readonly LogLine[], closed: ReadonlySet<number>, line: LogLine): number {
  const first = lines[0]
  if (!first) return 0
  const at = line.id - first.id
  if (at < 0 || lines[at] !== line) return 0
  let n = 0
  for (let i = lines.length - 1; i >= at; i--) if (!hiddenBy(lines[i]!, closed)) n++
  return n
}

/** Index of the first error annotation, or -1. */
export function firstErrorIndex(lines: readonly LogLine[]): number {
  return lines.findIndex((l) => l.level === 'error')
}

/**
 * Where "Go to error line" lands: the line carrying the API's 1-based `line` number when the log has
 * it, else the first error annotation. -1 when neither exists (nothing to jump to).
 */
export function errorTargetIndex(lines: readonly LogLine[], line: number | null): number {
  if (line !== null) {
    const exact = lines.findIndex((l) => l.no === line)
    if (exact >= 0) return exact
  }
  return firstErrorIndex(lines)
}

/** Case-insensitive match of the query on the line's text (and an annotation's file location). */
export function lineMatches(l: LogLine, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (q === '') return true
  return `${lineText(l)} ${formatLocation(l.location)}`.toLowerCase().includes(q)
}

/**
 * The flat list shown while filtering: groups are flattened away (their lines are listed on their own),
 * a group header stays only if its title matches. `annotationsOnly` keeps errors and warnings.
 */
export function filterLines(lines: readonly LogLine[], query: string, annotationsOnly: boolean): LogLine[] {
  return lines.filter((l) => {
    if (annotationsOnly && l.level !== 'error' && l.level !== 'warning') return false
    return lineMatches(l, query)
  })
}
