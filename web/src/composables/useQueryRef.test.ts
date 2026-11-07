// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useQueryRef } from './useQueryRef'

async function setup(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }] })
  await router.push(path)
  const replace = vi.spyOn(router, 'replace')
  let refs!: { status: ReturnType<typeof useQueryRef>; pr: ReturnType<typeof useQueryRef>; branch: ReturnType<typeof useQueryRef> }
  const Probe = defineComponent({
    setup() {
      refs = { status: useQueryRef('status'), pr: useQueryRef('pr'), branch: useQueryRef('branch') }
      return () => null
    },
  })
  mount(Probe, { global: { plugins: [router] } })
  return { router, replace, refs }
}

describe('useQueryRef', () => {
  it('turns writes of the same tick into one replace that keeps all of them', async () => {
    const { router, replace, refs } = await setup('/?status=Failed&pr=12&branch=main&keep=1')
    refs.status.value = ''
    refs.pr.value = ''
    refs.branch.value = ''
    await router.isReady()
    await nextTick()
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ keep: '1' }))
    expect(replace).toHaveBeenCalledTimes(1)
  })

  it('sets and removes keys, and later ticks start from the new query', async () => {
    const { router, replace, refs } = await setup('/?pr=3')
    refs.status.value = 'Failed'
    refs.pr.value = ''
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ status: 'Failed' }))
    refs.branch.value = 'dev'
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ status: 'Failed', branch: 'dev' }))
    expect(replace).toHaveBeenCalledTimes(2)
    expect(refs.status.value).toBe('Failed')
  })
})

describe('setQueryParams', () => {
  it('writes several keys of one tick, including a removal, in one replace', async () => {
    const { setQueryParams } = await import('./useQueryRef')
    const { router, replace } = await setup('/?task=a&line=45')
    setQueryParams(router, { task: 'b', step: 's', line: '' })
    setQueryParams(router, { line: '45' })
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ task: 'b', step: 's', line: '45' }))
    expect(replace).toHaveBeenCalledTimes(1)
  })
})
