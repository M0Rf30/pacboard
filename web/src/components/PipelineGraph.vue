<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useElementSize, useEventListener } from '@vueuse/core'
import type { GraphEdge, GraphNode } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { formatDuration, liveDuration } from '@/lib/format'
import {
  clampScale,
  fitToWidth,
  hideSkippedDefault,
  layoutGraph,
  nearestNode,
  withoutSkipped,
  type Direction,
} from '@/lib/graph'
import { statusStyle } from '@/lib/status'
import Icon from './Icon.vue'

const props = defineProps<{ nodes: GraphNode[]; edges: GraphEdge[]; selected: string }>()
const emit = defineEmits<{ select: [id: string] }>()
/** Whether the graph is shown (the page toggles it with `g`). */
const open = defineModel<boolean>('open', { default: true })

const now = useNow()

const skippedCount = computed(() => props.nodes.filter((n) => n.status === 'Skipped').length)
const executedCount = computed(() => props.nodes.filter((n) => n.status !== 'Skipped' && n.status !== 'Pending').length)

// Pipelines whose tasks are mostly skipped (PR runs of big pipelines) start with the skipped ones hidden.
const hideSkipped = ref(hideSkippedDefault(props.nodes))

const shown = computed(() => (hideSkipped.value ? withoutSkipped(props.nodes, props.edges) : { nodes: props.nodes, edges: props.edges }))
const layout = computed(() => layoutGraph(shown.value.nodes, shown.value.edges))

// Zoom: null follows the container (zoom-to-fit by width, never clipped); a number is the user's own zoom.
const scroller = ref<HTMLElement | null>(null)
const { width: viewWidth } = useElementSize(scroller)
const userScale = ref<number | null>(null)
const fit = computed(() => fitToWidth(layout.value.width, layout.value.height, viewWidth.value))
const scale = computed(() => userScale.value ?? fit.value.scale)
const size = computed(() => ({ width: layout.value.width * scale.value, height: layout.value.height * scale.value }))
const zoomPercent = computed(() => Math.round(scale.value * 100))

function zoomBy(factor: number, anchor?: { x: number; y: number }): void {
  const el = scroller.value
  const before = scale.value
  const next = clampScale(before, factor)
  if (next === before) return
  userScale.value = next
  if (!el) return
  const ax = anchor?.x ?? el.clientWidth / 2
  const ay = anchor?.y ?? el.clientHeight / 2
  const ratio = next / before
  const px = el.scrollLeft + ax
  const py = el.scrollTop + ay
  void nextTick(() => {
    el.scrollLeft = px * ratio - ax
    el.scrollTop = py * ratio - ay
  })
}

function refit(): void {
  userScale.value = null
  void nextTick(() => scroller.value?.scrollTo({ left: 0, top: 0 }))
}

// Ctrl/⌘ + wheel zooms around the pointer; a plain wheel keeps scrolling the page.
useEventListener(
  scroller,
  'wheel',
  (ev: WheelEvent) => {
    if (!ev.ctrlKey && !ev.metaKey) return
    ev.preventDefault()
    const rect = scroller.value!.getBoundingClientRect()
    zoomBy(Math.exp(-ev.deltaY * 0.0025), { x: ev.clientX - rect.left, y: ev.clientY - rect.top })
  },
  { passive: false },
)

// A different graph (hide skipped toggled, run refreshed with other tasks) refits unless the user zoomed.
watch(() => layout.value.nodes.length, () => void nextTick(() => scroller.value?.scrollTo({ left: 0 })))

// Drag on the background to pan; nodes keep their click.
const dragging = ref(false)
let drag: { x: number; y: number; left: number; top: number; id: number } | null = null

function onPointerDown(ev: PointerEvent): void {
  const el = scroller.value
  if (!el || ev.button !== 0 || ev.pointerType === 'touch' || (ev.target as Element).closest('.gnode')) return
  drag = { x: ev.clientX, y: ev.clientY, left: el.scrollLeft, top: el.scrollTop, id: ev.pointerId }
  el.setPointerCapture(ev.pointerId)
  dragging.value = true
}
function onPointerMove(ev: PointerEvent): void {
  const el = scroller.value
  if (!drag || !el) return
  el.scrollLeft = drag.left - (ev.clientX - drag.x)
  el.scrollTop = drag.top - (ev.clientY - drag.y)
}
function onPointerUp(): void {
  if (drag) scroller.value?.releasePointerCapture(drag.id)
  drag = null
  dragging.value = false
}

const KEY_DIRECTION: Record<string, Direction> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }

function onNodeKey(ev: KeyboardEvent, id: string): void {
  if (ev.key === 'Enter' || ev.key === ' ') {
    ev.preventDefault()
    emit('select', id)
    return
  }
  const dir = KEY_DIRECTION[ev.key]
  if (!dir) return
  ev.preventDefault()
  const to = nearestNode(layout.value.nodes, id, dir)
  if (to) scroller.value?.querySelector<SVGGElement>(`[data-task="${CSS.escape(to)}"]`)?.focus()
}

function sub(n: GraphNode): string {
  const label = statusStyle(n.status).label
  if (n.status === 'Pending') return label
  return `${label} · ${formatDuration(liveDuration(n.startTime, n.completionTime, n.status, null, now.value))}`
}

function incident(e: { from: string; to: string }): boolean {
  return props.selected !== '' && (e.from === props.selected || e.to === props.selected)
}
function nodeLabel(n: GraphNode): string {
  const parts = [n.id, statusStyle(n.status).label]
  if (n.matrixCount > 1) parts.push(`matrix ×${n.matrixCount}`)
  if (n.finally) parts.push('finally task')
  return parts.join(', ')
}
</script>

<template>
  <section class="panel" aria-label="Pipeline">
    <div class="panel-h">
      <h2>Pipeline</h2>
      <span class="text-muted text-sm">
        {{ executedCount }} of {{ nodes.length }} tasks executed<template v-if="skippedCount"> · {{ skippedCount }} skipped</template>
      </span>
      <div class="text-muted ml-auto flex flex-wrap items-center gap-4 text-xs" aria-label="Legend">
        <span class="inline-flex items-center gap-1.5"><svg class="h-2 w-7" viewBox="0 0 28 8"><path d="M0 4h28" stroke="var(--line-strong)" stroke-width="1.6" /></svg>runs after</span>
        <span class="inline-flex items-center gap-1.5"><svg class="h-2 w-7" viewBox="0 0 28 8"><path d="M0 4h28" stroke="var(--line-strong)" stroke-width="1.6" stroke-dasharray="5 3" /></svg>uses a result</span>
        <span class="inline-flex items-center gap-1.5"><svg class="h-2 w-7" viewBox="0 0 28 8"><path d="M0 4h28" stroke="var(--line-strong)" stroke-width="1.8" stroke-dasharray="1.5 3.5" stroke-linecap="round" /></svg>finally</span>
      </div>
      <button v-if="skippedCount" type="button" class="chip" :aria-pressed="hideSkipped" @click="hideSkipped = !hideSkipped">Hide skipped</button>
      <div v-if="open && layout.nodes.length" class="flex items-center gap-1" role="group" aria-label="Zoom">
        <button type="button" class="chip !px-2" aria-label="Zoom out" @click="zoomBy(1 / 1.25)"><Icon name="minus" sm /></button>
        <button type="button" class="chip" :title="userScale === null ? 'Fitted to the width' : 'Fit to the width'" :aria-pressed="userScale === null" @click="refit">
          <Icon name="fit" sm />Fit <b>{{ zoomPercent }}%</b>
        </button>
        <button type="button" class="chip !px-2" aria-label="Zoom in" @click="zoomBy(1.25)"><Icon name="plus" sm /></button>
      </div>
      <button type="button" class="chip" :aria-expanded="open" aria-controls="graph-scroll" @click="open = !open">
        {{ open ? 'Collapse' : 'Show graph' }} <kbd>g</kbd>
      </button>
    </div>
    <div
      v-show="open"
      id="graph-scroll"
      ref="scroller"
      class="graph-scroll"
      :class="{ dragging }"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
    >
      <p v-if="layout.nodes.length === 0" class="text-muted py-2 text-sm">This run has no tasks yet.</p>
      <svg
        v-else
        :width="size.width"
        :height="size.height"
        :viewBox="`0 0 ${layout.width} ${layout.height}`"
        role="group"
        aria-label="Pipeline graph, left to right. Arrow keys move between tasks."
        class="block"
      >
        <path
          v-for="e in layout.edges"
          :key="`${e.from}>${e.to}`"
          class="edge"
          :class="[e.kind, incident(e) ? 'hi' : selected ? 'dim' : '']"
          :d="e.path"
          marker-end="url(#arrow)"
        />
        <g
          v-for="n in layout.nodes"
          :key="n.id"
          class="gnode"
          :class="[statusStyle(n.node.status).cls, { sel: selected === n.id }]"
          :data-task="n.id"
          tabindex="0"
          role="button"
          :aria-pressed="selected === n.id"
          :aria-label="nodeLabel(n.node)"
          @click="emit('select', n.id)"
          @keydown="onNodeKey($event, n.id)"
        >
          <title>{{ n.id }} — {{ n.node.status === 'Skipped' ? n.node.reason || 'Skipped' : statusStyle(n.node.status).label }}</title>
          <rect class="ring" :x="n.x - 4" :y="n.y - 4" :width="n.width + 8" :height="n.height + 8" rx="9" />
          <rect class="body" :x="n.x" :y="n.y" :width="n.width" :height="n.height" rx="6" />
          <g class="gicon" :transform="`translate(${n.x + 9} ${n.y + (n.node.status === 'Skipped' ? 9 : 8)})`">
            <svg class="ic" :class="{ spin: n.node.status === 'Running' }" width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"><use :href="`#i-${statusStyle(n.node.status).icon}`" /></svg>
          </g>
          <text class="nm" :x="n.x + 32" :y="n.y + (n.node.status === 'Skipped' ? 21 : 19)">{{ n.label }}</text>
          <text v-if="n.node.status !== 'Skipped'" class="st" :x="n.x + 32" :y="n.y + 35">{{ sub(n.node) }}</text>
          <template v-if="n.node.matrixCount > 1">
            <rect class="mxbg" :x="n.x + n.width - 30" :y="n.y - 8" width="30" height="16" rx="8" />
            <text class="mx" :x="n.x + n.width - 15" :y="n.y + 3.5" text-anchor="middle">×{{ n.node.matrixCount }}</text>
          </template>
          <text v-if="n.node.finally" class="fin" :x="n.x" :y="n.y - 6">finally</text>
        </g>
      </svg>
    </div>
  </section>
</template>
