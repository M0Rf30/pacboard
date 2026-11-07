<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { RunSummary } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { formatDuration, liveDuration } from '@/lib/format'
import { statusStyle } from '@/lib/status'
import { durationBarPercent, slowDelta } from '@/lib/timing'
import EventTag from './EventTag.vue'
import Icon from './Icon.vue'
import RelTime from './RelTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  run: RunSummary
  /** Longest duration among the listed runs: the bar's 100%. */
  longest: number
  /** The repository's typical duration, for the slow marker. */
  typical: number | null
  /** All-runs view: the repository name leads the second line. */
  showRepo?: boolean
}>()
const now = useNow()

const git = computed(() => props.run.git)
const seconds = computed(() => liveDuration(props.run.startTime, props.run.completionTime, props.run.status, props.run.durationSeconds, now.value))
const slow = computed(() => props.run.status !== 'Failed' && slowDelta(seconds.value, props.typical) !== null)
const barWidth = computed(() => durationBarPercent(seconds.value, props.longest))
const edge = computed(() => (props.run.status === 'Failed' ? 'is-failed' : props.run.status === 'Running' ? 'is-running' : ''))
const title = computed(() => git.value.shaTitle || props.run.prName || props.run.name)
const barTitle = computed(() => (slow.value ? 'Slower than typical for this repository' : 'Relative to the longest run listed'))
</script>

<template>
  <RouterLink
    :to="{ name: 'run', params: { namespace: run.namespace, name: run.name } }"
    class="row"
    :class="edge"
    :aria-label="`${title}, ${run.status}`"
    :title="run.name"
  >
    <div class="c-status"><StatusBadge :status="run.status" :reason="run.message || run.reason" /></div>

    <div class="c-commit min-w-0">
      <div class="text-text truncate font-[550]" :title="title">{{ title }}</div>
      <div class="text-muted mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
        <span v-if="showRepo" class="text-text font-semibold">{{ run.repository }}</span>
        <span v-if="git.prNumber !== null" class="inline-flex items-center gap-1 font-mono"><Icon name="pr" sm />#{{ git.prNumber }}</span>
        <span v-if="git.shortSha" class="inline-flex items-center gap-1"><Icon name="commit" sm /><span class="text-accent font-mono">{{ git.shortSha }}</span></span>
        <span v-if="git.branch" class="inline-flex items-center gap-1 font-mono"><Icon name="branch" sm />{{ git.sourceBranch ? `${git.sourceBranch} → ` : '' }}{{ git.branch }}</span>
      </div>
    </div>

    <div class="c-event"><EventTag :event="git.eventType" /></div>
    <div class="c-sender text-muted truncate">{{ git.sender || '—' }}</div>
    <div class="c-start text-muted"><RelTime :iso="run.startTime" /></div>

    <div class="c-dur">
      <div class="font-mono text-base" :class="{ 'text-run': slow }">{{ formatDuration(seconds) }}</div>
      <div class="dur-bar" :class="[statusStyle(run.status).cls, { slow }]" :title="barTitle"><i :style="{ width: `${barWidth}%` }" /></div>
    </div>
  </RouterLink>
</template>
