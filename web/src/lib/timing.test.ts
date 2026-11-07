// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import type { RunBrief, Step } from '@/api/types'
import { durationBarPercent, historyBars, longestDuration, slowDelta, stepBars } from './timing'

const brief = (name: string): RunBrief => ({ name, status: 'Succeeded', startTime: null, durationSeconds: null })

describe('historyBars', () => {
  it('reverses the newest-first list to oldest → newest and caps it', () => {
    expect(historyBars([brief('c'), brief('b'), brief('a')]).map((b) => b.name)).toEqual(['a', 'b', 'c'])
    expect(historyBars([brief('c'), brief('b'), brief('a')], 2).map((b) => b.name)).toEqual(['b', 'c'])
    expect(historyBars([])).toEqual([])
  })
})

describe('durationBarPercent', () => {
  it('scales to the longest run with a visible minimum', () => {
    expect(durationBarPercent(50, 100)).toBe(50)
    expect(durationBarPercent(100, 100)).toBe(100)
    expect(durationBarPercent(1, 100)).toBe(4)
  })

  it('is empty when the duration or the maximum is unknown', () => {
    expect(durationBarPercent(null, 100)).toBe(0)
    expect(durationBarPercent(10, 0)).toBe(0)
    expect(durationBarPercent(Number.NaN, 10)).toBe(0)
  })
})

describe('longestDuration', () => {
  it('ignores runs without a duration', () => {
    expect(longestDuration([{ durationSeconds: 3 }, { durationSeconds: null }, { durationSeconds: 9 }])).toBe(9)
    expect(longestDuration([])).toBe(0)
  })
})

describe('slowDelta', () => {
  it('is the percent over typical beyond 1.4x', () => {
    expect(slowDelta(150, 100)).toBe(50)
    expect(slowDelta(200, 100)).toBe(100)
  })

  it('is null for a normal run or when typical is unknown', () => {
    expect(slowDelta(140, 100)).toBeNull()
    expect(slowDelta(90, 100)).toBeNull()
    expect(slowDelta(500, null)).toBeNull()
    expect(slowDelta(null, 100)).toBeNull()
    expect(slowDelta(500, 0)).toBeNull()
  })
})

describe('stepBars', () => {
  const t = (s: number) => new Date(Date.UTC(2025, 0, 1, 0, 0, s)).toISOString()
  const step = (name: string, start: number | null, end: number | null, status: Step['status'] = 'Succeeded'): Step => ({
    name,
    status,
    exitCode: null,
    reason: '',
    startTime: start === null ? null : t(start),
    completionTime: end === null ? null : t(end),
  })

  it('places each step inside the task runtime', () => {
    const bars = stepBars([step('a', 0, 10), step('b', 10, 40)], Date.parse(t(100)))
    expect(bars[0]).toEqual({ left: 0, width: 25, seconds: 10 })
    expect(bars[1]).toEqual({ left: 25, width: 75, seconds: 30 })
  })

  it('has no bar for a step that never started', () => {
    expect(stepBars([step('a', 0, 10), step('b', null, null, 'Pending')], 0)[1]).toBeNull()
  })

  it('runs a running step up to now', () => {
    const bars = stepBars([step('a', 0, 10), step('b', 10, null, 'Running')], Date.parse(t(20)))
    expect(bars[1]).toEqual({ left: 50, width: 50, seconds: 10 })
  })

  it('fills the bar when everything happens in one instant', () => {
    expect(stepBars([step('a', 5, 5)], 0)[0]).toEqual({ left: 0, width: 100, seconds: 0 })
  })
})
