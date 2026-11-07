<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { refDebounced } from '@vueuse/core'
import { api } from '@/api/client'
import type { RunStatus } from '@/api/types'
import ExtLink from '@/components/ExtLink.vue'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RunTable from '@/components/RunTable.vue'
import StateMessage from '@/components/StateMessage.vue'
import { useLive } from '@/composables/useLive'
import { useQueryRef } from '@/composables/useQueryRef'
import { useResource } from '@/composables/useResource'
import { RUN_STATUSES } from '@/lib/status'

const props = defineProps<{ namespace: string; name: string }>()

const PAGE = 50
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

const live = useLive(
  () => void refresh(),
  (e) => e.kind !== 'TaskRun' && e.namespace === props.namespace && (e.repository === props.name || (e.kind === 'Repository' && e.name === props.name)),
)

const runs = computed(() => data.value ?? [])
const repoUrl = computed(() => runs.value.find((r) => r.git.repoUrl)?.git.repoUrl ?? '')
const filtered = computed(() => status.value !== '' || pr.value !== '' || branch.value !== '')
const fieldClass = 'rounded border border-gray-300 bg-white px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-900'
</script>

<template>
  <section>
    <nav aria-label="Breadcrumb" class="mb-1 text-sm text-gray-500 dark:text-gray-400">
      <RouterLink to="/" class="hover:underline">Repositories</RouterLink> /
    </nav>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h1 class="text-xl font-semibold">{{ name }}</h1>
      <span class="text-sm text-gray-500 dark:text-gray-400">{{ namespace }}</span>
      <ExtLink v-if="repoUrl" :href="repoUrl" class="text-sm">GitHub</ExtLink>
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
        PR number
        <input v-model="pr" type="text" inputmode="numeric" placeholder="e.g. 255" :class="`${fieldClass} w-28`" />
      </label>
      <label class="flex flex-col gap-0.5 text-xs text-gray-600 dark:text-gray-400">
        Branch
        <input v-model="branch" type="text" placeholder="e.g. main" :class="`${fieldClass} w-48`" />
      </label>
    </form>

    <StateMessage :error="error" :loading="loading" :empty="runs.length === 0" :empty-text="filtered ? 'No run matches the filters.' : 'No runs for this repository.'" />
    <RunTable v-if="runs.length" :runs="runs" />
    <div v-if="runs.length >= limit" class="mt-3">
      <button type="button" class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800" :disabled="loading" @click="limit += PAGE">Show more</button>
    </div>
  </section>
</template>
