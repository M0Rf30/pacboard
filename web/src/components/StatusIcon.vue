<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import type { NodeStatus } from '@/api/types'
import { statusStyle } from '@/lib/status'
import Icon from './Icon.vue'

defineProps<{ status: NodeStatus; sm?: boolean }>()
</script>

<template>
  <Icon :name="statusStyle(status).icon" :sm="sm" :spin="status === 'Running'" />
</template>
