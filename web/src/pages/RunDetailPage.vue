<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import Duration from '@/components/Duration.vue'
import ExtLink from '@/components/ExtLink.vue'
import LiveIndicator from '@/components/LiveIndicator.vue'
import RelTime from '@/components/RelTime.vue'
import RerunPanel from '@/components/RerunPanel.vue'
import StateMessage from '@/components/StateMessage.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import TaskGraph from '@/components/TaskGraph.vue'
import TaskPanel from '@/components/TaskPanel.vue'
import { useLive } from '@/composables/useLive'
import { useMe } from '@/composables/useMe'
import { useResource } from '@/composables/useResource'

const props = defineProps<{ namespace: string; name: string }>()

const route = useRoute()
const router = useRouter()
const { me } = useMe()

const { data, error, errorStatus, loading, refresh } = useResource(
  (signal) => api.run(props.namespace, props.name, signal),
  [() => props.namespace, () => props.name],
)
const live = useLive(
  () => void refresh(),
  (e) => e.namespace === props.namespace && (e.run === props.name || (e.kind === 'PipelineRun' && e.name === props.name)),
)

type Selection = 'task' | 'taskRun' | 'step'

function query(key: Selection): string {
  const v = route.query[key]
  return typeof v === 'string' ? v : ''
}

// One replace for several keys: separate writes would each start from the same stale route.query.
function setSelection(patch: Partial<Record<Selection, string>>): void {
  const q = { ...route.query }
  for (const [k, v] of Object.entries(patch)) {
    if (v) q[k] = v
    else delete q[k]
  }
  void router.replace({ query: q })
}

const summary = computed(() => data.value?.summary)
const git = computed(() => summary.value?.git)
const selectedNode = computed(() => data.value?.nodes.find((n) => n.id === query('task')))

// Open the task most worth looking at when the link names none: the first failure, else a running task.
let picked = false
watch(data, (d) => {
  if (!d || picked) return
  picked = true
  if (query('task')) return
  const node = d.nodes.find((n) => n.status === 'Failed') ?? d.nodes.find((n) => n.status === 'Running')
  if (node) setSelection({ task: node.id })
}, { immediate: true })

watch(summary, (s) => {
  if (s) document.title = `${s.prName || s.name} · pacboard`
}, { immediate: true })

const canRerun = computed(() => !!data.value?.canRerun && git.value?.prNumber !== null && !!git.value?.prUrl)

const branchUrl = computed(() => {
  const g = git.value
  if (!g?.repoUrl || !g.branch) return ''
  return `${g.repoUrl}/tree/${g.branch.split('/').map(encodeURIComponent).join('/')}`
})
const senderUrl = computed(() => {
  const g = git.value
  if (!g?.sender || g.provider !== 'github') return ''
  return `https://github.com/${encodeURIComponent(g.sender.replace(/\[bot\]$/, ''))}`
})
const repoLabel = computed(() => git.value?.repoUrl.replace(/^https?:\/\/[^/]+\//, '') ?? '')

const dt = 'text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400'
</script>

<template>
  <section>
    <nav v-if="summary" aria-label="Breadcrumb" class="mb-1 text-sm text-gray-500 dark:text-gray-400">
      <RouterLink to="/" class="hover:underline">Repositories</RouterLink> /
      <RouterLink :to="{ name: 'repo-runs', params: { namespace: summary.namespace, name: summary.repository } }" class="hover:underline">{{ summary.repository }}</RouterLink> /
    </nav>

    <StateMessage
      :error="error ? (errorStatus === 404 ? 'This run was not found. It may have been pruned from the cluster.' : error) : ''"
      :loading="loading"
      :empty="!data && !error"
    />

    <template v-if="data && summary && git">
      <header class="mb-4 rounded border border-gray-200 bg-white p-4 dark:border-gray-800 dark:bg-gray-900">
        <div class="flex flex-wrap items-center gap-3">
          <h1 class="text-xl font-semibold">{{ summary.prName || summary.name }}</h1>
          <StatusBadge :status="summary.status" :reason="summary.reason" />
          <span class="text-sm text-gray-600 dark:text-gray-400">
            <Duration :start="summary.startTime" :end="summary.completionTime" :status="summary.status" :seconds="summary.durationSeconds" />
            · started <RelTime :iso="summary.startTime" />
          </span>
          <LiveIndicator class="ml-auto" :live="live" />
        </div>
        <p class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ summary.namespace }}/{{ summary.name }}</p>
        <p v-if="summary.message" class="mt-2 text-sm text-gray-700 dark:text-gray-300">{{ summary.reason }}: {{ summary.message }}</p>

        <dl class="mt-4 grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
          <div v-if="git.repoUrl">
            <dt :class="dt">Repository</dt>
            <dd class="text-sm"><ExtLink :href="git.repoUrl">{{ repoLabel }}</ExtLink></dd>
          </div>
          <div v-if="git.prNumber !== null">
            <dt :class="dt">Pull request</dt>
            <dd class="text-sm">
              <ExtLink :href="git.prUrl">#{{ git.prNumber }}</ExtLink>
              <span v-if="git.sourceBranch" class="ml-1.5 font-mono text-xs text-gray-600 dark:text-gray-400">{{ git.sourceBranch }} → {{ git.branch }}</span>
            </dd>
          </div>
          <div v-if="git.sha">
            <dt :class="dt">Commit</dt>
            <dd class="text-sm">
              <ExtLink :href="git.shaUrl" class="font-mono">{{ git.shortSha }}</ExtLink>
              <span v-if="git.shaTitle" class="ml-1.5">{{ git.shaTitle }}</span>
            </dd>
          </div>
          <div v-if="git.branch">
            <dt :class="dt">Branch</dt>
            <dd class="text-sm"><ExtLink :href="branchUrl" class="font-mono">{{ git.branch }}</ExtLink></dd>
          </div>
          <div v-if="git.checkRunUrl">
            <dt :class="dt">Check run</dt>
            <dd class="text-sm"><ExtLink :href="git.checkRunUrl">View check run</ExtLink></dd>
          </div>
          <div v-if="git.sender">
            <dt :class="dt">Triggered by</dt>
            <dd class="text-sm">
              <ExtLink :href="senderUrl">{{ git.sender }}</ExtLink>
              <span v-if="git.eventType" class="ml-1.5 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">{{ git.eventType }}</span>
            </dd>
          </div>
        </dl>

        <div v-if="canRerun" class="mt-4 border-t border-gray-100 pt-3 dark:border-gray-800">
          <RerunPanel :summary="summary" :login="me?.login ?? ''" />
        </div>

        <details v-if="data.params.length" class="mt-4">
          <summary class="cursor-pointer text-sm font-medium">Parameters ({{ data.params.length }})</summary>
          <table class="mt-2 w-full border-collapse text-sm">
            <caption class="sr-only">PipelineRun parameters</caption>
            <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
              <tr v-for="p in data.params" :key="p.name">
                <th scope="row" class="w-1/4 whitespace-nowrap px-2 py-1 text-left align-top font-mono text-xs font-medium">{{ p.name }}</th>
                <td class="break-all px-2 py-1 font-mono text-xs text-gray-700 dark:text-gray-300">{{ p.value }}</td>
              </tr>
            </tbody>
          </table>
        </details>
      </header>

      <h2 class="mb-2 text-base font-semibold">Pipeline</h2>
      <TaskGraph
        :nodes="data.nodes"
        :edges="data.edges"
        :selected="selectedNode?.id ?? ''"
        @select="(id) => setSelection({ task: id, taskRun: '', step: '' })"
      />

      <TaskPanel
        v-if="selectedNode"
        :key="selectedNode.id"
        :namespace="summary.namespace"
        :run="summary.name"
        :node="selectedNode"
        :task-run="query('taskRun')"
        :step="query('step')"
        @update:task-run="(n) => setSelection({ taskRun: n, step: '' })"
        @update:step="(n) => setSelection({ step: n })"
      />
      <p v-else-if="data.nodes.length" class="mt-4 text-sm text-gray-600 dark:text-gray-400">Select a task in the graph to see its steps and logs.</p>
    </template>
  </section>
</template>
