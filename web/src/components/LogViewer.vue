<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, shallowRef, triggerRef, watch } from 'vue'
import { useThrottleFn } from '@vueuse/core'
import { ApiError, logsUrl, streamLogs } from '@/api/client'
import {
  closedGroups,
  errorTargetIndex,
  filterLines,
  firstErrorIndex,
  focusMatches,
  hiddenLineCount,
  type FocusRequest,
  visibleFrom,
  visibleTail,
} from '@/lib/logview'
import { formatLocation, LogParser, MAX_LOG_LINES, type LogLevel, type LogLine, type Segment } from '@/lib/logparse'
import Icon from './Icon.vue'

const props = defineProps<{
  namespace: string
  run: string
  task: string
  taskRun: string
  /** Step name; "" streams all steps (separated by `==> step <==` headers). */
  step: string
  /** False while the task has not started (no pod yet): nothing is fetched. */
  active: boolean
  /** True while the task runs: asks the backend to stream (follow=1). */
  follow: boolean
  focus: FocusRequest | null
}>()
const highlight = defineModel<number | null>('highlight', { default: null })

type Phase = 'idle' | 'loading' | 'streaming' | 'done' | 'gone' | 'error'

const PAGE = 3000
const ANN_CLASS: Record<string, string> = { error: 's-failed', warning: 's-running', notice: 's-pending' }
const ANN_ICON: Record<string, string> = { error: 'failed', warning: 'warn', notice: 'unknown' }

const lines = shallowRef<LogLine[]>([])
const phase = ref<Phase>('idle')
const errorMessage = ref('')
/** Groups the user opened (true) or closed (false) by hand; the rest follow the default rule. */
const toggled = shallowRef<ReadonlyMap<number, boolean>>(new Map())
const query = ref('')
const annotationsOnly = ref(false)
const wrap = ref(true)
const following = ref(false)
const renderLimit = ref(PAGE)
const flashNo = ref<number | null>(null)
const scroller = ref<HTMLElement | null>(null)

// The parser's counters and group state are plain fields; every read below goes through `lines.value`
// first, so the `triggerRef` in publish() is what re-runs them.
let parser = new LogParser()
let controller: AbortController | undefined
let programmaticScroll = false
let pending: FocusRequest | null = null
const lastJump: Record<string, number> = {}

function isOpen(id: number): boolean {
  return toggled.value.get(id) ?? parser.openByDefault(id)
}

const closed = computed(() => {
  void lines.value
  return closedGroups(parser.groups, isOpen)
})
function groupLineCount(id: number): number {
  void lines.value
  return parser.groupLines(id)
}
const levels = computed(() => {
  void lines.value
  return { errors: parser.errors, warnings: parser.warnings }
})
const droppedLines = computed(() => (void lines.value, parser.dropped))
const filtering = computed(() => query.value.trim() !== '' || annotationsOnly.value)

// Without a filter only the window at the end is built: the counts come from the parser and the
// window is collected walking back from the last line.
const filtered = computed(() => (filtering.value ? filterLines(lines.value, query.value, annotationsOnly.value) : []))
const visibleCount = computed(() =>
  filtering.value
    ? filtered.value.length
    : lines.value.length - hiddenLineCount(parser.groups, closed.value, (id) => parser.groupLines(id)),
)
const shown = computed(() =>
  filtering.value ? filtered.value.slice(-renderLimit.value) : visibleTail(lines.value, closed.value, renderLimit.value),
)
const hiddenEarlier = computed(() => visibleCount.value - shown.value.length)

function scrollToEnd(): void {
  const el = scroller.value
  if (!el) return
  programmaticScroll = true
  el.scrollTop = el.scrollHeight
  requestAnimationFrame(() => (programmaticScroll = false))
}

function publish(): void {
  triggerRef(lines)
  tryFocus(false)
  if (following.value) void nextTick(scrollToEnd)
}
const publishThrottled = useThrottleFn(publish, 200, true, true)

function onScroll(): void {
  const el = scroller.value
  if (!el || programmaticScroll || !following.value) return
  if (el.scrollHeight - el.scrollTop - el.clientHeight > 48) following.value = false
}

function resume(): void {
  following.value = true
  void nextTick(scrollToEnd)
}

async function load(): Promise<void> {
  controller?.abort()
  // A request for another log than the one now shown is stale; one aimed at this log (it may have arrived
  // before the route switched the props over) stays pending.
  if (pending && !focusMatches(pending, props)) pending = null
  parser = new LogParser()
  lines.value = parser.lines
  toggled.value = new Map()
  renderLimit.value = PAGE
  errorMessage.value = ''
  following.value = props.follow
  flashNo.value = null

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
      parser.push(text)
      void publishThrottled()
    })
    if (mine.signal.aborted) return
    parser.flush()
    publish()
    phase.value = 'done'
    tryFocus(true)
  } catch (e) {
    if (mine.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return
    parser.flush()
    publish()
    tryFocus(true)
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

/** Index of the line a request points at, or -1 while the log does not have it yet. */
function resolve(f: FocusRequest, ls: readonly LogLine[], final: boolean): number {
  if (!f.error) return ls.findIndex((l) => l.no === f.line)
  if (f.line !== null) {
    const exact = ls.findIndex((l) => l.no === f.line)
    // The API's line may still be streaming in; only once the log is complete fall back to the first error.
    if (exact >= 0 || !final) return exact
  }
  return final ? errorTargetIndex(ls, null) : firstErrorIndex(ls)
}

function tryFocus(final: boolean): void {
  const f = pending
  if (!f || !focusMatches(f, props)) return
  const ls = lines.value
  const idx = resolve(f, ls, final)
  if (idx < 0) {
    if (final) pending = null
    return
  }
  pending = null
  reveal(ls[idx]!)
}

/** Opens the groups around the line, widens the window to include it, flags it and scrolls to it. */
function reveal(l: LogLine): void {
  query.value = ''
  annotationsOnly.value = false
  following.value = false
  if (l.path.length > 0) {
    const next = new Map(toggled.value)
    for (const id of l.path) next.set(id, true)
    toggled.value = next
  }
  const fromHere = visibleFrom(lines.value, closed.value, l)
  if (fromHere > renderLimit.value) renderLimit.value = fromHere + 50
  highlight.value = l.no
  flashNo.value = l.no
  void nextTick(() => scrollToLine(l.no))
}

function scrollToLine(no: number): void {
  const el = scroller.value
  const row = el?.querySelector<HTMLElement>(`[data-no="${no}"]`)
  if (!el || !row) return
  programmaticScroll = true
  el.scrollTop = Math.max(0, row.offsetTop - el.clientHeight / 2 + row.offsetHeight / 2)
  requestAnimationFrame(() => (programmaticScroll = false))
  // The panel may sit below the fold: bring the row into the window too (explicit reveals only).
  row.scrollIntoView({ block: 'center', behavior: 'auto' })
}

watch(
  () => props.focus,
  (f) => {
    if (!f) return
    pending = f
    tryFocus(phase.value === 'done')
  },
  { immediate: true },
)

// A permalink (?line=N) scrolls to its line once, when the log first has it.
if (highlight.value !== null && props.focus === null) {
  pending = { seq: 0, task: props.task, taskRun: props.taskRun, step: props.step, line: highlight.value, error: false }
}

function jumpTo(level: LogLevel): void {
  const ls = lines.value
  const hits: number[] = []
  ls.forEach((l, i) => {
    if (l.level === level) hits.push(i)
  })
  if (hits.length === 0) return
  const at = hits.find((i) => i > (lastJump[level] ?? -1)) ?? hits[0]!
  lastJump[level] = at
  reveal(ls[at]!)
}

function toggle(id: number): void {
  const next = new Map(toggled.value)
  next.set(id, !isOpen(id))
  toggled.value = next
}

function markLine(l: LogLine): void {
  highlight.value = highlight.value === l.no ? null : l.no
}

// The viewer keeps only the tail of a long log, so "Download raw" fetches the complete text again and
// lets the browser stream it to disk (a snapshot: no follow). Nothing raw is retained in memory.
const downloadUrl = computed(() =>
  logsUrl({
    namespace: props.namespace,
    run: props.run,
    task: props.task,
    taskRun: props.taskRun || undefined,
    step: props.step || undefined,
  }),
)
const downloadName = computed(() => `${[props.run, props.task, props.step].filter(Boolean).join('-')}.log`)
const canDownload = computed(() => lines.value.length > 0)

function segStyle(s: Segment): Record<string, string> | undefined {
  if (!s.color && !s.bg) return undefined
  const st: Record<string, string> = {}
  if (s.color) st.color = s.color
  if (s.bg) st.backgroundColor = s.bg
  return st
}

function indent(l: LogLine): Record<string, string> | undefined {
  const depth = filtering.value ? 0 : l.path.length
  return depth > 0 ? { paddingLeft: `${depth * 16}px` } : undefined
}

const statusText = computed(() => {
  switch (phase.value) {
    case 'loading':
      return 'Loading…'
    case 'streaming':
      return props.follow ? 'Streaming…' : 'Loading…'
    default:
      return `${(lines.value.length + droppedLines.value).toLocaleString()} lines`
  }
})
const showLog = computed(() => phase.value !== 'idle' && phase.value !== 'gone' && !(phase.value === 'error' && lines.value.length === 0))
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-col">
    <div class="border-line flex flex-wrap items-center gap-2 border-b px-3 py-2">
      <label class="sr-only" for="log-filter">Filter log lines</label>
      <input
        id="log-filter"
        v-model="query"
        data-filter
        type="search"
        class="input w-[220px] max-[900px]:w-full"
        placeholder="Filter log lines…  ( / )"
        autocomplete="off"
      />
      <button v-if="levels.errors" type="button" class="chip tinted s-failed" :title="`Jump to the next error (${levels.errors})`" @click="jumpTo('error')">
        <Icon name="failed" sm /><b>{{ levels.errors }}</b> {{ levels.errors === 1 ? 'error' : 'errors' }}
      </button>
      <button v-if="levels.warnings" type="button" class="chip tinted s-running" :title="`Jump to the next warning (${levels.warnings})`" @click="jumpTo('warning')">
        <Icon name="warn" sm /><b>{{ levels.warnings }}</b> {{ levels.warnings === 1 ? 'warning' : 'warnings' }}
      </button>
      <button type="button" class="chip" :aria-pressed="annotationsOnly" @click="annotationsOnly = !annotationsOnly">Annotations only</button>
      <button type="button" class="chip" :aria-pressed="wrap" @click="wrap = !wrap">Wrap</button>
      <span class="text-faint hidden text-xs min-[1100px]:inline"><kbd>j</kbd> <kbd>k</kbd> step · <kbd>/</kbd> filter · <kbd>e</kbd> error</span>
      <a v-if="canDownload" :href="downloadUrl" :download="downloadName" class="btn sm ml-auto hover:no-underline">
        <Icon name="down" sm />Download raw
      </a>
      <button v-else type="button" class="btn sm ml-auto" disabled>
        <Icon name="down" sm />Download raw
      </button>
    </div>

    <p v-if="droppedLines > 0" role="status" class="bg-raised border-line text-muted border-b px-4 py-1.5 text-xs">
      Log truncated: the first {{ droppedLines.toLocaleString() }} lines are not shown (this view keeps the last {{ MAX_LOG_LINES.toLocaleString() }}).
      <a :href="downloadUrl" :download="downloadName">Download raw</a> for the complete log.
    </p>

    <p v-if="phase === 'idle'" class="text-muted px-4 py-6 text-sm">This task has not started yet, so there are no logs.</p>
    <p v-else-if="phase === 'gone'" class="text-muted px-4 py-6 text-sm">Logs are no longer available: the pod was removed from the cluster.</p>
    <p v-else-if="phase === 'error'" role="alert" class="s-failed px-4 py-6 text-sm text-(--c)">Could not load the logs: {{ errorMessage }}</p>

    <div v-if="showLog" class="relative min-h-0">
      <div
        ref="scroller"
        class="log"
        :class="{ nowrap: !wrap }"
        tabindex="0"
        role="region"
        aria-label="Task log"
        @scroll.passive="onScroll"
      >
        <button v-if="hiddenEarlier > 0" type="button" class="text-accent bg-raised block w-full px-4 py-1 text-left hover:underline" @click="renderLimit += PAGE">
          Show {{ Math.min(PAGE, hiddenEarlier) }} earlier lines ({{ hiddenEarlier.toLocaleString() }} hidden)
        </button>
        <p v-if="phase === 'done' && lines.length === 0" class="text-muted px-4 py-3">No output.</p>
        <p v-else-if="phase === 'done' && filtering && visibleCount === 0" class="text-muted px-4 py-3">No line matches.</p>
        <template v-for="l in shown" :key="l.id">
          <button
            v-if="l.kind === 'group' && !filtering"
            type="button"
            class="g"
            :style="l.path.length ? { top: `${l.path.length * 23}px`, zIndex: 10 - l.path.length } : undefined"
            :aria-expanded="isOpen(l.groupId)"
            :data-no="l.no"
            @click="toggle(l.groupId)"
          >
            <span class="ln">{{ l.no }}</span>
            <span class="tt" :style="indent(l)">
              <Icon name="chev" sm />
              <span class="min-w-0 truncate" :class="{ 'font-mono': !l.step }">
                <template v-for="(s, i) in l.segments" :key="i">{{ s.text }}</template>
              </span>
              <span class="cnt">{{ groupLineCount(l.groupId).toLocaleString() }} lines</span>
            </span>
          </button>
          <div
            v-else
            class="r"
            :class="[ANN_CLASS[l.level], { ann: l.level && l.level !== 'debug', hl: highlight === l.no, flash: flashNo === l.no, dim: l.level === 'debug' }]"
            :data-no="l.no"
            @animationend="flashNo = null"
          >
            <button type="button" class="ln" tabindex="-1" :aria-label="`Line ${l.no}`" :title="highlight === l.no ? 'Clear the highlight' : 'Highlight this line'" @click="markLine(l)">{{ l.no }}</button>
            <span class="tx" :style="indent(l)">
              <template v-if="l.kind === 'group'">
                <b class="b">{{ l.segments.map((s) => s.text).join('') }}</b>
              </template>
              <template v-else-if="l.level && l.level !== 'debug'">
                <span class="lv"><Icon :name="ANN_ICON[l.level] ?? 'unknown'" sm />{{ l.level }}</span>
                <span class="m">
                  <template v-for="(s, i) in l.segments" :key="i">
                    <a v-if="s.href" :href="s.href" target="_blank" rel="noopener noreferrer nofollow" :class="s.cls" :style="segStyle(s)">{{ s.text }}</a>
                    <span v-else :class="s.cls" :style="segStyle(s)">{{ s.text }}</span>
                  </template>
                </span>
                <span v-if="formatLocation(l.location)" class="f">{{ formatLocation(l.location) }}</span>
              </template>
              <template v-else>
                <template v-for="(s, i) in l.segments" :key="i">
                  <a v-if="s.href" :href="s.href" target="_blank" rel="noopener noreferrer nofollow" :class="s.cls" :style="segStyle(s)">{{ s.text }}</a>
                  <span v-else :class="s.cls" :style="segStyle(s)">{{ s.text }}</span>
                </template>
              </template>
            </span>
          </div>
        </template>
      </div>
      <button
        v-if="follow && !following && phase !== 'done'"
        type="button"
        class="btn primary absolute bottom-3 left-1/2 -translate-x-1/2 shadow-pop"
        @click="resume"
      >
        <Icon name="down" sm />Jump to latest
      </button>
    </div>

    <div class="border-line bg-surface text-muted flex flex-wrap items-center gap-x-5 gap-y-1 border-t px-4 py-1.5 font-mono text-xs">
      <span role="status">{{ statusText }}</span>
      <span v-if="follow && following && phase !== 'done'" class="text-run">● following</span>
      <slot name="footer" />
    </div>
  </div>
</template>
