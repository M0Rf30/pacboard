<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { api, loginUrl } from '@/api/client'
import { useMe } from '@/composables/useMe'
import { useTheme } from '@/composables/useTheme'
import { useToast } from '@/composables/useToast'
import { otherTheme } from '@/lib/theme'
import Icon from './Icon.vue'

const route = useRoute()
const { me } = useMe()
const { theme, toggle } = useTheme()
const { show } = useToast()

const section = computed(() => (route.path === '/runs' ? 'runs' : 'repositories'))
const login = computed(() => loginUrl(route.fullPath))
const showLogin = computed(() => me.value?.authMode === 'github' && !me.value.authenticated)
const initials = computed(() => (me.value?.login ?? '').slice(0, 2).toUpperCase())

async function logout(): Promise<void> {
  try {
    await api.logout()
  } catch (e) {
    // Still signed in (for example a rejected CSRF token): say so instead of bouncing home.
    show({ title: 'Could not log out', detail: e instanceof Error ? e.message : String(e), error: true })
    return
  }
  window.location.assign('/')
}

const navClass =
  'inline-flex h-[30px] items-center rounded-md px-3 font-medium text-muted transition-colors duration-150 hover:bg-raised hover:text-text hover:no-underline'
</script>

<template>
  <header class="bg-surface border-line flex h-12 items-center gap-4 border-b px-4">
    <RouterLink to="/" class="text-text flex items-center gap-2 text-md font-bold tracking-tight hover:no-underline" aria-label="pacboard home">
      <img src="/favicon.svg" alt="" class="h-[22px] w-[22px]" width="22" height="22" />
      pacboard
    </RouterLink>
    <nav aria-label="Primary" class="flex gap-0.5">
      <RouterLink to="/" :class="[navClass, { 'bg-raised text-text': section === 'repositories' }]" :aria-current="section === 'repositories' ? 'page' : undefined">Repositories</RouterLink>
      <RouterLink to="/runs" :class="[navClass, { 'bg-raised text-text': section === 'runs' }]" :aria-current="section === 'runs' ? 'page' : undefined">Runs</RouterLink>
    </nav>
    <div class="text-muted ml-auto flex items-center gap-3 text-sm">
      <button type="button" class="btn sm" :aria-label="`Switch to ${otherTheme(theme)} theme`" :title="`Switch to ${otherTheme(theme)} theme`" @click="toggle">
        <Icon :name="theme === 'dark' ? 'sun' : 'moon'" sm />
        <span class="capitalize">{{ otherTheme(theme) }}</span>
      </button>
      <template v-if="me?.authenticated">
        <span class="flex items-center gap-2" aria-label="Signed in user">
          <img v-if="me.avatarUrl" :src="me.avatarUrl" alt="" class="border-line-strong h-[22px] w-[22px] rounded border" width="22" height="22" referrerpolicy="no-referrer" />
          <span v-else class="bg-raised border-line-strong text-text grid h-[22px] w-[22px] place-items-center rounded border text-[10px] font-bold" aria-hidden="true">{{ initials }}</span>
          {{ me.login }}
        </span>
        <button v-if="me.authMode === 'github'" type="button" class="btn sm" @click="logout">Log out</button>
      </template>
      <a v-else-if="showLogin" :href="login" class="btn primary sm hover:no-underline">Log in with GitHub</a>
      <span v-else-if="me">Read-only</span>
    </div>
  </header>
</template>
