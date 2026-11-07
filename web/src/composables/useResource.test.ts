// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { ApiError } from '@/api/client'
import { useResource } from './useResource'

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('useResource retries', () => {
  it('retries a 503 with backoff until the data arrives', async () => {
    const fetcher = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new ApiError(503, 'informers not synced yet'))
      .mockRejectedValueOnce(new ApiError(503, 'informers not synced yet'))
      .mockResolvedValue('ready')
    const scope = effectScope()
    const r = scope.run(() => useResource(fetcher))!

    await vi.advanceTimersByTimeAsync(0)
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(r.error.value).toBe('informers not synced yet')
    expect(r.errorStatus.value).toBe(503)

    await vi.advanceTimersByTimeAsync(999)
    expect(fetcher).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetcher).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(2000)
    expect(fetcher).toHaveBeenCalledTimes(3)
    expect(r.data.value).toBe('ready')
    expect(r.error.value).toBe('')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetcher).toHaveBeenCalledTimes(3)
    scope.stop()
  })

  it('retries network failures and caps the delay at 5 s', async () => {
    const fetcher = vi.fn<() => Promise<string>>().mockRejectedValue(new TypeError('Failed to fetch'))
    const scope = effectScope()
    scope.run(() => useResource(fetcher))
    await vi.advanceTimersByTimeAsync(0)
    for (const wait of [1000, 2000, 4000, 5000, 5000]) await vi.advanceTimersByTimeAsync(wait)
    expect(fetcher).toHaveBeenCalledTimes(6)
    scope.stop()
  })

  it.each([401, 403, 404, 500])('does not retry a %i', async (status) => {
    const fetcher = vi.fn<() => Promise<string>>().mockRejectedValue(new ApiError(status, 'nope'))
    const scope = effectScope()
    const r = scope.run(() => useResource(fetcher))!
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(r.errorStatus.value).toBe(status)
    scope.stop()
  })

  it('stops retrying when the scope ends', async () => {
    const fetcher = vi.fn<() => Promise<string>>().mockRejectedValue(new ApiError(503, 'starting'))
    const scope = effectScope()
    scope.run(() => useResource(fetcher))
    await vi.advanceTimersByTimeAsync(0)
    scope.stop()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('an explicit refresh replaces the pending retry and starts the backoff over', async () => {
    const fetcher = vi
      .fn<() => Promise<string>>()
      .mockRejectedValueOnce(new ApiError(503, 'starting'))
      .mockResolvedValue('ok')
    const scope = effectScope()
    const r = scope.run(() => useResource(fetcher))!
    await vi.advanceTimersByTimeAsync(0)
    await r.refresh()
    expect(r.data.value).toBe('ok')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetcher).toHaveBeenCalledTimes(2)
    scope.stop()
  })
})
