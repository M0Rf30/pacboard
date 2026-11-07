<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { GraphNode } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { elapsedSeconds, formatDuration } from '@/lib/format'
import type { FocusRequest } from '@/lib/logview'
import { resolveTaskRun } from '@/lib/runview'
import Duration from './Duration.vue'
import LogViewer from './LogViewer.vue'
import StatusBadge from './StatusBadge.vue'
import StepTimeline from './StepTimeline.vue'

const props = defineProps<{
  namespace: string
  run: string
  node: GraphNode
  /** Selected step name; "" = all steps. */
  step: string
  /** Selected matrix TaskRun; "" = the first one. */
  taskRun: string
  focus: FocusRequest | null
}>()
const highlight = defineModel<number | null>('highlight', { default: null })
defineEmits<{ 'update:step': [step: string]; 'update:taskRun': [name: string] }>()

const now = useNow()

const skipped = computed(() => props.node.status === 'Skipped')
const selectedStep = computed(() => props.node.steps.find((s) => s.name === props.step))
const stepDidNotRun = computed(() => selectedStep.value?.status === 'Pending')
const active = computed(() => props.node.status !== 'Pending')
// Stream only what is still being written: the task while it runs, or its running step.
const follow = computed(() => props.node.status === 'Running' && (props.step === '' || selectedStep.value?.status === 'Running'))
const taskRunName = computed(() => resolveTaskRun(props.node, props.taskRun))
const stepSeconds = computed(() => {
  const s = selectedStep.value
  return s ? elapsedSeconds(s.startTime, s.completionTime, now.value) : null
})
// "Succeeded" needs no explanation next to the badge; a reason like "Failed" or "TaskRunTimeout" does.
const reason = computed(() => (props.node.reason && props.node.reason !== props.node.status ? props.node.reason : ''))
</script>

<template>
  <section class="panel" aria-label="Task details">
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line px-4 py-3">
      <h2 class="font-mono text-md">{{ node.id }}</h2>
      <StatusBadge :status="node.status" />
      <span v-if="node.matrixCount > 1" class="tag">matrix ×{{ node.matrixCount }}</span>
      <span v-if="node.finally" class="tag">finally</span>
      <span v-if="reason" class="text-muted text-sm">{{ reason }}</span>
      <span class="flex-1" />
      <template v-if="node.taskRunNames.length > 1">
        <label class="text-muted text-sm" for="taskrun-select">TaskRun</label>
        <select
          id="taskrun-select"
          class="input !h-7"
          :value="taskRunName"
          @change="$emit('update:taskRun', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="(t, i) in node.taskRunNames" :key="t" :value="t">#{{ i + 1 }} · {{ t }}</option>
        </select>
      </template>
      <span v-if="!skipped && node.startTime" class="text-muted font-mono text-sm"><Duration :start="node.startTime" :end="node.completionTime" :status="node.status" /></span>
    </div>

    <div v-if="skipped" class="text-muted px-5 py-6">
      <b class="text-text mb-1 block text-md">This task was skipped</b>
      {{ node.reason || 'Its condition was not met' }}. No TaskRun was created, so there are no steps or logs.
    </div>

    <div v-else class="split">
      <StepTimeline :node="node" :selected="step" @select="(s) => $emit('update:step', s)" />
      <div class="min-w-0">
        <div v-if="stepDidNotRun" class="text-muted px-5 py-6">
          <b class="text-text mb-1 block text-md">This step did not run</b>
          An earlier step in <span class="font-mono">{{ node.id }}</span> failed, so Tekton never started it.
        </div>
        <LogViewer
          v-else
          v-model:highlight="highlight"
          :namespace="namespace"
          :run="run"
          :task="node.id"
          :task-run="taskRunName"
          :step="step"
          :active="active"
          :follow="follow"
          :focus="focus"
        >
          <template #footer>
            <span v-if="selectedStep">step <span class="text-text">{{ selectedStep.name }}</span></span>
            <span v-else>all steps</span>
            <span v-if="selectedStep?.status === 'Failed' && selectedStep.exitCode !== null" class="text-fail">exit code {{ selectedStep.exitCode }}</span>
            <span v-if="selectedStep && selectedStep.startTime">{{ formatDuration(stepSeconds) }}</span>
          </template>
        </LogViewer>
      </div>
    </div>
  </section>
</template>
