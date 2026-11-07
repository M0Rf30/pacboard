<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { NodeStatus } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { formatDuration, liveDuration } from '@/lib/format'

const props = defineProps<{
  start: string | null
  end: string | null
  status: NodeStatus
  seconds?: number | null
}>()
const now = useNow()
const text = computed(() => formatDuration(liveDuration(props.start, props.end, props.status, props.seconds, now.value)))
</script>

<template>
  <span class="whitespace-nowrap tabular-nums">{{ text }}</span>
</template>
