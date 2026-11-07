// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { onScopeDispose, ref, shallowRef, watch, type WatchSource } from 'vue'
import { ApiError } from '@/api/client'

/**
 * Loads data with a fetcher. The previous data stays on screen while a refresh is in flight, and a
 * slower older response never overwrites a newer one. Re-fetches when any watched source changes.
 */
export function useResource<T>(fetcher: (signal: AbortSignal) => Promise<T>, sources: WatchSource[] = []) {
  const data = shallowRef<T>()
  const error = ref('')
  const errorStatus = ref(0)
  const loading = ref(false)
  let controller: AbortController | undefined

  async function refresh(): Promise<void> {
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
    } catch (e) {
      if (mine.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return
      error.value = e instanceof Error ? e.message : String(e)
      errorStatus.value = e instanceof ApiError ? e.status : 0
    } finally {
      if (controller === mine) loading.value = false
    }
  }

  watch(sources, () => void refresh(), { immediate: true })
  onScopeDispose(() => controller?.abort())

  return { data, error, errorStatus, loading, refresh }
}
