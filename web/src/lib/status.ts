// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NodeStatus, RunStatus } from '@/api/types'

export const RUN_STATUSES: RunStatus[] = ['Succeeded', 'Failed', 'Running', 'Pending', 'Cancelled', 'Unknown']

/**
 * The one place a status gets its look: the `s-*` class sets `--c` (colour token), the icon is a
 * distinct shape, the label is the word printed next to it. Badge, strip, node and step all read this,
 * so a status can never be styled inconsistently (spec §3.2, §7).
 */
export interface StatusStyle {
  cls: string
  icon: string
  label: string
}

const STYLES: Record<NodeStatus, StatusStyle> = {
  Succeeded: { cls: 's-succeeded', icon: 'succeeded', label: 'Succeeded' },
  Failed: { cls: 's-failed', icon: 'failed', label: 'Failed' },
  Running: { cls: 's-running', icon: 'running', label: 'Running' },
  Pending: { cls: 's-pending', icon: 'pending', label: 'Pending' },
  Cancelled: { cls: 's-cancelled', icon: 'cancelled', label: 'Cancelled' },
  Unknown: { cls: 's-unknown', icon: 'unknown', label: 'Unknown' },
  Skipped: { cls: 's-skipped', icon: 'skipped', label: 'Skipped' },
}

/** Style of a status; anything unexpected from the API renders as Unknown. */
export function statusStyle(status: NodeStatus): StatusStyle {
  return STYLES[status] ?? STYLES.Unknown
}
