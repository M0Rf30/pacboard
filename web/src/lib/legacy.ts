// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Tekton Dashboard links (hash routes) that Pipelines-as-Code wrote into existing GitHub check runs
// and comments, mapped to pacboard routes so they keep working after pacboard replaces the Dashboard:
//   #/namespaces/<ns>/pipelineruns/<name>[?pipelineTask=<task>&step=<step>]
//   #/namespaces/<ns>/pipelineruns
//   #/namespaces/<ns>/taskruns/<name>  (no owning run known: falls back to the run list)

export interface LegacyTarget {
  path: string
  query: Record<string, string>
}

const NAME = '[a-z0-9]([-a-z0-9.]*[a-z0-9])?'
const RUN = new RegExp(`^#/namespaces/(${NAME})/pipelineruns/(${NAME})$`)
const NS_RUNS = new RegExp(`^#/namespaces/(${NAME})/(pipelineruns|taskruns)(/${NAME})?$`)

/** The pacboard route for a Tekton Dashboard hash, or null when the hash is not one of its routes. */
export function legacyDashboardTarget(hash: string): LegacyTarget | null {
  if (!hash.startsWith('#/')) return null
  const q = hash.indexOf('?')
  const path = q === -1 ? hash : hash.slice(0, q)
  const params = new URLSearchParams(q === -1 ? '' : hash.slice(q + 1))

  const run = RUN.exec(path)
  if (run) {
    const query: Record<string, string> = {}
    const task = params.get('pipelineTask')
    const step = params.get('step')
    if (task) query.task = task
    if (task && step) query.step = step
    return { path: `/runs/${run[1]}/${run[3]}`, query }
  }
  if (NS_RUNS.test(path)) return { path: '/runs', query: {} }
  return null
}
