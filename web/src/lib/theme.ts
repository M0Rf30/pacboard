// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

export type Theme = 'light' | 'dark'

/** localStorage key of the manual choice; public/theme-init.js reads the same key before first paint. */
export const THEME_KEY = 'pacboard-theme'

/** A stored value is only trusted when it is one of the two themes. */
export function parseTheme(raw: string | null | undefined): Theme | null {
  return raw === 'light' || raw === 'dark' ? raw : null
}

/** The manual choice wins; without one the OS preference decides. */
export function resolveTheme(stored: Theme | null, prefersDark: boolean): Theme {
  return stored ?? (prefersDark ? 'dark' : 'light')
}

export function otherTheme(t: Theme): Theme {
  return t === 'dark' ? 'light' : 'dark'
}
