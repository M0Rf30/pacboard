<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, shallowRef, triggerRef, watch } from 'vue'
import { useThrottleFn } from '@vueuse/core'
import { ApiError, logsUrl, streamLogs } from '@/api/client'
import { LogParser, visibleLines, type LogLine, type Segment } from '@/lib/logparse'

const props = defineProps<{
  namespace: string
  run: string
  task: string
  taskRun: string
  /** Step name; "" streams all steps. */
  step: string
  /** False while the task has not started (no pod yet): nothing is fetched. */
  active: boolean
  /** True while the task runs: asks the backend to stream (follow=1). */
  follow: boolean
}>()

type Phase = 'idle' | 'loading' | 'streaming' | 'done' | 'gone' | 'error'

const PAGE = 3000

const lines = shallowRef<LogLine[]>([])
const phase = ref<Phase>('idle')
const errorMessage = ref('')
const collapsed = shallowRef<ReadonlySet<number>>(new Set())
const groupIds = ref<number[]>([])
const autoScroll = ref(false)
const renderLimit = ref(PAGE)
const scroller = ref<HTMLElement | null>(null)

let parser = new LogParser()
let raw: string[] = []
let controller: AbortController | undefined
let programmaticScroll = false

const visible = computed(() => visibleLines(lines.value, collapsed.value))
const shown = computed(() => (visible.value.length > renderLimit.value ? visible.value.slice(-renderLimit.value) : visible.value))
const hiddenEarlier = computed(() => visible.value.length - shown.value.length)

function scrollToEnd(): void {
  const el = scroller.value
  if (!el) return
  programmaticScroll = true
  el.scrollTop = el.scrollHeight
  requestAnimationFrame(() => (programmaticScroll = false))
}

function publish(): void {
  lines.value = parser.lines
  triggerRef(lines)
  groupIds.value = parser.groupIds()
  if (autoScroll.value) void nextTick(scrollToEnd)
}
const publishThrottled = useThrottleFn(publish, 200, true, true)

function onScroll(): void {
  const el = scroller.value
  if (!el || programmaticScroll || !autoScroll.value) return
  if (el.scrollHeight - el.scrollTop - el.clientHeight > 48) autoScroll.value = false
}

async function load(): Promise<void> {
  controller?.abort()
  parser = new LogParser()
  raw = []
  lines.value = []
  collapsed.value = new Set()
  groupIds.value = []
  renderLimit.value = PAGE
  errorMessage.value = ''
  autoScroll.value = props.follow

  if (!props.active) {
    phase.value = 'idle'
    return
  }

  const mine = new AbortController()
  controller = mine
  phase.value = 'loading'
  const url = logsUrl({
    namespace: props.namespace,
    run: props.run,
    task: props.task,
    taskRun: props.taskRun || undefined,
    step: props.step || undefined,
    follow: props.follow,
  })
  try {
    await streamLogs(url, mine.signal, (text) => {
      phase.value = 'streaming'
      raw.push(text)
      parser.push(text)
      void publishThrottled()
    })
    if (mine.signal.aborted) return
    parser.flush()
    publish()
    phase.value = 'done'
  } catch (e) {
    if (mine.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return
    parser.flush()
    publish()
    if (e instanceof ApiError && e.status === 404) {
      phase.value = 'gone'
    } else {
      phase.value = 'error'
      errorMessage.value = e instanceof Error ? e.message : String(e)
    }
  }
}

watch(
  [() => props.namespace, () => props.run, () => props.task, () => props.taskRun, () => props.step, () => props.active],
  () => void load(),
  { immediate: true },
)
onScopeDispose(() => controller?.abort())

function toggle(id: number): void {
  const next = new Set(collapsed.value)
  if (!next.delete(id)) next.add(id)
  collapsed.value = next
}

function collapseAll(): void {
  collapsed.value = new Set(groupIds.value)
}

function expandAll(): void {
  collapsed.value = new Set()
}

function download(): void {
  const blob = new Blob(raw, { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${[props.run, props.task, props.step].filter(Boolean).join('-')}.log`
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function segStyle(s: Segment): Record<string, string> | undefined {
  if (!s.color && !s.bg) return undefined
  const st: Record<string, string> = {}
  if (s.color) st.color = s.color
  if (s.bg) st.backgroundColor = s.bg
  return st
}

const LEVEL_CLASSES: Record<string, string> = {
  error: 'bg-red-50 text-red-800 dark:bg-red-950/60 dark:text-red-300',
  warning: 'bg-yellow-50 text-yellow-800 dark:bg-yellow-950/60 dark:text-yellow-300',
  notice: 'bg-blue-50 text-blue-800 dark:bg-blue-950/60 dark:text-blue-300',
  debug: 'text-gray-500 dark:text-gray-400',
}

function lineClass(l: LogLine): string {
  if (l.kind === 'group') return 'bg-gray-100 font-semibold dark:bg-gray-800'
  if (l.kind === 'step') return 'bg-gray-200 font-semibold dark:bg-gray-700'
  return LEVEL_CLASSES[l.level] ?? ''
}

const statusText = computed(() => {
  switch (phase.value) {
    case 'loading':
      return 'Loading…'
    case 'streaming':
      return props.follow ? 'Streaming…' : 'Loading…'
    case 'done':
      return `${lines.value.length} lines`
    default:
      return ''
  }
})
</script>

<template>
  <div class="rounded border border-gray-200 dark:border-gray-800">
    <div class="flex flex-wrap items-center gap-3 border-b border-gray-200 bg-gray-50 px-2 py-1 text-xs dark:border-gray-800 dark:bg-gray-900">
      <span class="text-gray-600 dark:text-gray-400" role="status">{{ statusText }}</span>
      <label class="ml-auto flex items-center gap-1.5">
        <input v-model="autoScroll" type="checkbox" @change="autoScroll && nextTick(scrollToEnd)" />
        Auto-scroll
      </label>
      <template v-if="groupIds.length">
        <button type="button" class="rounded px-1.5 py-0.5 hover:bg-gray-200 dark:hover:bg-gray-800" @click="collapseAll">Collapse groups</button>
        <button type="button" class="rounded px-1.5 py-0.5 hover:bg-gray-200 dark:hover:bg-gray-800" @click="expandAll">Expand groups</button>
      </template>
      <button type="button" class="rounded px-1.5 py-0.5 hover:bg-gray-200 disabled:opacity-40 dark:hover:bg-gray-800" :disabled="raw.length === 0 && lines.length === 0" @click="download">Download raw</button>
    </div>

    <p v-if="phase === 'idle'" class="px-3 py-4 text-sm text-gray-500 dark:text-gray-400">This task has not started yet, so there are no logs.</p>
    <p v-else-if="phase === 'gone'" class="px-3 py-4 text-sm text-gray-600 dark:text-gray-300">Logs are no longer available: the pod has been removed from the cluster.</p>
    <p v-else-if="phase === 'error'" role="alert" class="px-3 py-4 text-sm text-red-700 dark:text-red-300">Could not load the logs: {{ errorMessage }}</p>

    <div
      v-if="phase !== 'idle' && phase !== 'gone' && !(phase === 'error' && lines.length === 0)"
      ref="scroller"
      class="max-h-[32rem] min-h-16 overflow-auto bg-white font-mono text-xs dark:bg-gray-950"
      tabindex="0"
      role="log"
      aria-label="Task log"
      @scroll.passive="onScroll"
    >
      <button v-if="hiddenEarlier > 0" type="button" class="block w-full bg-gray-100 px-2 py-1 text-left text-blue-700 hover:underline dark:bg-gray-800 dark:text-blue-400" @click="renderLimit += PAGE">
        Show {{ Math.min(PAGE, hiddenEarlier) }} earlier lines ({{ hiddenEarlier }} hidden)
      </button>
      <p v-if="phase === 'done' && lines.length === 0" class="px-3 py-3 text-gray-500 dark:text-gray-400">No output.</p>
      <div v-for="l in shown" :key="l.id" class="flex min-h-5 gap-2 px-2 leading-5" :class="lineClass(l)">
        <span class="w-12 shrink-0 select-none text-right text-gray-400 dark:text-gray-500" aria-hidden="true">{{ l.no }}</span>
        <span class="min-w-0 grow whitespace-pre-wrap break-all" :style="{ paddingLeft: `${l.path.length * 16}px` }">
          <button
            v-if="l.kind === 'group'"
            type="button"
            class="mr-1 inline-block w-3 text-left"
            :aria-expanded="!collapsed.has(l.groupId)"
            :aria-label="`${collapsed.has(l.groupId) ? 'Expand' : 'Collapse'} group`"
            @click="toggle(l.groupId)"
          >{{ collapsed.has(l.groupId) ? '▸' : '▾' }}</button>
          <span v-if="l.level" class="mr-1.5 rounded bg-black/10 px-1 font-semibold uppercase dark:bg-white/10">{{ l.level }}</span>
          <template v-for="(s, i) in l.segments" :key="i">
            <a
              v-if="s.href"
              :href="s.href"
              target="_blank"
              rel="noopener noreferrer nofollow"
              class="underline"
              :class="s.cls"
              :style="segStyle(s)"
            >{{ s.text }}</a>
            <span v-else :class="s.cls" :style="segStyle(s)">{{ s.text }}</span>
          </template>
        </span>
      </div>
    </div>
  </div>
</template>
