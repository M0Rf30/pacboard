<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { GraphNode } from '@/api/types'
import Duration from './Duration.vue'
import LogViewer from './LogViewer.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  namespace: string
  run: string
  node: GraphNode
  /** Selected matrix TaskRun ("" = first). */
  taskRun: string
  /** Selected step ("" = all steps). */
  step: string
}>()
const emit = defineEmits<{ 'update:taskRun': [name: string]; 'update:step': [name: string] }>()

const effectiveTaskRun = computed(() => props.taskRun || props.node.taskRunNames[0] || props.node.taskRunName)
const isFirstTaskRun = computed(() => effectiveTaskRun.value === (props.node.taskRunNames[0] ?? props.node.taskRunName))
const stepNames = computed(() => new Set(props.node.steps.map((s) => s.name)))
// The API lists the steps of the first TaskRun only, so a step filter applies to that one.
const step = computed(() => (isFirstTaskRun.value && stepNames.value.has(props.step) ? props.step : ''))
const active = computed(() => props.node.taskRunName !== '' && props.node.status !== 'Pending' && props.node.status !== 'Skipped')
const follow = computed(() => props.node.status === 'Running')

function selectStep(name: string): void {
  emit('update:step', name)
}

const stepBtn =
  'flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-800'
</script>

<template>
  <section class="mt-4 rounded border border-gray-200 bg-white p-3 dark:border-gray-800 dark:bg-gray-900" aria-label="Selected task">
    <header class="mb-3 flex flex-wrap items-center gap-3">
      <h2 class="text-base font-semibold">{{ node.id }}</h2>
      <StatusBadge :status="node.status" :reason="node.reason" />
      <span v-if="node.finally" class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800">finally</span>
      <span v-if="node.matrixCount > 1" class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800">matrix ×{{ node.matrixCount }}</span>
      <span v-if="node.reason" class="text-sm text-gray-600 dark:text-gray-400">{{ node.reason }}</span>
      <span class="ml-auto text-sm text-gray-600 dark:text-gray-400">
        Duration
        <Duration :start="node.startTime" :end="node.completionTime" :status="node.status" />
      </span>
    </header>

    <div v-if="node.taskRunNames.length > 1" class="mb-3">
      <label class="flex items-center gap-2 text-sm">
        Matrix TaskRun
        <select
          class="rounded border border-gray-300 bg-white px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-900"
          :value="effectiveTaskRun"
          @change="emit('update:taskRun', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="(n, i) in node.taskRunNames" :key="n" :value="n">#{{ i + 1 }} · {{ n }}</option>
        </select>
      </label>
    </div>

    <p v-if="node.taskRunName === ''" class="text-sm text-gray-600 dark:text-gray-400">
      {{ node.status === 'Skipped' ? 'This task was skipped' : 'This task has no TaskRun yet' }}{{ node.reason ? ` (${node.reason})` : '' }}.
    </p>

    <div v-else class="grid gap-3 lg:grid-cols-[16rem_1fr]">
      <nav aria-label="Steps">
        <h3 class="mb-1 px-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">Steps</h3>
        <ul v-if="isFirstTaskRun" class="space-y-0.5">
          <li>
            <button type="button" :class="[stepBtn, { 'bg-gray-100 font-medium dark:bg-gray-800': step === '' }]" :aria-pressed="step === ''" @click="selectStep('')">All steps</button>
          </li>
          <li v-for="s in node.steps" :key="s.name">
            <button
              type="button"
              :class="[stepBtn, { 'bg-gray-100 font-medium dark:bg-gray-800': step === s.name }]"
              :aria-pressed="step === s.name"
              :title="s.reason"
              @click="selectStep(s.name)"
            >
              <span class="min-w-0 grow truncate">{{ s.name }}</span>
              <StatusBadge :status="s.status" />
              <Duration :start="s.startTime" :end="s.completionTime" :status="s.status" class="w-14 text-right text-xs text-gray-600 dark:text-gray-400" />
            </button>
          </li>
        </ul>
        <p v-else class="px-2 text-sm text-gray-600 dark:text-gray-400">Per-step details are shown for the first TaskRun; this log has all steps.</p>
      </nav>

      <LogViewer
        :namespace="namespace"
        :run="run"
        :task="node.id"
        :task-run="isFirstTaskRun ? '' : effectiveTaskRun"
        :step="step"
        :active="active"
        :follow="follow"
      />
    </div>
  </section>
</template>
