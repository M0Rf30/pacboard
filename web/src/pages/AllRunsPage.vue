<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api } from '@/api/client'
import type { RunStatus, RunSummary } from '@/api/types'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RunList from '@/components/RunList.vue'
import StateMessage from '@/components/StateMessage.vue'
import { useLive } from '@/composables/useLive'
import { useQueryRef } from '@/composables/useQueryRef'
import { useResource } from '@/composables/useResource'

const PAGE = 100
const STATUS_CHIPS: { value: RunStatus | ''; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'Failed', label: 'Failed' },
  { value: 'Succeeded', label: 'Succeeded' },
  { value: 'Running', label: 'Running' },
  { value: 'Cancelled', label: 'Cancelled' },
]

const status = useQueryRef('status')
const text = ref('')
const limit = ref(PAGE)

// "Show more" applies to one filter; declared before the resource so the reset joins the same flush.
watch(status, () => (limit.value = PAGE))

const { data, error, loading, refresh } = useResource(
  (signal) => api.runs({ limit: limit.value, status: status.value as RunStatus | '' }, signal),
  [status, limit],
)
// Typical durations (median per repository) for the slow marker.
const { data: repos, refresh: refreshRepos } = useResource((signal) => api.repositories(signal))
const { connected, paused } = useLive(
  () => {
    void refresh()
    void refreshRepos()
  },
  (e) => e.kind === 'PipelineRun',
)

const typicals = computed(() => new Map((repos.value ?? []).map((r) => [`${r.namespace}/${r.name}`, r.typicalDurationSeconds])))
const typicalOf = (r: RunSummary): number | null => typicals.value.get(`${r.namespace}/${r.repository}`) ?? null

const runs = computed(() => {
  const all = data.value ?? []
  const q = text.value.trim().toLowerCase()
  if (!q) return all
  return all.filter((r) =>
    [r.repository, r.prName, r.name, r.git.branch, r.git.sourceBranch, r.git.shaTitle, r.git.sender, r.git.prNumber === null ? '' : `#${r.git.prNumber}`]
      .join(' ')
      .toLowerCase()
      .includes(q),
  )
})
const filtered = computed(() => text.value.trim() !== '' || status.value !== '')

function clearFilters(): void {
  status.value = ''
  text.value = ''
}
</script>

<template>
  <section>
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <h1>Runs</h1>
      <LiveIndicator v-model:paused="paused" :connected="connected" />
    </div>

    <div class="mb-3 flex flex-wrap items-center gap-3">
      <div class="flex flex-wrap gap-2" role="group" aria-label="Filter by status">
        <button v-for="c in STATUS_CHIPS" :key="c.label" type="button" class="chip" :aria-pressed="status === c.value" @click="status = c.value">{{ c.label }}</button>
      </div>
      <form class="ml-auto flex items-center gap-2 max-[900px]:ml-0 max-[900px]:w-full" role="search" aria-label="Filter runs" @submit.prevent>
        <label class="sr-only" for="run-filter">Search runs</label>
        <input id="run-filter" v-model="text" data-filter type="search" class="input w-72 max-[900px]:w-full" placeholder="repository, branch, commit, sender…  ( / )" autocomplete="off" />
      </form>
    </div>

    <StateMessage :error="error" :loading="loading" :empty="runs.length === 0" :empty-text="filtered ? 'No runs match these filters.' : 'No runs found.'" @retry="refresh">
      <button v-if="filtered" type="button" class="btn sm mt-3" @click="clearFilters">Clear filters</button>
    </StateMessage>
    <RunList v-if="runs.length" :runs="runs" :typical-of="typicalOf" show-repo />
    <div v-if="(data?.length ?? 0) >= limit" class="mt-3">
      <button type="button" class="btn" :disabled="loading" @click="limit += PAGE">Show more</button>
    </div>
  </section>
</template>
