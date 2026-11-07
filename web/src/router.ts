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

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'repositories', component: RepositoriesPage, meta: { title: 'Repositories' } },
    { path: '/repos/:namespace/:name', name: 'repo-runs', component: RepoRunsPage, props: true },
    { path: '/runs', name: 'runs', component: AllRunsPage, meta: { title: 'Runs' } },
    { path: '/runs/:namespace/:name', name: 'run', component: RunDetailPage, props: true },
    { path: '/:rest(.*)*', name: 'not-found', component: NotFoundPage, meta: { title: 'Not found' } },
  ],
  scrollBehavior: (to, from) => (to.path === from.path ? false : { top: 0 }),
})

// Links to the Tekton Dashboard (hash routes) left in old GitHub check runs open the same run here.
router.beforeEach((to) => {
  const legacy = to.path === '/' ? legacyDashboardTarget(to.hash) : null
  return legacy ? { path: legacy.path, query: legacy.query, hash: '', replace: true } : true
})
