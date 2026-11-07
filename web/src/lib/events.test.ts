// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { eventLabel, isCommandEvent } from './events'

describe('eventLabel', () => {
  it('names the known events', () => {
    expect(eventLabel('pull_request')).toBe('Pull request')
    expect(eventLabel('push')).toBe('Push')
    expect(eventLabel('retest-all-comment')).toBe('/retest all')
    expect(eventLabel('test-comment')).toBe('/test')
  })

  it('derives a command from an unknown comment event', () => {
    expect(eventLabel('foo-comment')).toBe('/foo')
    expect(eventLabel('foo-all-comment')).toBe('/foo all')
    expect(eventLabel('-comment')).toBe('/')
  })

  it('humanises other identifiers and tolerates an empty one', () => {
    expect(eventLabel('some_new-event')).toBe('Some new event')
    expect(eventLabel('')).toBe('')
  })
})

describe('isCommandEvent', () => {
  it('is true for slash-command events only', () => {
    expect(isCommandEvent('retest-all-comment')).toBe(true)
    expect(isCommandEvent('push')).toBe(false)
  })
})
