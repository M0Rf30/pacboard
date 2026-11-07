<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { refDebounced } from '@vueuse/core'
import { api } from '@/api/client'
import type { RunStatus } from '@/api/types'
import Breadcrumb from '@/components/Breadcrumb.vue'
import ExtLink from '@/components/ExtLink.vue'
import HealthStrip from '@/components/HealthStrip.vue'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RelTime from '@/components/RelTime.vue'
import RunList from '@/components/RunList.vue'
import StateMessage from '@/components/StateMessage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { useLive } from '@/composables/useLive'
import { useQueryRef } from '@/composables/useQueryRef'
import { usePageTitle } from '@/composables/usePageTitle'
import { useResource } from '@/composables/useResource'
import { formatDuration } from '@/lib/format'

const props = defineProps<{ namespace: string; name: string }>()

usePageTitle(() => props.name)

const PAGE = 50
const STATUS_CHIPS: { value: RunStatus | ''; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'Failed', label: 'Failed' },
  { value: 'Succeeded', label: 'Succeeded' },
  { value: 'Running', label: 'Running' },
  { value: 'Cancelled', label: 'Cancelled' },
]

const status = useQueryRef('status')
const pr = useQueryRef('pr')
const branch = useQueryRef('branch')
const branchDebounced = refDebounced(branch, 400)
const prDebounced = refDebounced(pr, 400)
const limit = ref(PAGE)

const prNumber = computed(() => {
  const n = Number.parseInt(prDebounced.value.replace(/^#/, ''), 10)
  return Number.isFinite(n) && n > 0 ? n : null
})

// "Show more" applies to one filter: a new filter starts from the first page again. Declared before
// the resource so the reset lands in the same flush as the filter change (one fetch, not two).
watch([status, prNumber, branchDebounced], () => (limit.value = PAGE))

const { data, error, loading, refresh } = useResource(
  (signal) =>
    api.repoRuns(
      props.namespace,
      props.name,
      { limit: limit.value, pr: prNumber.value, branch: branchDebounced.value.trim(), status: status.value as RunStatus | '' },
      signal,
    ),
  [() => props.namespace, () => props.name, status, prNumber, branchDebounced, limit],
)
// The summary bar and the typical duration come from the repository summary (history strip, median).
const { data: repos, refresh: refreshRepos } = useResource((signal) => api.repositories(signal), [() => props.namespace, () => props.name])

const { connected, paused } = useLive(
  () => {
    void refresh()
    void refreshRepos()
  },
  (e) => e.kind !== 'TaskRun' && e.namespace === props.namespace && (e.repository === props.name || (e.kind === 'Repository' && e.name === props.name)),
)

const runs = computed(() => data.value ?? [])
const repo = computed(() => repos.value?.find((r) => r.namespace === props.namespace && r.name === props.name))
const repoUrl = computed(() => repo.value?.url || (runs.value.find((r) => r.git.repoUrl)?.git.repoUrl ?? ''))
const typical = computed(() => repo.value?.typicalDurationSeconds ?? null)
const filtered = computed(() => status.value !== '' || pr.value !== '' || branch.value !== '')
const passed = computed(() => repo.value?.recent.filter((b) => b.status === 'Succeeded').length ?? 0)

function clearFilters(): void {
  status.value = ''
  pr.value = ''
  branch.value = ''
}
</script>

<template>
  <section>
    <Breadcrumb :items="[{ label: 'Repositories', to: '/' }, { label: name }]" />

    <div class="mb-4 flex flex-wrap items-center gap-3">
      <h1>{{ name }}</h1>
      <span class="tag font-mono">{{ namespace }}</span>
      <ExtLink v-if="repoUrl" :href="repoUrl" icon>GitHub</ExtLink>
      <LiveIndicator v-model:paused="paused" :connected="connected" />
    </div>

    <div v-if="repo" class="bg-surface border-line mb-4 flex flex-wrap items-center gap-6 rounded-lg border px-4 py-3" aria-label="Repository summary">
      <div>
        <div class="text-faint text-xs font-[650] tracking-wider uppercase">History · {{ repo.recent.length }} retained</div>
        <HealthStrip :recent="repo.recent" large />
      </div>
      <div v-if="repo.recent.length">
        <div class="text-faint text-xs font-[650] tracking-wider uppercase">Passed</div>
        <div class="font-mono text-lg font-semibold">{{ passed }}<span class="text-faint"> / {{ repo.recent.length }}</span></div>
      </div>
      <div v-if="typical !== null">
        <div class="text-faint text-xs font-[650] tracking-wider uppercase">Typical duration</div>
        <div class="font-mono text-lg font-semibold">{{ formatDuration(typical) }}</div>
      </div>
      <div v-if="typeof repo.concurrencyLimit === 'number'">
        <div class="text-faint text-xs font-[650] tracking-wider uppercase">Concurrency</div>
        <div class="font-mono text-lg font-semibold">{{ repo.concurrencyLimit }}</div>
      </div>
      <div v-if="repo.lastRun">
        <div class="text-faint text-xs font-[650] tracking-wider uppercase">Last run</div>
        <div class="flex items-center gap-2">
          <StatusBadge :status="repo.lastRun.status" />
          <span class="text-muted text-sm"><RelTime :iso="repo.lastRun.startTime" /></span>
        </div>
      </div>
    </div>

    <div class="mb-3 flex flex-wrap items-center gap-3">
      <div class="flex flex-wrap gap-2" role="group" aria-label="Filter by status">
        <button v-for="c in STATUS_CHIPS" :key="c.label" type="button" class="chip" :aria-pressed="status === c.value" @click="status = c.value">{{ c.label }}</button>
      </div>
      <form class="ml-auto flex flex-wrap items-center gap-2 max-[900px]:ml-0 max-[900px]:w-full" role="search" aria-label="Filter runs" @submit.prevent>
        <label class="sr-only" for="pr-filter">PR number</label>
        <input id="pr-filter" v-model="pr" data-filter class="input w-[120px] max-[900px]:flex-1" inputmode="numeric" placeholder="PR #  ( / )" autocomplete="off" />
        <label class="sr-only" for="branch-filter">Branch</label>
        <input id="branch-filter" v-model="branch" class="input w-[170px] max-[900px]:flex-1" placeholder="Branch" autocomplete="off" />
      </form>
    </div>

    <StateMessage :error="error" :loading="loading" :empty="runs.length === 0" :empty-text="filtered ? 'No runs match these filters.' : 'No runs for this repository.'" @retry="refresh">
      <button v-if="filtered" type="button" class="btn sm mt-3" @click="clearFilters">Clear filters</button>
    </StateMessage>
    <RunList v-if="runs.length" :runs="runs" :typical-of="() => typical" />
    <div v-if="runs.length >= limit" class="mt-3">
      <button type="button" class="btn" :disabled="loading" @click="limit += PAGE">Show more</button>
    </div>
  </section>
</template>
