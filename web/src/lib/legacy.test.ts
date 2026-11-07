// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { legacyDashboardTarget } from './legacy'

describe('legacyDashboardTarget', () => {
  it('maps PaC detail and task log links', () => {
    expect(legacyDashboardTarget('#/namespaces/tekton-ci/pipelineruns/carbonio-proxy-9fjnt')).toEqual({
      path: '/runs/tekton-ci/carbonio-proxy-9fjnt',
      query: {},
    })
    expect(legacyDashboardTarget('#/namespaces/tekton-ci/pipelineruns/carbonio-proxy-9fjnt?pipelineTask=gitleaks&step=scan')).toEqual({
      path: '/runs/tekton-ci/carbonio-proxy-9fjnt',
      query: { task: 'gitleaks', step: 'scan' },
    })
  })

  it('sends namespace and TaskRun links to the run list', () => {
    expect(legacyDashboardTarget('#/namespaces/tekton-ci/pipelineruns')?.path).toBe('/runs')
    expect(legacyDashboardTarget('#/namespaces/tekton-ci/taskruns/x-clone')?.path).toBe('/runs')
  })

  it('ignores other hashes and names that are not Kubernetes names', () => {
    for (const h of ['', '#', '#top', '#/about', '#/namespaces/a/pipelineruns/..%2F..', '#/namespaces/A/pipelineruns/x', '#/namespaces/a/pipelineruns/x/y']) {
      expect(legacyDashboardTarget(h)).toBeNull()
    }
  })
})
