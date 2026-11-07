<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import type { NodeStatus } from '@/api/types'
import { statusStyle } from '@/lib/status'
import StatusIcon from './StatusIcon.vue'

// Colour + shape + word: a status is never shown as a bare dot.
defineProps<{ status: NodeStatus; size?: 'md' | 'lg'; reason?: string }>()
</script>

<template>
  <span class="badge" :class="[statusStyle(status).cls, { lg: size === 'lg' }]" :title="reason || undefined">
    <StatusIcon :status="status" />
    <span>{{ statusStyle(status).label }}</span>
  </span>
</template>
