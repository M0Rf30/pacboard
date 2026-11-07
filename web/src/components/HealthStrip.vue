<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { RunBrief } from '@/api/types'
import { historyBars } from '@/lib/timing'
import { statusStyle } from '@/lib/status'

// The retained runs, oldest left to newest right. Bar height is a second cue: failed and running
// are tall, succeeded about half height, cancelled/unknown short.
const props = defineProps<{ recent: RunBrief[]; large?: boolean }>()

const bars = computed(() => historyBars(props.recent))
const label = computed(
  () => `Last ${bars.value.length} runs, oldest to newest: ${bars.value.map((b) => b.status).join(', ')}`,
)
</script>

<template>
  <span v-if="bars.length" class="strip" :class="{ lg: large }" role="img" :aria-label="label">
    <i v-for="b in bars" :key="b.name" :class="statusStyle(b.status).cls" :title="b.status" />
  </span>
  <span v-else class="text-faint text-sm">no runs</span>
</template>
