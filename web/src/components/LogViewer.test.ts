// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { FocusRequest } from '@/lib/logview'
import LogViewer from './LogViewer.vue'

vi.mock('@/api/client', async (orig) => ({
  ...(await orig<typeof import('@/api/client')>()),
  streamLogs: vi.fn(async (_url: string, _signal: AbortSignal, onText: (t: string) => void) => {
    onText('one\ntwo\nthree\n')
  }),
}))

const base = { namespace: 'ns', run: 'r', taskRun: 'tr', step: 'go', active: true, follow: false, highlight: null }

describe('LogViewer focus requests', () => {
  it('ignores a request for another task, then consumes it once the props switch to that task', async () => {
    Element.prototype.scrollIntoView = vi.fn()
    const req: FocusRequest = { seq: 1, task: 'x', taskRun: 'tr', step: 'go', line: 2, error: false }
    const w = mount(LogViewer, { props: { ...base, task: 'y', focus: req } })
    await vi.waitFor(() => expect(w.findAll('[data-no]').length).toBe(3))
    await new Promise((r) => setTimeout(r, 20))
    expect(w.emitted('update:highlight')).toBeUndefined()

    await w.setProps({ task: 'x' })
    await vi.waitFor(() => expect(w.emitted('update:highlight')?.at(-1)).toEqual([2]))
  })
})
