<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '@/api/client'
import type { RunStatus } from '@/api/types'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RunTable from '@/components/RunTable.vue'
import StateMessage from '@/components/StateMessage.vue'
import { useLive } from '@/composables/useLive'
import { useQueryRef } from '@/composables/useQueryRef'
import { useResource } from '@/composables/useResource'
import { RUN_STATUSES } from '@/lib/status'

const PAGE = 100
const status = useQueryRef('status')
const text = ref('')
const limit = ref(PAGE)

const { data, error, loading, refresh } = useResource(
  (signal) => api.runs({ limit: limit.value, status: status.value as RunStatus | '' }, signal),
  [status, limit],
)
const live = useLive(() => void refresh(), (e) => e.kind === 'PipelineRun')

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
const fieldClass = 'rounded border border-gray-300 bg-white px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-900'
</script>

<template>
  <section>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h1 class="text-xl font-semibold">Runs</h1>
      <LiveIndicator :live="live" />
    </div>

    <form class="mb-3 flex flex-wrap items-end gap-3" role="search" aria-label="Filter runs" @submit.prevent>
      <label class="flex flex-col gap-0.5 text-xs text-gray-600 dark:text-gray-400">
        Status
        <select v-model="status" :class="fieldClass">
          <option value="">All</option>
          <option v-for="s in RUN_STATUSES" :key="s" :value="s">{{ s }}</option>
        </select>
      </label>
      <label class="flex flex-col gap-0.5 text-xs text-gray-600 dark:text-gray-400">
        Search
        <input v-model="text" type="search" placeholder="repository, branch, commit, sender…" :class="`${fieldClass} w-72`" />
      </label>
    </form>

    <StateMessage :error="error" :loading="loading" :empty="runs.length === 0" :empty-text="text || status ? 'No run matches the filters.' : 'No runs found.'" />
    <RunTable v-if="runs.length" :runs="runs" show-repo />
    <div v-if="(data?.length ?? 0) >= limit" class="mt-3">
      <button type="button" class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800" :disabled="loading" @click="limit += PAGE">Show more</button>
    </div>
  </section>
</template>
