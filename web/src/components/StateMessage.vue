<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
defineProps<{ loading?: boolean; error?: string; empty?: boolean; emptyText?: string }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div v-if="error" role="alert" class="callout s-failed text-text text-sm">
    <span class="min-w-0 flex-1">{{ error }}</span>
    <button type="button" class="btn sm" :disabled="loading" @click="$emit('retry')">Retry</button>
  </div>
  <p v-else-if="loading && empty" class="text-muted px-1 py-6 text-sm">Loading…</p>
  <div v-else-if="empty" class="text-muted px-4 py-8 text-center">
    <p>{{ emptyText ?? 'Nothing to show.' }}</p>
    <slot />
  </div>
</template>
