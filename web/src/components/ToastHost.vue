<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { useToast } from '@/composables/useToast'
import ExtLink from './ExtLink.vue'

const { toast } = useToast()
</script>

<template>
  <div
    class="pointer-events-none fixed right-6 bottom-6 z-60 max-w-[420px]"
    role="status"
    aria-live="polite"
  >
    <div
      v-if="toast"
      class="bg-surface border-line-strong shadow-pop pointer-events-auto rounded-lg border px-4 py-3 text-sm"
      :class="{ 's-failed': toast.error }"
      :role="toast.error ? 'alert' : undefined"
    >
      <strong :class="toast.error ? 'text-(--c)' : ''">{{ toast.title }}</strong>
      <code v-if="toast.command" class="text-muted mt-1 block font-mono">{{ toast.command }}</code>
      <span v-if="toast.detail" class="text-muted mt-1 block">{{ toast.detail }}</span>
      <ExtLink v-if="toast.href" :href="toast.href" icon class="mt-1">{{ toast.hrefLabel ?? 'Open' }}</ExtLink>
    </div>
  </div>
</template>
