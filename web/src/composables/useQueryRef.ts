// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

/** A string ref bound to one URL query parameter (empty string removes it), so filters are shareable. */
export function useQueryRef(key: string) {
  const route = useRoute()
  const router = useRouter()
  return computed<string>({
    get() {
      const v = route.query[key]
      return typeof v === 'string' ? v : ''
    },
    set(value) {
      const query = { ...route.query }
      if (value) query[key] = value
      else delete query[key]
      void router.replace({ query })
    },
  })
}
