<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppHeader from './components/AppHeader.vue'
import { loadMe } from './composables/useMe'

const route = useRoute()

onMounted(() => void loadMe())

watch(
  () => route.meta.title,
  (t) => {
    if (typeof t === 'string') document.title = `${t} · pacboard`
  },
  { immediate: true },
)
</script>

<template>
  <a href="#main" class="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:rounded focus:bg-white focus:px-3 focus:py-1 focus:text-gray-900">Skip to content</a>
  <AppHeader />
  <main id="main" class="mx-auto max-w-screen-2xl px-4 py-4">
    <!-- Keyed by path: moving between runs or repositories never shows the previous page's data. -->
    <RouterView v-slot="{ Component, route: current }">
      <component :is="Component" :key="current.path" />
    </RouterView>
  </main>
</template>
