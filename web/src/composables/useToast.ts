// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { shallowRef } from 'vue'

export interface Toast {
  title: string
  /** Monospace line under the title (e.g. the slash command that was posted). */
  command?: string
  detail?: string
  /** External link shown under the text; ToastHost renders it through the safe-link component. */
  href?: string
  hrefLabel?: string
  error?: boolean
}

const DURATION_MS = 4000
const ERROR_DURATION_MS = 7000

const current = shallowRef<Toast | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined

/** One toast at a time (bottom right, 4 s); a new one replaces the previous. */
export function useToast() {
  function show(t: Toast): void {
    current.value = t
    clearTimeout(timer)
    timer = setTimeout(() => (current.value = null), t.error ? ERROR_DURATION_MS : DURATION_MS)
  }
  return { toast: current, show }
}
