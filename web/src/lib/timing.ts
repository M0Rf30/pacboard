// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { RunBrief, Step } from '@/api/types'
import { elapsedSeconds } from './format'

/** A run counts as slow when it takes more than this multiple of the repository's typical duration. */
export const SLOW_FACTOR = 1.4

/** Smallest width of a duration bar, so a short run is still visible. */
const MIN_BAR_PERCENT = 4

/** The retained runs oldest → newest, as the history strip draws them (the API sends newest first). */
export function historyBars(recent: readonly RunBrief[], max = 20): RunBrief[] {
  return recent.slice(0, max).reverse()
}

/** Width in percent of a duration bar relative to the longest duration listed (4 … 100); 0 when unknown. */
export function durationBarPercent(seconds: number | null | undefined, longest: number): number {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds) || longest <= 0) return 0
  return Math.min(100, Math.max(MIN_BAR_PERCENT, Math.round((seconds / longest) * 100)))
}

/** The longest duration among the runs (0 when none has one). */
export function longestDuration(runs: readonly { durationSeconds: number | null }[]): number {
  let max = 0
  for (const r of runs) if (r.durationSeconds !== null && r.durationSeconds > max) max = r.durationSeconds
  return max
}

/** Percent over the typical duration when the run is slow (> 1.4×), else null. */
export function slowDelta(seconds: number | null | undefined, typical: number | null | undefined): number | null {
  if (seconds === null || seconds === undefined || !typical || typical <= 0) return null
  if (seconds <= typical * SLOW_FACTOR) return null
  return Math.round((seconds / typical - 1) * 100)
}

export interface StepBar {
  /** Offset of the step inside the task's runtime, percent. */
  left: number
  /** Share of the task's runtime, percent. */
  width: number
  seconds: number
}

/**
 * The mini Gantt of a task: where each step starts inside the task's runtime and how much of it the
 * step takes. null for a step that never started (Pending: "not run").
 */
export function stepBars(steps: readonly Step[], now: number): (StepBar | null)[] {
  let start = Infinity
  let end = -Infinity
  for (const s of steps) {
    if (!s.startTime) continue
    const a = Date.parse(s.startTime)
    if (Number.isNaN(a)) continue
    const b = s.completionTime ? Date.parse(s.completionTime) : now
    start = Math.min(start, a)
    end = Math.max(end, Number.isNaN(b) ? a : b)
  }
  const total = end - start
  return steps.map((s) => {
    if (!s.startTime || !Number.isFinite(total)) return null
    const seconds = elapsedSeconds(s.startTime, s.completionTime, now)
    const a = Date.parse(s.startTime)
    if (seconds === null || Number.isNaN(a)) return null
    if (total <= 0) return { left: 0, width: 100, seconds }
    const left = Math.min(100, Math.max(0, ((a - start) / total) * 100))
    return { left, width: Math.min(100 - left, (seconds * 1000 / total) * 100), seconds }
  })
}
