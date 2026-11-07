// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import type { RepositorySummary } from '@/api/types'

export type RepoGroup = 'attention' | 'running' | 'passing'
export type RepoFilter = 'all' | 'failing' | 'running' | 'passing'

export const GROUP_ORDER: RepoGroup[] = ['attention', 'running', 'passing']

export const GROUP_TITLE: Record<RepoGroup, string> = {
  attention: 'Needs attention',
  running: 'Running',
  passing: 'Passing',
}

export const FILTER_OF_GROUP: Record<RepoGroup, Exclude<RepoFilter, 'all'>> = {
  attention: 'failing',
  running: 'running',
  passing: 'passing',
}

/** Where a repository belongs: by its last run. No run, a cancelled or an unknown one count as passing. */
export function repoGroup(r: RepositorySummary): RepoGroup {
  switch (r.lastRun?.status) {
    case 'Failed':
      return 'attention'
    case 'Running':
    case 'Pending':
      return 'running'
    default:
      return 'passing'
  }
}

export interface RepoSection {
  group: RepoGroup
  repos: RepositorySummary[]
}

/** Sections in display order (empty ones omitted); inside a section the API order (newest run first) stays. */
export function groupRepos(repos: readonly RepositorySummary[]): RepoSection[] {
  const sections: RepoSection[] = GROUP_ORDER.map((group) => ({ group, repos: [] }))
  for (const r of repos) sections[GROUP_ORDER.indexOf(repoGroup(r))]!.repos.push(r)
  return sections.filter((s) => s.repos.length > 0)
}

export function repoCounts(repos: readonly RepositorySummary[]): Record<RepoFilter, number> {
  const c: Record<RepoFilter, number> = { all: repos.length, failing: 0, running: 0, passing: 0 }
  for (const r of repos) c[FILTER_OF_GROUP[repoGroup(r)]]++
  return c
}

/** Whether a repository passes the chip and the free-text filter (name or namespace, case-insensitive). */
export function matchesRepo(r: RepositorySummary, filter: RepoFilter, text: string): boolean {
  if (filter !== 'all' && FILTER_OF_GROUP[repoGroup(r)] !== filter) return false
  const q = text.trim().toLowerCase()
  return !q || `${r.name} ${r.namespace}`.toLowerCase().includes(q)
}

export type HealthTag =
  | { kind: 'intermittent' }
  | { kind: 'failed'; failed: number; total: number }
  | { kind: 'retained'; total: number }

/** The small tag under a repository name: intermittent, "k of N failed" when the latest failed, else the retained count. */
export function healthTag(r: RepositorySummary): HealthTag {
  if (r.intermittent) return { kind: 'intermittent' }
  if (r.lastRun?.status === 'Failed') {
    const failed = r.recent.filter((b) => b.status === 'Failed').length
    if (failed > 0) return { kind: 'failed', failed, total: r.recent.length }
  }
  return { kind: 'retained', total: r.runCount }
}
