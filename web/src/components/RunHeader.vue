<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { RunParam, RunSummary } from '@/api/types'
import { formatDuration } from '@/lib/format'
import Duration from './Duration.vue'
import EventTag from './EventTag.vue'
import ExtLink from './ExtLink.vue'
import Icon from './Icon.vue'
import RelTime from './RelTime.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ summary: RunSummary; params: RunParam[]; typical: number | null }>()

const git = computed(() => props.summary.git)
const title = computed(() => git.value.shaTitle || props.summary.prName || props.summary.name)
const branchUrl = computed(() => {
  const g = git.value
  if (!g.repoUrl || !g.branch) return ''
  return `${g.repoUrl}/tree/${g.branch.split('/').map(encodeURIComponent).join('/')}`
})
const senderUrl = computed(() => {
  const g = git.value
  if (!g.sender || g.provider !== 'github') return ''
  return `https://github.com/${encodeURIComponent(g.sender.replace(/\[bot\]$/, ''))}`
})
</script>

<template>
  <section class="bg-surface border-line mb-3 rounded-lg border p-4" aria-label="Run identity">
    <div class="flex flex-wrap items-start gap-x-4 gap-y-3">
      <div class="min-w-[280px] flex-1">
        <h1 class="text-lg flex flex-wrap items-center gap-x-3 gap-y-2 font-[650]">
          <StatusBadge :status="summary.status" size="lg" :reason="summary.reason" />
          <span v-if="git.prNumber !== null" class="text-accent font-mono font-semibold">#{{ git.prNumber }}</span>
          <span class="min-w-0 break-words">{{ title }}</span>
        </h1>
        <div class="text-muted mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          <span class="inline-flex min-w-0 items-center gap-1.5">
            <RouterLink :to="{ name: 'repo-runs', params: { namespace: summary.namespace, name: summary.repository } }" class="font-semibold">{{ summary.repository }}</RouterLink>
          </span>
          <span v-if="git.branch" class="inline-flex items-center gap-1.5 font-mono">
            <Icon name="branch" sm />{{ git.sourceBranch ? `${git.sourceBranch} → ` : '' }}<ExtLink :href="branchUrl">{{ git.branch }}</ExtLink>
          </span>
          <span v-if="git.shortSha" class="inline-flex items-center gap-1.5">
            <Icon name="commit" sm /><ExtLink :href="git.shaUrl" class="font-mono">{{ git.shortSha }}</ExtLink>
          </span>
          <span v-if="git.sender" class="inline-flex items-center gap-1.5">
            <Icon name="user" sm /><ExtLink :href="senderUrl">{{ git.sender }}</ExtLink><EventTag :event="git.eventType" />
          </span>
          <span class="inline-flex items-center gap-1.5">
            <Icon name="clock" sm />
            <span>
              <RelTime :iso="summary.startTime" /> ·
              <b class="text-text font-mono"><Duration :start="summary.startTime" :end="summary.completionTime" :status="summary.status" :seconds="summary.durationSeconds" /></b>
              <span v-if="typical !== null" class="text-faint"> (typ. {{ formatDuration(typical) }})</span>
            </span>
          </span>
          <span v-if="git.checkRunUrl || git.prUrl" class="inline-flex items-center gap-3">
            <ExtLink v-if="git.checkRunUrl" :href="git.checkRunUrl" icon>View check run</ExtLink>
            <ExtLink v-if="git.prUrl && git.prNumber !== null" :href="git.prUrl" icon>PR #{{ git.prNumber }}</ExtLink>
          </span>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
    <details v-if="params.length" class="params border-line group mt-3 border-t pt-2">
      <summary class="text-muted inline-flex cursor-pointer list-none items-center gap-1.5 py-1 text-sm [&::-webkit-details-marker]:hidden">
        <Icon name="chev" sm class="transition-transform group-open:rotate-90" />Parameters ({{ params.length }})
      </summary>
      <dl class="mt-2 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-0.5 text-sm">
        <template v-for="p in params" :key="p.name">
          <dt class="text-muted font-mono">{{ p.name }}</dt>
          <dd class="m-0 font-mono break-all">{{ p.value }}</dd>
        </template>
      </dl>
    </details>
  </section>
</template>
