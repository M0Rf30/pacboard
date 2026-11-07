<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import type { RunSummary } from '@/api/types'
import Duration from './Duration.vue'
import ExtLink from './ExtLink.vue'
import RelTime from './RelTime.vue'
import StatusBadge from './StatusBadge.vue'

defineProps<{ runs: RunSummary[]; showRepo?: boolean }>()

const th = 'px-2 py-1.5 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400'
const td = 'px-2 py-1 align-top'
</script>

<template>
  <div class="overflow-x-auto rounded border border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
    <table class="w-full border-collapse text-sm">
      <caption class="sr-only">Pipeline runs, newest first</caption>
      <thead class="border-b border-gray-200 dark:border-gray-800">
        <tr>
          <th scope="col" :class="th">Status</th>
          <th scope="col" :class="th">Run</th>
          <th v-if="showRepo" scope="col" :class="th">Repository</th>
          <th scope="col" :class="th">PR</th>
          <th scope="col" :class="th">Branch</th>
          <th scope="col" :class="th">Commit</th>
          <th scope="col" :class="th">Sender</th>
          <th scope="col" :class="th">Event</th>
          <th scope="col" :class="th">Started</th>
          <th scope="col" :class="th">Duration</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
        <tr v-for="r in runs" :key="`${r.namespace}/${r.name}`" class="hover:bg-gray-50 dark:hover:bg-gray-800/50">
          <td :class="td"><StatusBadge :status="r.status" :reason="r.message || r.reason" /></td>
          <td :class="td">
            <RouterLink
              :to="{ name: 'run', params: { namespace: r.namespace, name: r.name } }"
              class="font-medium text-blue-700 hover:underline dark:text-blue-400"
              :title="r.name"
            >
              {{ r.prName || r.name }}
            </RouterLink>
          </td>
          <td v-if="showRepo" :class="td">
            <RouterLink
              :to="{ name: 'repo-runs', params: { namespace: r.namespace, name: r.repository } }"
              class="hover:underline"
            >
              {{ r.repository }}
            </RouterLink>
          </td>
          <td :class="td">
            <ExtLink v-if="r.git.prNumber !== null" :href="r.git.prUrl" :aria-label="`Pull request ${r.git.prNumber} on GitHub`">#{{ r.git.prNumber }}</ExtLink>
            <span v-else class="text-gray-400">—</span>
          </td>
          <td :class="td">
            <span class="font-mono text-xs" :title="r.git.sourceBranch ? `${r.git.sourceBranch} → ${r.git.branch}` : r.git.branch">{{ r.git.branch || '—' }}</span>
          </td>
          <td :class="td">
            <ExtLink :href="r.git.shaUrl" class="font-mono text-xs">{{ r.git.shortSha || '—' }}</ExtLink>
            <span class="ml-1.5 inline-block max-w-md truncate align-bottom text-gray-700 dark:text-gray-300" :title="r.git.shaTitle">{{ r.git.shaTitle }}</span>
          </td>
          <td :class="td">{{ r.git.sender || '—' }}</td>
          <td :class="td">
            <span v-if="r.git.eventType" class="whitespace-nowrap rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">{{ r.git.eventType }}</span>
            <span v-else class="text-gray-400">—</span>
          </td>
          <td :class="td"><RelTime :iso="r.startTime" /></td>
          <td :class="td"><Duration :start="r.startTime" :end="r.completionTime" :status="r.status" :seconds="r.durationSeconds" /></td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
