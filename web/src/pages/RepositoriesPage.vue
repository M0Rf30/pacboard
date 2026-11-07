<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '@/api/client'
import Duration from '@/components/Duration.vue'
import ExtLink from '@/components/ExtLink.vue'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RelTime from '@/components/RelTime.vue'
import StateMessage from '@/components/StateMessage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { useLive } from '@/composables/useLive'
import { useResource } from '@/composables/useResource'

const { data, error, loading, refresh } = useResource((signal) => api.repositories(signal))
const live = useLive(() => void refresh(), (e) => e.kind !== 'TaskRun')

const filter = ref('')
const rows = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const all = data.value ?? []
  if (!q) return all
  return all.filter((r) => `${r.namespace}/${r.name} ${r.url}`.toLowerCase().includes(q))
})

const th = 'px-2 py-1.5 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400'
const td = 'px-2 py-1.5 align-top'
</script>

<template>
  <section>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h1 class="text-xl font-semibold">Repositories</h1>
      <LiveIndicator :live="live" />
      <label class="ml-auto flex items-center gap-2 text-sm">
        <span class="sr-only">Filter repositories</span>
        <input
          v-model="filter"
          type="search"
          placeholder="Filter repositories…"
          class="w-64 rounded border border-gray-300 bg-white px-2 py-1 dark:border-gray-700 dark:bg-gray-900"
        />
      </label>
    </div>

    <StateMessage :error="error" :loading="loading" :empty="rows.length === 0" :empty-text="filter ? 'No repository matches the filter.' : 'No repositories found.'" />

    <div v-if="rows.length" class="overflow-x-auto rounded border border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
      <table class="w-full border-collapse text-sm">
        <caption class="sr-only">Repositories, most recently active first</caption>
        <thead class="border-b border-gray-200 dark:border-gray-800">
          <tr>
            <th scope="col" :class="th">Repository</th>
            <th scope="col" :class="th">Last run</th>
            <th scope="col" :class="th">Started</th>
            <th scope="col" :class="th">Duration</th>
            <th scope="col" :class="th">Event</th>
            <th scope="col" :class="th">PR</th>
            <th scope="col" :class="`${th} text-right`">Runs</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
          <tr v-for="r in rows" :key="`${r.namespace}/${r.name}`" class="hover:bg-gray-50 dark:hover:bg-gray-800/50">
            <td :class="td">
              <RouterLink :to="{ name: 'repo-runs', params: { namespace: r.namespace, name: r.name } }" class="font-medium text-blue-700 hover:underline dark:text-blue-400">{{ r.name }}</RouterLink>
              <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">{{ r.namespace }}</span>
              <div v-if="r.url" class="text-xs"><ExtLink :href="r.url">{{ r.url.replace(/^https?:\/\//, '') }}</ExtLink></div>
            </td>
            <template v-if="r.lastRun">
              <td :class="td">
                <RouterLink :to="{ name: 'run', params: { namespace: r.lastRun.namespace, name: r.lastRun.name } }">
                  <StatusBadge :status="r.lastRun.status" :reason="r.lastRun.message || r.lastRun.reason" />
                </RouterLink>
              </td>
              <td :class="td"><RelTime :iso="r.lastRun.startTime" /></td>
              <td :class="td"><Duration :start="r.lastRun.startTime" :end="r.lastRun.completionTime" :status="r.lastRun.status" :seconds="r.lastRun.durationSeconds" /></td>
              <td :class="td">
                <span v-if="r.lastRun.git.eventType" class="whitespace-nowrap rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">{{ r.lastRun.git.eventType }}</span>
              </td>
              <td :class="td">
                <ExtLink v-if="r.lastRun.git.prNumber !== null" :href="r.lastRun.git.prUrl" :aria-label="`Pull request ${r.lastRun.git.prNumber} on GitHub`">#{{ r.lastRun.git.prNumber }}</ExtLink>
                <span v-else class="text-gray-400">—</span>
              </td>
            </template>
            <td v-else :class="td" colspan="5"><span class="text-gray-400">No runs</span></td>
            <td :class="`${td} text-right tabular-nums`">{{ r.runCount }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
