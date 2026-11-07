// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NodeStatus } from '@/api/types'

const MINUTE = 60
const HOUR = 3600
const DAY = 86400

/** "45s", "3m 05s", "2h 10m", "3d 4h"; "—" when unknown. */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return '—'
  const s = Math.max(0, Math.floor(seconds))
  if (s < MINUTE) return `${s}s`
  if (s < HOUR) return `${Math.floor(s / MINUTE)}m ${String(s % MINUTE).padStart(2, '0')}s`
  if (s < DAY) return `${Math.floor(s / HOUR)}h ${String(Math.floor((s % HOUR) / MINUTE)).padStart(2, '0')}m`
  return `${Math.floor(s / DAY)}d ${Math.floor((s % DAY) / HOUR)}h`
}

/** "just now", "5m ago", "3h ago", "2d ago", "in 4m" (clock skew); "—" when unknown or invalid. */
export function relativeTime(iso: string | null | undefined, now: number): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  const diff = Math.round((now - t) / 1000)
  const abs = Math.abs(diff)
  if (abs < 5) return 'just now'
  let text: string
  if (abs < MINUTE) text = `${abs}s`
  else if (abs < HOUR) text = `${Math.floor(abs / MINUTE)}m`
  else if (abs < DAY) text = `${Math.floor(abs / HOUR)}h`
  else if (abs < 30 * DAY) text = `${Math.floor(abs / DAY)}d`
  else if (abs < 365 * DAY) text = `${Math.floor(abs / (30 * DAY))}mo`
  else text = `${Math.floor(abs / (365 * DAY))}y`
  return diff >= 0 ? `${text} ago` : `in ${text}`
}

/** Locale-formatted absolute time for tooltips; "" when unknown. */
export function absoluteTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

/**
 * Elapsed seconds between start and end (or now while there is no end). null when there is no start.
 */
export function elapsedSeconds(start: string | null | undefined, end: string | null | undefined, now: number): number | null {
  if (!start) return null
  const s = Date.parse(start)
  if (Number.isNaN(s)) return null
  const e = end ? Date.parse(end) : now
  if (Number.isNaN(e)) return null
  return Math.max(0, (e - s) / 1000)
}

/** Duration to show: the server value for finished runs, a live counter while a run is in flight. */
export function liveDuration(
  start: string | null | undefined,
  end: string | null | undefined,
  status: NodeStatus,
  serverSeconds: number | null | undefined,
  now: number,
): number | null {
  if (!end && status === 'Running') return elapsedSeconds(start, null, now)
  if (end) return elapsedSeconds(start, end, now)
  return serverSeconds ?? null
}

/** The URL normalised when it is an absolute http(s) URL with a host; "" for anything else. */
export function safeHttpUrl(raw: string | null | undefined): string {
  if (!raw) return ''
  try {
    const url = new URL(raw)
    return (url.protocol === 'http:' || url.protocol === 'https:') && url.hostname ? url.href : ''
  } catch {
    return ''
  }
}
