<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
defineProps<{ live: boolean }>()
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400"
    role="status"
    :title="live ? 'Receiving live updates' : 'Live updates disconnected, retrying'"
  >
    <span class="h-2 w-2 rounded-full" :class="live ? 'bg-green-500' : 'bg-gray-400'" aria-hidden="true" />
    {{ live ? 'Live' : 'Offline' }}
  </span>
</template>
