<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import type { NodeStatus } from '@/api/types'
import { statusClasses, statusGlyph } from '@/lib/status'

defineProps<{ status: NodeStatus; reason?: string }>()
</script>

<template>
  <span
    class="inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 text-xs font-medium"
    :class="statusClasses(status)"
    :title="reason || undefined"
  >
    <span aria-hidden="true" :class="{ 'animate-pulse': status === 'Running' }">{{ statusGlyph(status) }}</span>
    {{ status }}
  </span>
</template>
