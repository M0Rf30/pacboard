// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import type { RepositorySummary, RunBrief, RunStatus, RunSummary } from '@/api/types'
import { groupRepos, healthTag, matchesRepo, repoCounts, repoGroup } from './repos'

function run(status: RunStatus): RunSummary {
  return {
    namespace: 'ns',
    name: 'r',
    prName: 'r',
    repository: 'repo',
    status,
    reason: '',
    message: '',
    startTime: null,
    completionTime: null,
    durationSeconds: null,
    git: {
      provider: 'github',
      repoUrl: '',
      sha: '',
      shortSha: '',
      shaUrl: '',
      shaTitle: '',
      branch: '',
      sourceBranch: '',
      prNumber: null,
      prUrl: '',
      eventType: '',
      sender: '',
      checkRunUrl: '',
    },
  }
}

const brief = (status: RunStatus): RunBrief => ({ name: status, status, startTime: null, durationSeconds: null })

function repo(name: string, last: RunStatus | null, extra: Partial<RepositorySummary> = {}): RepositorySummary {
  return {
    namespace: 'ns',
    name,
    url: '',
    concurrencyLimit: null,
    lastRun: last ? run(last) : null,
    runCount: 3,
    recent: [],
    typicalDurationSeconds: null,
    intermittent: false,
    ...extra,
  }
}

const all = [repo('ok', 'Succeeded'), repo('bad', 'Failed'), repo('busy', 'Running'), repo('none', null), repo('queued', 'Pending')]

describe('repoGroup', () => {
  it('groups by the last run', () => {
    expect(repoGroup(all[0]!)).toBe('passing')
    expect(repoGroup(all[1]!)).toBe('attention')
    expect(repoGroup(all[2]!)).toBe('running')
    expect(repoGroup(all[3]!)).toBe('passing')
    expect(repoGroup(all[4]!)).toBe('running')
    expect(repoGroup(repo('x', 'Cancelled'))).toBe('passing')
  })
})

describe('groupRepos', () => {
  it('orders the sections and omits empty ones', () => {
    const s = groupRepos(all)
    expect(s.map((x) => x.group)).toEqual(['attention', 'running', 'passing'])
    expect(s[1]!.repos.map((r) => r.name)).toEqual(['busy', 'queued'])
    expect(groupRepos([repo('ok', 'Succeeded')]).map((x) => x.group)).toEqual(['passing'])
    expect(groupRepos([])).toEqual([])
  })
})

describe('repoCounts and matchesRepo', () => {
  it('counts per chip', () => {
    expect(repoCounts(all)).toEqual({ all: 5, failing: 1, running: 2, passing: 2 })
  })

  it('combines the chip and the text filter', () => {
    expect(matchesRepo(all[1]!, 'failing', '')).toBe(true)
    expect(matchesRepo(all[0]!, 'failing', '')).toBe(false)
    expect(matchesRepo(all[0]!, 'all', ' OK ')).toBe(true)
    expect(matchesRepo(all[0]!, 'all', 'zzz')).toBe(false)
    expect(matchesRepo(all[0]!, 'all', 'NS')).toBe(true)
  })
})

describe('healthTag', () => {
  it('flags intermittent repositories first', () => {
    expect(healthTag(repo('a', 'Failed', { intermittent: true, recent: [brief('Failed')] }))).toEqual({ kind: 'intermittent' })
  })

  it('counts failures when the latest run failed', () => {
    const r = repo('a', 'Failed', { recent: [brief('Failed'), brief('Succeeded'), brief('Failed')] })
    expect(healthTag(r)).toEqual({ kind: 'failed', failed: 2, total: 3 })
  })

  it('falls back to the retained count', () => {
    expect(healthTag(repo('a', 'Succeeded', { runCount: 7 }))).toEqual({ kind: 'retained', total: 7 })
    expect(healthTag(repo('a', 'Failed', { runCount: 4 }))).toEqual({ kind: 'retained', total: 4 })
  })
})
