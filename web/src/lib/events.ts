// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Pipelines-as-Code event identifiers (`retest-all-comment`) as the words a developer would use.

const KNOWN: Record<string, string> = {
  pull_request: 'Pull request',
  push: 'Push',
  incoming: 'Incoming webhook',
  'test-comment': '/test',
  'test-all-comment': '/test all',
  'retest-comment': '/retest',
  'retest-all-comment': '/retest all',
  'cancel-comment': '/cancel',
  'cancel-all-comment': '/cancel all',
  'ok-to-test-comment': '/ok-to-test',
}

const COMMENT = '-comment'

/** Whether the event came from a slash command in a PR comment (shown in monospace). */
export function isCommandEvent(event: string): boolean {
  return event.endsWith(COMMENT)
}

/**
 * Readable label of an event identifier. Slash-command events show the command; other identifiers
 * lose their separators and gain a capital. The raw identifier belongs in a `title` attribute.
 */
export function eventLabel(event: string): string {
  const known = KNOWN[event]
  if (known !== undefined) return known
  if (isCommandEvent(event)) {
    const base = event.slice(0, -COMMENT.length)
    const all = base.endsWith('-all')
    const name = all ? base.slice(0, -'-all'.length) : base
    return name ? `/${name}${all ? ' all' : ''}` : '/'
  }
  const words = event.replace(/[_-]+/g, ' ').trim()
  return words ? words[0]!.toUpperCase() + words.slice(1) : ''
}
