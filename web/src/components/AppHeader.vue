<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { api, loginUrl } from '@/api/client'
import { useMe } from '@/composables/useMe'

const route = useRoute()
const { me } = useMe()

const section = computed(() => (route.path.startsWith('/runs') ? 'runs' : 'repositories'))
const login = computed(() => loginUrl(route.fullPath))
const showLogin = computed(() => me.value?.authMode === 'github' && !me.value.authenticated)

async function logout(): Promise<void> {
  try {
    await api.logout()
  } finally {
    window.location.assign('/')
  }
}

const linkClass =
  'rounded px-2 py-1 text-sm font-medium text-gray-700 hover:bg-gray-200 dark:text-gray-300 dark:hover:bg-gray-800'
const activeClass = 'bg-gray-200 dark:bg-gray-800'
</script>

<template>
  <header class="border-b border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
    <div class="mx-auto flex max-w-screen-2xl items-center gap-4 px-4 py-2">
      <RouterLink to="/" class="flex items-center gap-2 text-base font-semibold" aria-label="pacboard home">
        <img src="/favicon.svg" alt="" class="h-6 w-6" width="24" height="24" />
        pacboard
      </RouterLink>
      <nav aria-label="Main" class="flex gap-1">
        <RouterLink to="/" :class="[linkClass, { [activeClass]: section === 'repositories' }]" :aria-current="section === 'repositories' ? 'page' : undefined">Repositories</RouterLink>
        <RouterLink to="/runs" :class="[linkClass, { [activeClass]: section === 'runs' }]" :aria-current="section === 'runs' ? 'page' : undefined">Runs</RouterLink>
      </nav>
      <div class="ml-auto flex items-center gap-3 text-sm">
        <template v-if="me?.authenticated">
          <span class="flex items-center gap-2" aria-label="Signed in user">
            <img v-if="me.avatarUrl" :src="me.avatarUrl" alt="" class="h-6 w-6 rounded-full" width="24" height="24" referrerpolicy="no-referrer" />
            {{ me.login }}
          </span>
          <button v-if="me.authMode === 'github'" type="button" class="rounded border border-gray-300 px-2 py-1 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800" @click="logout">Log out</button>
        </template>
        <a v-else-if="showLogin" :href="login" class="rounded bg-gray-900 px-3 py-1 font-medium text-white hover:bg-gray-700 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-300">Log in with GitHub</a>
        <span v-else-if="me" class="text-gray-500 dark:text-gray-400">Read-only</span>
      </div>
    </div>
  </header>
</template>
