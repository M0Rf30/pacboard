<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useEventListener } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import Breadcrumb from '@/components/Breadcrumb.vue'
import FailureCallout from '@/components/FailureCallout.vue'
import LiveIndicator from '@/components/LiveIndicator.vue'
import PipelineGraph from '@/components/PipelineGraph.vue'
import RerunButtons from '@/components/RerunButtons.vue'
import RunHeader from '@/components/RunHeader.vue'
import StateMessage from '@/components/StateMessage.vue'
import TaskPanel from '@/components/TaskPanel.vue'
import { useLive } from '@/composables/useLive'
import { setQueryParams } from '@/composables/useQueryRef'
import { useMe } from '@/composables/useMe'
import { usePageTitle } from '@/composables/usePageTitle'
import { useResource } from '@/composables/useResource'
import { isTyping } from '@/lib/keys'
import type { FocusRequest } from '@/lib/logview'
import { failureTargets, landingSelection, neighbourStep, pickTask, resolveTaskRun, type Selection } from '@/lib/runview'

const props = defineProps<{ namespace: string; name: string }>()

const route = useRoute()
const router = useRouter()
const { me } = useMe()

const { data, error, errorStatus, loading, refresh } = useResource(
  (signal) => api.run(props.namespace, props.name, signal),
  [() => props.namespace, () => props.name],
)
const { connected, paused } = useLive(
  () => void refresh(),
  (e) => e.namespace === props.namespace && (e.run === props.name || (e.kind === 'PipelineRun' && e.name === props.name)),
)

type Key = keyof Selection | 'line'

function query(key: Key): string {
  const v = route.query[key]
  return typeof v === 'string' ? v : ''
}

// Same-tick writes (select + highlight) share one replace built from the router's current query.
function setQuery(patch: Partial<Record<Key, string>>): void {
  setQueryParams(router, patch as Record<string, string>)
}

function select(sel: Selection): void {
  setQuery({ task: sel.task, step: sel.step, taskRun: sel.taskRun, line: '' })
}

const summary = computed(() => data.value?.summary)
const failure = computed(() => data.value?.failure ?? null)
const nodes = computed(() => data.value?.nodes ?? [])
const selected = computed(() => nodes.value.find((n) => n.id === query('task')))
const selection = computed<Selection>(() => ({ task: query('task'), step: query('step'), taskRun: query('taskRun') }))
const highlight = computed<number | null>({
  get: () => {
    const n = Number.parseInt(query('line'), 10)
    return Number.isFinite(n) && n > 0 ? n : null
  },
  set: (n) => setQuery({ line: n === null ? '' : String(n) }),
})

// Where "Go to error line" and a failed landing send the log viewer.
const focus = ref<FocusRequest | null>(null)
let seq = 0
function requestError(sel: Selection): void {
  const node = nodes.value.find((n) => n.id === sel.task)
  if (!node) return
  focus.value = {
    seq: ++seq,
    task: sel.task,
    taskRun: resolveTaskRun(node, sel.taskRun),
    step: sel.step,
    line: failure.value?.line ?? null,
    error: true,
  }
}

// Open the task most worth looking at when the link names none: the failure, else the last task that ran.
let landed = false
watch(
  data,
  (d) => {
    if (!d || landed) return
    landed = true
    if (selected.value) return
    const sel = landingSelection(d.nodes, d.summary.status, d.failure)
    if (!sel.task) return
    select(sel)
    if (failureTargets(d.failure, sel, d.nodes.find((n) => n.id === sel.task))) requestError(sel)
  },
  { immediate: true },
)

usePageTitle(() => (summary.value ? summary.value.git.shaTitle || summary.value.prName || summary.value.name : ''))

function pick(id: string): void {
  const node = nodes.value.find((n) => n.id === id)
  if (!node) return
  const sel = pickTask(node, failure.value)
  select(sel)
  if (failureTargets(failure.value, sel, node)) requestError(sel)
}

function goToError(): void {
  const d = data.value
  const f = d?.failure
  if (!d || !f || !f.task) return
  const sel = landingSelection(d.nodes, 'Failed', f)
  const node = d.nodes.find((n) => n.id === sel.task)
  const cur = selection.value
  // Already there: leave the URL (and ?line=) alone and just re-reveal.
  const here =
    node !== undefined &&
    cur.task === sel.task &&
    cur.step === sel.step &&
    resolveTaskRun(node, cur.taskRun) === resolveTaskRun(node, sel.taskRun)
  if (!here) select(sel)
  requestError(sel)
}

const graphOpen = ref(true)

useEventListener(document, 'keydown', (ev: KeyboardEvent) => {
  if (isTyping(ev.target) || ev.metaKey || ev.ctrlKey || ev.altKey) return
  switch (ev.key) {
    case 'j':
    case 'k': {
      const node = selected.value
      if (!node) return
      const next = neighbourStep(node, query('step'), ev.key === 'j' ? 1 : -1)
      if (next !== null) setQuery({ step: next, line: '' })
      break
    }
    case 'e':
      goToError()
      break
    case 'g':
      graphOpen.value = !graphOpen.value
      break
    default:
      return
  }
  ev.preventDefault()
})

const showRerun = computed(() => {
  const d = data.value
  return !!me.value?.canRerun && !!d?.canRerun && d.summary.git.prNumber !== null && !!d.summary.git.prUrl
})
</script>

<template>
  <section>
    <Breadcrumb
      v-if="summary"
      :items="[
        { label: 'Repositories', to: '/' },
        { label: summary.repository, to: { name: 'repo-runs', params: { namespace: summary.namespace, name: summary.repository } } },
        { label: summary.name, mono: true },
      ]"
    />

    <StateMessage
      :error="error ? (errorStatus === 404 ? 'This run was not found. It may have been pruned from the cluster.' : error) : ''"
      :loading="loading"
      :empty="!data && !error"
      @retry="refresh"
    />

    <template v-if="data && summary">
      <RunHeader :summary="summary" :params="data.params" :typical="data.typicalDurationSeconds">
        <template #actions>
          <RerunButtons v-if="showRerun" :summary="summary" :login="me?.login ?? ''" />
          <span v-else-if="me && !me.canRerun" class="text-faint text-sm">Read-only deployment</span>
          <LiveIndicator v-model:paused="paused" :connected="connected" />
        </template>
      </RunHeader>

      <FailureCallout :summary="summary" :failure="failure" :nodes="data.nodes" @go-to-error="goToError" />

      <PipelineGraph v-model:open="graphOpen" :nodes="data.nodes" :edges="data.edges" :selected="selection.task" @select="pick" />

      <TaskPanel
        v-if="selected"
        v-model:highlight="highlight"
        :namespace="summary.namespace"
        :run="summary.name"
        :node="selected"
        :step="selection.step"
        :task-run="selection.taskRun"
        :focus="focus"
        @update:step="(s) => setQuery({ step: s, line: '' })"
        @update:task-run="(t) => setQuery({ taskRun: t, line: '' })"
      />
      <p v-else-if="data.nodes.length" class="text-muted mt-4 text-sm">Select a task in the graph to see its steps and logs.</p>
    </template>
  </section>
</template>
