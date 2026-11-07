<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { Failure, GraphNode, RunSummary } from '@/api/types'
import { elapsedSeconds, formatDuration } from '@/lib/format'
import { statusStyle } from '@/lib/status'
import Icon from './Icon.vue'
import StatusIcon from './StatusIcon.vue'

// The one-line answer to "what went wrong", above the graph. Failed runs show the API's failure; the
// other states get a quieter one-line summary so the page keeps the same shape.
const props = defineProps<{ summary: RunSummary; failure: Failure | null; nodes: GraphNode[] }>()
defineEmits<{ goToError: [] }>()

const executed = computed(() => props.nodes.filter((n) => n.status !== 'Skipped' && n.status !== 'Pending').length)
const skipped = computed(() => props.nodes.filter((n) => n.status === 'Skipped').length)
const finished = computed(() => props.nodes.filter((n) => n.status === 'Succeeded' || n.status === 'Failed' || n.status === 'Cancelled').length)

const failedNode = computed(() => props.nodes.find((n) => n.id === props.failure?.task))
const failedStep = computed(() => failedNode.value?.steps.find((s) => s.name === props.failure?.step))
const isFailure = computed(() => props.summary.status === 'Failed')

const headline = computed(() => {
  const s = props.summary.status
  if (s === 'Succeeded') return `${executed.value} ${executed.value === 1 ? 'task' : 'tasks'} succeeded`
  if (s === 'Running') return `${finished.value} of ${props.nodes.length} tasks finished`
  return statusStyle(s).label
})
const compact = computed(() => !isFailure.value)
const detail = computed(() => {
  const s = props.summary
  if (s.status === 'Succeeded') return [s.reason, s.message].filter(Boolean).join(': ') + (skipped.value ? ` · ${skipped.value} skipped` : '')
  return s.message || s.reason
})
</script>

<template>
  <section class="callout" :class="[statusStyle(summary.status).cls, { compact }]" :aria-label="isFailure ? 'Failure summary' : 'Run summary'">
    <div class="text-(--c) flex items-center gap-2 text-md font-[650]">
      <StatusIcon :status="summary.status" class="!h-5 !w-5" />
      <span v-if="isFailure && failure && failure.task">
        Failed in <span class="font-mono">{{ failure.task }}</span><template v-if="failure.step"> › <span class="font-mono">{{ failure.step }}</span></template>
      </span>
      <span v-else>{{ headline }}</span>
    </div>
    <div class="min-w-[240px] flex-1 text-sm">
      <template v-if="isFailure && failure">
        <div class="font-mono break-words">{{ failure.message || summary.message || summary.reason }}</div>
        <div class="text-muted mt-0.5">
          <template v-if="failure.exitCode !== null">exit code {{ failure.exitCode }}</template>
          <template v-if="failedStep && failedStep.startTime"><template v-if="failure.exitCode !== null"> · </template>{{ formatDuration(elapsedSeconds(failedStep.startTime, failedStep.completionTime, Date.now())) }}</template>
          <template v-if="failure.skippedBecause > 0"><template v-if="failure.exitCode !== null || failedStep"> · </template>{{ failure.skippedBecause }} {{ failure.skippedBecause === 1 ? 'task' : 'tasks' }} skipped because {{ failure.task || 'of the failure' }} failed</template>
        </div>
      </template>
      <div v-else class="text-muted">{{ detail }}</div>
    </div>
    <button v-if="isFailure && failure && failure.task" type="button" class="btn" @click="$emit('goToError')">
      <Icon name="down" sm />{{ failure.line !== null ? 'Go to error line' : 'Go to failed step' }} <kbd>e</kbd>
    </button>
  </section>
</template>
