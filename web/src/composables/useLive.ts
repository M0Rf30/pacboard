// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { onScopeDispose, ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { api, ApiError } from '@/api/client'
import type { ChangeEvent } from '@/api/types'

const RECONNECT_BASE_MS = 1000
const RECONNECT_MAX_MS = 30_000

/**
 * Subscribes to the backend change stream and calls `refresh` (debounced) for every event accepted by
 * `match`. A refetch also runs after every successful open that follows an error (a reconnect, or the
 * first connect once the backend finished starting), since events may have been missed meanwhile.
 * When the browser gives up on the stream (a non-200 answer), `/api/v1/me` tells an expired session
 * (401: the unauthorized handler runs and retrying stops) from a backend that is merely unavailable
 * (retry with exponential backoff).
 * `connected` is the stream state; `paused` is the user's toggle: while paused events are dropped, and
 * resuming refetches once (elapsed-time ticks are not affected, they are local).
 */
export function useLive(refresh: () => void, match: (e: ChangeEvent) => boolean = () => true) {
  const connected = ref(false)
  const paused = ref(false)
  // A debounced call that is still pending when the scope ends must not refetch for a dead consumer.
  const debounced = useDebounceFn(
    () => {
      if (!stopped) refresh()
    },
    400,
    { maxWait: 2000 },
  )
  const probe = new AbortController()
  let source: EventSource | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let dropped = false
  let stopped = false
  let failures = 0

  /** The stream is closed for good: find out why, then reconnect unless the session is gone. */
  async function recover(): Promise<void> {
    try {
      await api.me(probe.signal)
    } catch (e) {
      // check() already ran the unauthorized handler (login redirect).
      if (e instanceof ApiError && e.status === 401) return
    }
    if (stopped) return
    const delay = Math.min(RECONNECT_BASE_MS * 2 ** failures, RECONNECT_MAX_MS)
    failures++
    retry = setTimeout(connect, delay)
  }

  function connect(): void {
    source = new EventSource('/api/v1/events')
    source.onopen = () => {
      connected.value = true
      failures = 0
      if (dropped) {
        dropped = false
        if (!paused.value) void debounced()
      }
    }
    source.addEventListener('change', (ev) => {
      if (paused.value) return
      try {
        const e = JSON.parse((ev as MessageEvent<string>).data) as ChangeEvent
        if (match(e)) void debounced()
      } catch {
        // malformed event: ignore
      }
    })
    source.onerror = () => {
      connected.value = false
      dropped = true
      // The browser retries by itself unless the server answered with an error status.
      if (source?.readyState === EventSource.CLOSED && !stopped) {
        source.close()
        void recover()
      }
    }
  }

  watch(paused, (p) => {
    if (!p) void debounced()
  })

  connect()
  onScopeDispose(() => {
    stopped = true
    probe.abort()
    clearTimeout(retry)
    source?.close()
  })

  return { connected, paused }
}
