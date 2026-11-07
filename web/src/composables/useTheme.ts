// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { ref } from 'vue'
import { otherTheme, parseTheme, resolveTheme, THEME_KEY, type Theme } from '@/lib/theme'

function readStored(): Theme | null {
  try {
    return parseTheme(localStorage.getItem(THEME_KEY))
  } catch {
    return null
  }
}

const media = window.matchMedia('(prefers-color-scheme: dark)')
let stored = readStored()
const theme = ref<Theme>(resolveTheme(stored, media.matches))

function apply(): void {
  document.documentElement.dataset.theme = theme.value
}
apply()

// Without a manual choice the theme follows the OS, live.
media.addEventListener('change', () => {
  if (stored) return
  theme.value = resolveTheme(null, media.matches)
  apply()
})

/** The shared theme: follows the OS until the user toggles it, then the choice is persisted. */
export function useTheme() {
  function toggle(): void {
    stored = otherTheme(theme.value)
    theme.value = stored
    try {
      localStorage.setItem(THEME_KEY, stored)
    } catch {
      // storage blocked: the choice lasts for this page view only
    }
    apply()
  }
  return { theme, toggle }
}
