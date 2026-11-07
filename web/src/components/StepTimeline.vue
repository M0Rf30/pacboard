<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { GraphNode } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { elapsedSeconds, formatDuration } from '@/lib/format'
import { statusStyle } from '@/lib/status'
import { stepBars } from '@/lib/timing'
import Icon from './Icon.vue'
import StatusIcon from './StatusIcon.vue'

// A task's steps as a mini Gantt: each bar sits where the step ran inside the task's runtime.
const props = defineProps<{ node: GraphNode; selected: string }>()
defineEmits<{ select: [step: string] }>()

const now = useNow()
const bars = computed(() => stepBars(props.node.steps, now.value))
const total = computed(() => elapsedSeconds(props.node.startTime, props.node.completionTime, now.value))
</script>

<template>
  <aside class="steps" :aria-label="`Steps of ${node.id}`">
    <h3 class="text-faint m-2 text-xs font-[650] tracking-wider uppercase">Steps · {{ node.steps.length }}</h3>
    <button v-if="node.steps.length > 1" type="button" class="step" :aria-pressed="selected === ''" @click="$emit('select', '')">
      <span class="sicon text-muted"><Icon name="chev" sm /></span>
      <span class="n">All steps</span>
      <span class="d">{{ formatDuration(total) }}</span>
    </button>
    <button
      v-for="(s, i) in node.steps"
      :key="s.name"
      type="button"
      class="step"
      :class="statusStyle(s.status).cls"
      :aria-pressed="selected === s.name"
      @click="$emit('select', s.name)"
    >
      <span class="sicon"><StatusIcon :status="s.status" sm /></span>
      <span class="n font-mono break-all">{{ s.name }}</span>
      <span class="d">{{ s.status === 'Pending' ? 'not run' : formatDuration(bars[i]?.seconds ?? elapsedSeconds(s.startTime, s.completionTime, now)) }}</span>
      <span v-if="bars[i]" class="track" aria-hidden="true"><i :style="{ left: `${bars[i]!.left}%`, width: `${bars[i]!.width}%` }" /></span>
      <span v-if="s.status === 'Failed' && s.exitCode !== null" class="xc">exit code {{ s.exitCode }}</span>
    </button>
    <p v-if="node.steps.length === 0" class="text-muted px-2 py-3 text-sm">No steps yet.</p>
  </aside>
</template>
