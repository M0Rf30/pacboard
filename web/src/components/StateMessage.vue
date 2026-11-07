<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
defineProps<{ loading?: boolean; error?: string; empty?: boolean; emptyText?: string }>()
</script>

<template>
  <p v-if="error" role="alert" class="rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
    {{ error }}
  </p>
  <p v-else-if="loading && empty" class="px-1 py-6 text-sm text-gray-500 dark:text-gray-400">Loading…</p>
  <p v-else-if="empty" class="px-1 py-6 text-sm text-gray-500 dark:text-gray-400">{{ emptyText ?? 'Nothing to show.' }}</p>
</template>
