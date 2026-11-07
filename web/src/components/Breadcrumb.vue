<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'

export interface Crumb {
  label: string
  /** Absent on the current page. */
  to?: RouteLocationRaw
  mono?: boolean
}

defineProps<{ items: Crumb[] }>()
</script>

<template>
  <nav aria-label="Breadcrumb" class="text-faint mb-3 flex flex-wrap items-center gap-2 text-sm">
    <template v-for="(c, i) in items" :key="i">
      <span v-if="i > 0" class="text-line-strong" aria-hidden="true">/</span>
      <RouterLink v-if="c.to" :to="c.to" class="text-muted" :class="{ 'font-mono': c.mono }">{{ c.label }}</RouterLink>
      <span v-else aria-current="page" :class="{ 'font-mono': c.mono }">{{ c.label }}</span>
    </template>
  </nav>
</template>
