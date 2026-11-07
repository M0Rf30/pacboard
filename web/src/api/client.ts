// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type {
  Me,
  RepositorySummary,
  RerunMode,
  RerunResult,
  RunDetail,
  RunStatus,
  RunSummary,
} from './types'

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const CSRF_COOKIE = 'pacboard_csrf'
const CSRF_HEADER = 'X-Pacboard-CSRF'

type UnauthorizedHandler = () => void

let onUnauthorized: UnauthorizedHandler = () => {}

/** Registers what happens when an API call answers 401 (the app redirects to the login). */
export function setUnauthorizedHandler(h: UnauthorizedHandler): void {
  onUnauthorized = h
}

/** Reads the CSRF token the backend stores in a readable cookie. */
export function csrfToken(cookies: string = document.cookie): string {
  for (const part of cookies.split(';')) {
    const eq = part.indexOf('=')
    if (eq < 0) continue
    if (part.slice(0, eq).trim() === CSRF_COOKIE) {
      try {
        return decodeURIComponent(part.slice(eq + 1).trim())
      } catch {
        return part.slice(eq + 1).trim()
      }
    }
  }
  return ''
}

export function seg(s: string): string {
  return encodeURIComponent(s)
}

/** Turns a failed response into an ApiError carrying the backend's `{"error": ...}` message. */
export async function errorFrom(res: Response): Promise<ApiError> {
  let message = res.statusText || `HTTP ${res.status}`
  try {
    const body = (await res.json()) as { error?: unknown }
    if (typeof body.error === 'string' && body.error) message = body.error
  } catch {
    // not JSON: keep the status text
  }
  return new ApiError(res.status, message)
}

/** Checks a response: 401 triggers the unauthorized handler, any non-2xx throws. */
export async function check(res: Response): Promise<Response> {
  if (res.ok) return res
  if (res.status === 401) onUnauthorized()
  throw await errorFrom(res)
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await check(await fetch(path, { credentials: 'same-origin', ...init }))
  return (await res.json()) as T
}

function qs(params: Record<string, string | number | null | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

// A backend that predates the history/failure fields still renders: missing means empty or null.
function withRepoDefaults(r: RepositorySummary): RepositorySummary {
  return { ...r, recent: r.recent ?? [], typicalDurationSeconds: r.typicalDurationSeconds ?? null, concurrencyLimit: r.concurrencyLimit ?? null, intermittent: r.intermittent ?? false }
}

function withRunDefaults(d: RunDetail): RunDetail {
  return { ...d, typicalDurationSeconds: d.typicalDurationSeconds ?? null, failure: d.failure ?? null }
}

export interface RepoRunsQuery {
  limit?: number
  pr?: number | null
  branch?: string
  status?: RunStatus | ''
}

export const api = {
  me: (signal?: AbortSignal) => request<Me>('/api/v1/me', { signal }),

  repositories: async (signal?: AbortSignal) =>
    (await request<RepositorySummary[]>('/api/v1/repositories', { signal })).map(withRepoDefaults),

  repoRuns: (namespace: string, name: string, q: RepoRunsQuery = {}, signal?: AbortSignal) =>
    request<RunSummary[]>(
      `/api/v1/repositories/${seg(namespace)}/${seg(name)}/runs${qs({ limit: q.limit ?? 50, pr: q.pr, branch: q.branch, status: q.status })}`,
      { signal },
    ),

  runs: (q: { limit?: number; status?: RunStatus | '' } = {}, signal?: AbortSignal) =>
    request<RunSummary[]>(`/api/v1/runs${qs({ limit: q.limit ?? 100, status: q.status })}`, { signal }),

  run: async (namespace: string, name: string, signal?: AbortSignal) =>
    withRunDefaults(await request<RunDetail>(`/api/v1/runs/${seg(namespace)}/${seg(name)}`, { signal })),

  rerun: (namespace: string, name: string, mode: RerunMode) =>
    request<RerunResult>(`/api/v1/runs/${seg(namespace)}/${seg(name)}/rerun`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', [CSRF_HEADER]: csrfToken() },
      body: JSON.stringify({ mode }),
    }),

  async logout(): Promise<void> {
    await check(
      await fetch('/auth/logout', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { [CSRF_HEADER]: csrfToken() },
      }),
    )
  },
}

export interface LogQuery {
  namespace: string
  run: string
  task: string
  taskRun?: string
  step?: string
  follow?: boolean
}

export function logsUrl(q: LogQuery): string {
  return (
    `/api/v1/runs/${seg(q.namespace)}/${seg(q.run)}/tasks/${seg(q.task)}/logs` +
    qs({ taskRun: q.taskRun, step: q.step, follow: q.follow ? 1 : undefined })
  )
}

export function loginUrl(next: string): string {
  return `/auth/login${qs({ next })}`
}

/**
 * Streams a log response, calling onText for every decoded chunk. Resolves when the stream ends;
 * throws ApiError for a non-2xx status (404 = logs gone). Aborting via the signal rejects with AbortError.
 */
export async function streamLogs(url: string, signal: AbortSignal, onText: (text: string) => void): Promise<void> {
  const res = await check(await fetch(url, { credentials: 'same-origin', signal }))
  if (!res.body) {
    onText(await res.text())
    return
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder('utf-8')
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    const text = decoder.decode(value, { stream: true })
    if (text) onText(text)
  }
  const rest = decoder.decode()
  if (rest) onText(rest)
}
