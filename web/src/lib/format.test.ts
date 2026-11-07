// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { absoluteTime, elapsedSeconds, formatDuration, liveDuration, relativeTime, safeHttpUrl } from './format'

describe('formatDuration', () => {
  it('formats the ranges', () => {
    expect(formatDuration(0)).toBe('0s')
    expect(formatDuration(59.9)).toBe('59s')
    expect(formatDuration(60)).toBe('1m 00s')
    expect(formatDuration(185)).toBe('3m 05s')
    expect(formatDuration(3600)).toBe('1h 00m')
    expect(formatDuration(7500)).toBe('2h 05m')
    expect(formatDuration(3 * 86400 + 4 * 3600 + 120)).toBe('3d 4h')
  })

  it('handles unknown and invalid input', () => {
    expect(formatDuration(null)).toBe('—')
    expect(formatDuration(undefined)).toBe('—')
    expect(formatDuration(Number.NaN)).toBe('—')
    expect(formatDuration(-5)).toBe('0s')
  })
})

describe('relativeTime', () => {
  const now = Date.parse('2026-11-07T12:00:00Z')
  const ago = (s: number) => new Date(now - s * 1000).toISOString()

  it('formats past times', () => {
    expect(relativeTime(ago(1), now)).toBe('just now')
    expect(relativeTime(ago(30), now)).toBe('30s ago')
    expect(relativeTime(ago(5 * 60 + 10), now)).toBe('5m ago')
    expect(relativeTime(ago(3 * 3600), now)).toBe('3h ago')
    expect(relativeTime(ago(2 * 86400), now)).toBe('2d ago')
    expect(relativeTime(ago(65 * 86400), now)).toBe('2mo ago')
    expect(relativeTime(ago(800 * 86400), now)).toBe('2y ago')
  })

  it('formats clock skew as future', () => {
    expect(relativeTime(ago(-120), now)).toBe('in 2m')
  })

  it('handles unknown and invalid input', () => {
    expect(relativeTime(null, now)).toBe('—')
    expect(relativeTime('', now)).toBe('—')
    expect(relativeTime('not a date', now)).toBe('—')
  })
})

describe('absoluteTime', () => {
  it('is empty for unknown input', () => {
    expect(absoluteTime(null)).toBe('')
    expect(absoluteTime('nope')).toBe('')
    expect(absoluteTime('2026-11-07T12:00:00Z')).not.toBe('')
  })
})

describe('elapsed and live durations', () => {
  const now = Date.parse('2026-11-07T12:00:00Z')

  it('computes elapsed from start to end or now', () => {
    expect(elapsedSeconds('2026-11-07T11:59:00Z', '2026-11-07T11:59:30Z', now)).toBe(30)
    expect(elapsedSeconds('2026-11-07T11:59:00Z', null, now)).toBe(60)
    expect(elapsedSeconds(null, null, now)).toBeNull()
    expect(elapsedSeconds('garbage', null, now)).toBeNull()
  })

  it('counts live while running and uses the server value otherwise', () => {
    expect(liveDuration('2026-11-07T11:58:00Z', null, 'Running', 5, now)).toBe(120)
    expect(liveDuration('2026-11-07T11:58:00Z', '2026-11-07T11:59:00Z', 'Succeeded', 999, now)).toBe(60)
    expect(liveDuration(null, null, 'Pending', null, now)).toBeNull()
    expect(liveDuration(null, null, 'Skipped', 7, now)).toBe(7)
  })
})

describe('safeHttpUrl', () => {
  it('keeps absolute http(s) URLs', () => {
    expect(safeHttpUrl('https://github.com/zextras/carbonio-proxy/pull/323')).toBe('https://github.com/zextras/carbonio-proxy/pull/323')
    expect(safeHttpUrl('http://ghe.example.com/o/r')).toBe('http://ghe.example.com/o/r')
  })

  it('drops other schemes, relative and malformed values', () => {
    for (const bad of ['javascript:alert(1)', ' JaVaScRiPt:alert(1)', 'data:text/html,x', '//evil.example', '/pull/1', 'not a url', '', null, undefined]) {
      expect(safeHttpUrl(bad)).toBe('')
    }
  })
})
