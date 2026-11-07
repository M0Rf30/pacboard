// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRouter, createWebHistory } from 'vue-router'
import RepositoriesPage from './pages/RepositoriesPage.vue'
import RepoRunsPage from './pages/RepoRunsPage.vue'
import AllRunsPage from './pages/AllRunsPage.vue'
import RunDetailPage from './pages/RunDetailPage.vue'
import NotFoundPage from './pages/NotFoundPage.vue'
import { legacyDashboardTarget } from './lib/legacy'

// Pages fetch their data after the route changed, so the content that a saved position or a #hash
// points at does not exist yet; scrolling waits a moment for it.
const SCROLL_DELAY_MS = 200
const afterRender = <T,>(pos: T) => new Promise<T>((resolve) => setTimeout(() => resolve(pos), SCROLL_DELAY_MS))

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'repositories', component: RepositoriesPage, meta: { title: 'Repositories' } },
    { path: '/repos/:namespace/:name', name: 'repo-runs', component: RepoRunsPage, props: true, meta: { title: 'Repository' } },
    { path: '/runs', name: 'runs', component: AllRunsPage, meta: { title: 'Runs' } },
    { path: '/runs/:namespace/:name', name: 'run', component: RunDetailPage, props: true, meta: { title: 'Run' } },
    { path: '/:rest(.*)*', name: 'not-found', component: NotFoundPage, meta: { title: 'Not found' } },
  ],
  scrollBehavior(to, from, saved) {
    if (saved) return afterRender(saved)
    if (to.hash) return afterRender({ el: to.hash })
    return to.path === from.path ? false : { top: 0 }
  },
})

// Links to the Tekton Dashboard (hash routes) left in old GitHub check runs open the same run here.
router.beforeEach((to) => {
  const legacy = to.path === '/' ? legacyDashboardTarget(to.hash) : null
  return legacy ? { path: legacy.path, query: legacy.query, hash: '', replace: true } : true
})
