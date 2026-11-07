<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { RepositorySummary } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { formatDuration, liveDuration } from '@/lib/format'
import { healthTag } from '@/lib/repos'
import { slowDelta } from '@/lib/timing'
import EventTag from './EventTag.vue'
import HealthStrip from './HealthStrip.vue'
import Icon from './Icon.vue'
import RelTime from './RelTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ repo: RepositorySummary }>()
const now = useNow()

const last = computed(() => props.repo.lastRun)
const tag = computed(() => healthTag(props.repo))
const seconds = computed(() => {
  const l = last.value
  return l ? liveDuration(l.startTime, l.completionTime, l.status, l.durationSeconds, now.value) : null
})
// Only a run that did not fail is compared with the typical duration: a failure is usually short.
const delta = computed(() => (last.value?.status === 'Failed' ? null : slowDelta(seconds.value, props.repo.typicalDurationSeconds)))
const edge = computed(() => (last.value?.status === 'Failed' ? 'is-failed' : last.value?.status === 'Running' ? 'is-running' : ''))
</script>

<template>
  <RouterLink
    :to="{ name: 'repo-runs', params: { namespace: repo.namespace, name: repo.name } }"
    class="row"
    :class="edge"
    :aria-label="`${repo.name}, last run ${last?.status ?? 'none'}`"
    :title="repo.namespace"
  >
    <div class="min-w-0">
      <div class="text-text text-md truncate font-semibold">{{ repo.name }}</div>
      <div class="text-muted flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
        <span v-if="tag.kind === 'intermittent'" class="tag warn" title="Failed and passed on the same commit within the retained runs">intermittent</span>
        <span v-else-if="tag.kind === 'failed'" class="tag" title="Failed runs among the retained runs">{{ tag.failed }} of {{ tag.total }} failed</span>
        <span v-else class="text-faint">{{ tag.total }} {{ tag.total === 1 ? 'run' : 'runs' }} retained</span>
      </div>
    </div>

    <div class="c-hist">
      <HealthStrip :recent="repo.recent" />
      <div v-if="repo.recent.length" class="text-faint mt-[3px] text-xs">last {{ repo.recent.length }} runs</div>
    </div>

    <div class="c-last min-w-0">
      <template v-if="last">
        <div class="flex min-w-0 items-center gap-2">
          <StatusBadge :status="last.status" :reason="last.message || last.reason" class="shrink-0" />
          <span class="text-text min-w-0 truncate font-[550]" :title="last.git.shaTitle">{{ last.git.shaTitle || last.prName || last.name }}</span>
        </div>
        <div class="text-muted mt-[3px] flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
          <span v-if="last.git.prNumber !== null" class="inline-flex items-center gap-1 font-mono"><Icon name="pr" sm />#{{ last.git.prNumber }}</span>
          <EventTag :event="last.git.eventType" />
          <span v-if="last.git.shortSha" class="text-accent font-mono">{{ last.git.shortSha }}</span>
        </div>
      </template>
      <span v-else class="text-faint">No runs</span>
    </div>

    <div class="c-dur">
      <template v-if="last">
        <div class="font-mono text-base" :class="{ 'text-run': delta !== null }">{{ formatDuration(seconds) }}</div>
        <div class="text-faint text-sm">
          <template v-if="repo.typicalDurationSeconds !== null">typ. {{ formatDuration(repo.typicalDurationSeconds) }}</template>
          <template v-if="delta !== null"> · <span class="text-run">+{{ delta }}%</span></template>
        </div>
      </template>
    </div>

    <div class="c-start text-muted"><RelTime v-if="last" :iso="last.startTime" /></div>
  </RouterLink>
</template>
