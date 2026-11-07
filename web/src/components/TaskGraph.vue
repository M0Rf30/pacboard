<!--
SPDX-FileCopyrightText: The pacboard Authors

SPDX-License-Identifier: AGPL-3.0-or-later
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useElementSize, useEventListener } from '@vueuse/core'
import type { GraphEdge, GraphNode, NodeStatus } from '@/api/types'
import { useNow } from '@/composables/useNow'
import { formatDuration, liveDuration } from '@/lib/format'
import { layoutGraph, NODE_HEIGHT, type LayoutEdge } from '@/lib/graph'
import { statusGlyph } from '@/lib/status'

const props = defineProps<{ nodes: GraphNode[]; edges: GraphEdge[]; selected: string }>()
const emit = defineEmits<{ select: [id: string] }>()

const now = useNow()
const layout = computed(() => layoutGraph(props.nodes, props.edges))
// Statuses change on every refresh; only a change of the task set or wiring should re-fit the view.
const structure = computed(() => `${props.nodes.map((n) => n.id).join(',')}|${props.edges.map((e) => `${e.from}>${e.to}`).join(',')}`)

const MIN_SCALE = 0.2
const MAX_SCALE = 3
const container = ref<HTMLElement | null>(null)
const { width: viewW, height: viewH } = useElementSize(container)
const tx = ref(0)
const ty = ref(0)
const scale = ref(1)
let fitted = ''

function fit(): void {
  const l = layout.value
  if (!l.width || !viewW.value || !viewH.value) return
  const s = Math.min(1, viewW.value / l.width, viewH.value / l.height)
  scale.value = s
  tx.value = (viewW.value - l.width * s) / 2
  ty.value = (viewH.value - l.height * s) / 2
}

watch([structure, viewW, viewH], () => {
  if (!viewW.value || !viewH.value) return
  if (fitted !== structure.value) {
    fitted = structure.value
    fit()
  }
}, { immediate: true, flush: 'post' })

function zoomAt(factor: number, cx: number, cy: number): void {
  const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale.value * factor))
  const k = next / scale.value
  tx.value = cx - (cx - tx.value) * k
  ty.value = cy - (cy - ty.value) * k
  scale.value = next
}

function zoomCentre(factor: number): void {
  zoomAt(factor, viewW.value / 2, viewH.value / 2)
}

// A plain wheel scrolls the page; only Ctrl/⌘+wheel (also what trackpad pinch emits) zooms the graph.
function onWheel(ev: WheelEvent): void {
  if (!ev.ctrlKey && !ev.metaKey) return
  ev.preventDefault()
  const rect = container.value?.getBoundingClientRect()
  if (!rect) return
  zoomAt(Math.exp(-ev.deltaY * 0.0015), ev.clientX - rect.left, ev.clientY - rect.top)
}

let drag: { x: number; y: number; tx: number; ty: number; moved: boolean } | null = null
let suppressClick = false

function onPointerDown(ev: PointerEvent): void {
  if (ev.button !== 0) return
  drag = { x: ev.clientX, y: ev.clientY, tx: tx.value, ty: ty.value, moved: false }
}

useEventListener(window, 'pointermove', (ev: PointerEvent) => {
  if (!drag) return
  const dx = ev.clientX - drag.x
  const dy = ev.clientY - drag.y
  if (!drag.moved && Math.hypot(dx, dy) < 4) return
  drag.moved = true
  tx.value = drag.tx + dx
  ty.value = drag.ty + dy
})

useEventListener(window, 'pointerup', () => {
  if (drag?.moved) {
    suppressClick = true
    setTimeout(() => (suppressClick = false), 0)
  }
  drag = null
})

function select(id: string): void {
  if (!suppressClick) emit('select', id)
}

function onKey(ev: KeyboardEvent): void {
  const step = 40
  switch (ev.key) {
    case 'ArrowLeft': tx.value += step; break
    case 'ArrowRight': tx.value -= step; break
    case 'ArrowUp': ty.value += step; break
    case 'ArrowDown': ty.value -= step; break
    case '+': case '=': zoomCentre(1.2); break
    case '-': zoomCentre(1 / 1.2); break
    case '0': fit(); break
    default: return
  }
  ev.preventDefault()
}

const NODE_CLASSES: Record<NodeStatus, string> = {
  Succeeded: 'fill-green-50 stroke-green-600 dark:fill-green-950 dark:stroke-green-500',
  Failed: 'fill-red-50 stroke-red-600 dark:fill-red-950 dark:stroke-red-500',
  Running: 'fill-blue-50 stroke-blue-600 dark:fill-blue-950 dark:stroke-blue-400 graph-running',
  Pending: 'fill-gray-100 stroke-gray-400 dark:fill-gray-800 dark:stroke-gray-500',
  Skipped: 'fill-transparent stroke-gray-400 dark:stroke-gray-500',
  Cancelled: 'fill-orange-50 stroke-orange-500 dark:fill-orange-950 dark:stroke-orange-400',
  Unknown: 'fill-gray-100 stroke-gray-400 dark:fill-gray-800 dark:stroke-gray-500',
}

const GLYPH_CLASSES: Record<NodeStatus, string> = {
  Succeeded: 'fill-green-700 dark:fill-green-400',
  Failed: 'fill-red-700 dark:fill-red-400',
  Running: 'fill-blue-700 dark:fill-blue-400',
  Pending: 'fill-gray-500 dark:fill-gray-400',
  Skipped: 'fill-gray-500 dark:fill-gray-400',
  Cancelled: 'fill-orange-700 dark:fill-orange-400',
  Unknown: 'fill-gray-500 dark:fill-gray-400',
}

function nodeClass(status: NodeStatus): string {
  return NODE_CLASSES[status] ?? NODE_CLASSES.Unknown
}

function nodeDash(status: NodeStatus): string | undefined {
  return status === 'Skipped' ? '6 4' : undefined
}

function edgeDash(kind: LayoutEdge['kind']): string | undefined {
  if (kind === 'result') return '7 4'
  if (kind === 'finally') return '2 4'
  return undefined
}

function edgeClass(e: LayoutEdge): string {
  const hot = props.selected !== '' && (e.from === props.selected || e.to === props.selected)
  if (hot) return 'stroke-blue-500'
  return e.kind === 'finally' ? 'stroke-gray-300 dark:stroke-gray-600' : 'stroke-gray-400 dark:stroke-gray-500'
}

function detail(n: GraphNode): string {
  const secs = liveDuration(n.startTime, n.completionTime, n.status, null, now.value)
  return secs === null ? n.status : `${n.status} · ${formatDuration(secs)}`
}

const edgeKindDescription: Record<LayoutEdge['kind'], string> = {
  runAfter: 'runs after',
  result: 'uses result of',
  finally: 'finally after',
}
</script>

<template>
  <div class="relative rounded border border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900">
    <div class="absolute right-2 top-2 z-10 flex gap-1">
      <button type="button" class="h-7 w-7 rounded border border-gray-300 bg-white text-sm hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800" aria-label="Zoom in" @click="zoomCentre(1.25)">+</button>
      <button type="button" class="h-7 w-7 rounded border border-gray-300 bg-white text-sm hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800" aria-label="Zoom out" @click="zoomCentre(0.8)">−</button>
      <button type="button" class="h-7 rounded border border-gray-300 bg-white px-2 text-xs hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800" aria-label="Fit graph to view" @click="fit">Fit</button>
    </div>

    <div
      ref="container"
      class="h-80 touch-none select-none overflow-hidden"
      @wheel="onWheel"
      @pointerdown="onPointerDown"
    >
      <p v-if="layout.nodes.length === 0" class="p-4 text-sm text-gray-500 dark:text-gray-400">This run has no tasks.</p>
      <svg
        v-else
        class="h-full w-full cursor-grab active:cursor-grabbing"
        role="group"
        aria-label="Task graph. Arrow keys pan, plus and minus zoom, 0 fits."
        tabindex="0"
        @keydown="onKey"
      >
        <defs>
          <marker id="graph-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L10,5 L0,10 z" class="fill-gray-400 dark:fill-gray-500" />
          </marker>
        </defs>
        <g :transform="`translate(${tx} ${ty}) scale(${scale})`">
          <g v-for="grp in layout.groups" :key="grp.label">
            <rect :x="grp.x" :y="grp.y" :width="grp.width" :height="grp.height" rx="10" fill="none" stroke-dasharray="3 3" class="stroke-gray-400 dark:stroke-gray-600" />
            <text :x="grp.x + 10" :y="grp.y + 14" class="fill-gray-500 text-[11px] font-semibold uppercase tracking-wide dark:fill-gray-400">{{ grp.label }}</text>
          </g>

          <path
            v-for="e in layout.edges"
            :key="`${e.from}>${e.to}`"
            :d="e.path"
            fill="none"
            stroke-width="1.5"
            :stroke-dasharray="edgeDash(e.kind)"
            marker-end="url(#graph-arrow)"
            :class="edgeClass(e)"
          >
            <title>{{ e.to }} {{ edgeKindDescription[e.kind] }} {{ e.from }}</title>
          </path>

          <g
            v-for="n in layout.nodes"
            :key="n.id"
            :transform="`translate(${n.x} ${n.y})`"
            class="cursor-pointer"
            role="button"
            tabindex="0"
            :aria-pressed="selected === n.id"
            :aria-label="`Task ${n.id}: ${n.node.status}${n.node.matrixCount > 1 ? `, matrix of ${n.node.matrixCount}` : ''}${n.node.finally ? ', finally' : ''}`"
            @click="select(n.id)"
            @keydown.enter.prevent="select(n.id)"
            @keydown.space.prevent="select(n.id)"
          >
            <title>{{ n.id }}{{ n.node.reason ? ` — ${n.node.reason}` : '' }}</title>
            <rect
              :width="n.width"
              :height="NODE_HEIGHT"
              rx="8"
              :stroke-width="selected === n.id ? 3 : 1.5"
              :stroke-dasharray="nodeDash(n.node.status)"
              :class="nodeClass(n.node.status)"
            />
            <text x="12" y="22" font-size="14" :class="GLYPH_CLASSES[n.node.status]" aria-hidden="true">{{ statusGlyph(n.node.status) }}</text>
            <text x="30" y="21" class="fill-gray-900 text-[13px] font-semibold dark:fill-gray-100">{{ n.label }}</text>
            <text x="30" y="40" class="fill-gray-600 text-[11px] dark:fill-gray-400">{{ detail(n.node) }}</text>
            <g v-if="n.node.matrixCount > 1" :transform="`translate(${n.width - 38} -9)`">
              <rect width="34" height="18" rx="9" class="fill-gray-700 dark:fill-gray-300" />
              <text x="17" y="13" text-anchor="middle" class="fill-white text-[11px] font-semibold dark:fill-gray-900">×{{ n.node.matrixCount }}</text>
            </g>
          </g>
        </g>
      </svg>
    </div>

    <ul class="flex flex-wrap gap-x-4 gap-y-1 border-t border-gray-100 px-3 py-1.5 text-xs text-gray-500 dark:border-gray-800 dark:text-gray-400" aria-label="Legend">
      <li class="flex items-center gap-1.5"><svg width="28" height="6" aria-hidden="true"><line x1="0" y1="3" x2="28" y2="3" stroke="currentColor" stroke-width="1.5" /></svg>runs after</li>
      <li class="flex items-center gap-1.5"><svg width="28" height="6" aria-hidden="true"><line x1="0" y1="3" x2="28" y2="3" stroke="currentColor" stroke-width="1.5" stroke-dasharray="7 4" /></svg>uses a result</li>
      <li class="flex items-center gap-1.5"><svg width="28" height="6" aria-hidden="true"><line x1="0" y1="3" x2="28" y2="3" stroke="currentColor" stroke-width="1.5" stroke-dasharray="2 4" /></svg>finally</li>
      <li>Ctrl+scroll to zoom, drag to pan</li>
    </ul>
  </div>
</template>
