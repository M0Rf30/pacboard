// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NodeStatus, RunStatus } from '@/api/types'

export const RUN_STATUSES: RunStatus[] = ['Succeeded', 'Failed', 'Running', 'Pending', 'Cancelled', 'Unknown']

/** Tailwind classes of the status pill (background, text, border), light + dark. */
export function statusClasses(status: NodeStatus): string {
  switch (status) {
    case 'Succeeded':
      return 'bg-green-100 text-green-800 border-green-600/30 dark:bg-green-950 dark:text-green-300 dark:border-green-500/40'
    case 'Failed':
      return 'bg-red-100 text-red-800 border-red-600/30 dark:bg-red-950 dark:text-red-300 dark:border-red-500/40'
    case 'Running':
      return 'bg-blue-100 text-blue-800 border-blue-600/30 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-500/40'
    case 'Cancelled':
      return 'bg-orange-100 text-orange-800 border-orange-600/30 dark:bg-orange-950 dark:text-orange-300 dark:border-orange-500/40'
    case 'Skipped':
      return 'bg-transparent text-gray-500 border-dashed border-gray-400 dark:text-gray-400 dark:border-gray-500'
    default:
      return 'bg-gray-100 text-gray-700 border-gray-500/30 dark:bg-gray-800 dark:text-gray-300 dark:border-gray-500/40'
  }
}

/** Glyph for the status so it is never conveyed by colour alone. */
export function statusGlyph(status: NodeStatus): string {
  switch (status) {
    case 'Succeeded':
      return '✓'
    case 'Failed':
      return '✕'
    case 'Running':
      return '●'
    case 'Cancelled':
      return '⊘'
    case 'Skipped':
      return '↷'
    case 'Pending':
      return '○'
    default:
      return '?'
  }
}
