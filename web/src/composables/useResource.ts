// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { onScopeDispose, ref, shallowRef, watch, type WatchSource } from 'vue'
import { ApiError } from '@/api/client'

/** Waits before the 1st, 2nd, ... retry; the last value repeats. */
const RETRY_DELAYS_MS = [1000, 2000, 4000, 5000]

/**
 * Whether a failed load is worth repeating by itself: the backend still starting (503, informers not
 * synced) or the network failing (fetch rejects with a TypeError). 401, 403, 404 and every other
 * answer is final.
 */
function retryable(e: unknown): boolean {
  if (e instanceof ApiError) return e.status === 503
  return e instanceof TypeError
}

/**
 * Loads data with a fetcher. The previous data stays on screen while a refresh is in flight, and a
 * slower older response never overwrites a newer one. Re-fetches when any watched source changes.
 * A 503 or network failure is retried with backoff (1 s, 2 s, 4 s, then every 5 s) until it succeeds,
 * the next refresh starts or the scope ends.
 */
export function useResource<T>(fetcher: (signal: AbortSignal) => Promise<T>, sources: WatchSource[] = []) {
  const data = shallowRef<T>()
  const error = ref('')
  const errorStatus = ref(0)
  const loading = ref(false)
  let controller: AbortController | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let attempt = 0

  function cancelRetry(): void {
    clearTimeout(retryTimer)
    retryTimer = undefined
  }

  async function refresh(): Promise<void> {
    cancelRetry()
    attempt = 0
    await load()
  }

  async function load(): Promise<void> {
    controller?.abort()
    const mine = new AbortController()
    controller = mine
    loading.value = true
    try {
      const result = await fetcher(mine.signal)
      if (mine.signal.aborted) return
      data.value = result
      error.value = ''
      errorStatus.value = 0
      attempt = 0
    } catch (e) {
      if (mine.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return
      error.value = e instanceof Error ? e.message : String(e)
      errorStatus.value = e instanceof ApiError ? e.status : 0
      if (retryable(e)) {
        const delay = RETRY_DELAYS_MS[Math.min(attempt, RETRY_DELAYS_MS.length - 1)]!
        attempt++
        retryTimer = setTimeout(() => void load(), delay)
      }
    } finally {
      if (controller === mine) loading.value = false
    }
  }

  watch(sources, () => void refresh(), { immediate: true })
  onScopeDispose(() => {
    cancelRetry()
    controller?.abort()
  })

  return { data, error, errorStatus, loading, refresh }
}
