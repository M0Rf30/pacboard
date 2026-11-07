// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { computed } from 'vue'
import { useRoute, useRouter, type Router } from 'vue-router'

// Writes made in the same tick, per router. route.query only updates once router.replace resolves, so
// separate replaces would each start from the same stale query and the last one would undo the others.
const batches = new WeakMap<Router, Map<string, string>>()

function enqueue(router: Router, key: string, value: string): void {
  let batch = batches.get(router)
  if (!batch) {
    const mine = new Map<string, string>()
    batch = mine
    batches.set(router, mine)
    queueMicrotask(() => {
      batches.delete(router)
      const query = { ...router.currentRoute.value.query }
      for (const [k, v] of mine) {
        if (v) query[k] = v
        else delete query[k]
      }
      void router.replace({ query })
    })
  }
  batch.set(key, value)
}

/**
 * A string ref bound to one URL query parameter (empty string removes it), so filters are shareable.
 * Writes to several refs in the same tick end up in a single `router.replace`.
 */
export function useQueryRef(key: string) {
  const route = useRoute()
  const router = useRouter()
  return computed<string>({
    get() {
      const v = route.query[key]
      return typeof v === 'string' ? v : ''
    },
    set(value) {
      enqueue(router, key, value)
    },
  })
}
