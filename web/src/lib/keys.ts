// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

/** Whether the keyboard event target is a field the user is typing in (shortcuts must not fire there). */
export function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || /^(INPUT|SELECT|TEXTAREA)$/.test(target.tagName)
}
