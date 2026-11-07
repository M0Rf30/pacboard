// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { onScopeDispose, ref } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import type { ChangeEvent } from '@/api/types'

const RECONNECT_MS = 5000

/**
 * Subscribes to the backend change stream and calls `refresh` (debounced) for every event accepted by
 * `match`. A refetch also runs after a reconnect, since events may have been missed meanwhile.
 * Returns whether the stream is currently connected.
 */
export function useLive(refresh: () => void, match: (e: ChangeEvent) => boolean = () => true) {
  const live = ref(false)
  const debounced = useDebounceFn(refresh, 400, { maxWait: 2000 })
  let source: EventSource | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let dropped = false
  let stopped = false

  function connect(): void {
    source = new EventSource('/api/v1/events')
    source.onopen = () => {
      live.value = true
      if (dropped) {
        dropped = false
        void debounced()
      }
    }
    source.addEventListener('change', (ev) => {
      try {
        const e = JSON.parse((ev as MessageEvent<string>).data) as ChangeEvent
        if (match(e)) void debounced()
      } catch {
        // malformed event: ignore
      }
    })
    source.onerror = () => {
      live.value = false
      dropped = true
      // The browser retries by itself unless the server answered with an error status.
      if (source?.readyState === EventSource.CLOSED && !stopped) {
        source.close()
        retry = setTimeout(connect, RECONNECT_MS)
      }
    }
  }

  connect()
  onScopeDispose(() => {
    stopped = true
    if (retry) clearTimeout(retry)
    source?.close()
  })

  return live
}
