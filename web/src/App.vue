<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useEventListener } from '@vueuse/core'
import { useRoute } from 'vue-router'
import AppHeader from './components/AppHeader.vue'
import IconSprite from './components/IconSprite.vue'
import ToastHost from './components/ToastHost.vue'
import { loadMe } from './composables/useMe'
import { pageTitle } from './composables/usePageTitle'
import { isTyping } from './lib/keys'

const route = useRoute()

onMounted(() => void loadMe())

// A page's own title (usePageTitle) wins; every route has a fallback in meta.title.
const title = computed(() => pageTitle.value || (typeof route.meta.title === 'string' ? route.meta.title : ''))
watch(
  title,
  (t) => {
    if (t) document.title = `${t} · pacboard`
  },
  { immediate: true },
)

// `/` focuses the page's filter input (marked data-filter), Esc leaves any input.
useEventListener(document, 'keydown', (ev: KeyboardEvent) => {
  const typing = isTyping(ev.target)
  if (ev.key === 'Escape' && typing) {
    ;(ev.target as HTMLElement).blur()
    return
  }
  if (ev.key !== '/' || typing || ev.metaKey || ev.ctrlKey || ev.altKey) return
  const input = document.querySelector<HTMLInputElement>('[data-filter]')
  if (!input) return
  ev.preventDefault()
  input.focus()
})
</script>

<template>
  <IconSprite />
  <a
    href="#main"
    class="bg-accent text-on-accent sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:px-3 focus:py-1.5"
  >Skip to content</a>
  <AppHeader />
  <main id="main" class="mx-auto max-w-[1680px] px-3 pt-4 pb-12 min-[900px]:px-6 min-[900px]:pt-6 min-[900px]:pb-16" tabindex="-1">
    <!-- Keyed by path: moving between runs or repositories never shows the previous page's data. -->
    <RouterView v-slot="{ Component, route: current }">
      <component :is="Component" :key="current.path" />
    </RouterView>
  </main>
  <ToastHost />
</template>
