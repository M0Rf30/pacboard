// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import dagre, { type Edge, type Graph } from '@dagrejs/dagre'
import type { EdgeKind, GraphEdge, GraphNode } from '@/api/types'

export interface Point {
  x: number
  y: number
}

export interface LayoutNode {
  id: string
  /** Top-left corner. */
  x: number
  y: number
  width: number
  height: number
  label: string
  node: GraphNode
}

export interface LayoutEdge {
  from: string
  to: string
  kind: EdgeKind
  points: Point[]
  /** SVG path data. */
  path: string
}

export interface GraphLayout {
  nodes: LayoutNode[]
  edges: LayoutEdge[]
  width: number
  height: number
}

/** Executed tasks: status colour, duration, steps. */
export const NODE_WIDTH = 172
export const NODE_HEIGHT = 46
/** Skipped tasks are smaller and muted: they carry less weight than executed work. */
export const SKIPPED_WIDTH = 132
export const SKIPPED_HEIGHT = 34
const MAX_WIDTH = 300
const CHAR_WIDTH = 7.4
// Icon column plus padding on both sides of the label.
const LABEL_CHROME = 44
const MARGIN_X = 8
const MARGIN_Y = 14

/** Zoom limits of the graph viewer. */
export const MIN_SCALE = 0.2
export const MAX_SCALE = 3

// When two edges join the same pair, the strongest kind wins the drawing.
const KIND_PRIORITY: Record<EdgeKind, number> = { runAfter: 0, finally: 1, result: 2 }

function isSkipped(n: GraphNode): boolean {
  return n.status === 'Skipped'
}

/** Box of a task: the spec's fixed size, widened only when the name would not fit (long task names). */
export function nodeSize(n: GraphNode): { width: number; height: number } {
  const skipped = isSkipped(n)
  const base = skipped ? SKIPPED_WIDTH : NODE_WIDTH
  const wanted = Math.ceil(n.id.length * CHAR_WIDTH + LABEL_CHROME)
  return { width: Math.min(MAX_WIDTH, Math.max(base, wanted)), height: skipped ? SKIPPED_HEIGHT : NODE_HEIGHT }
}

/** The name drawn in the box: ellipsised only when even the widest box cannot hold it. */
export function nodeLabel(id: string): string {
  const max = Math.floor((MAX_WIDTH - LABEL_CHROME) / CHAR_WIDTH)
  return id.length > max ? `${id.slice(0, max - 1)}…` : id
}

/**
 * Hide skipped tasks by default when more than a third of the tasks are skipped (typical of PR runs
 * of big pipelines). Never when nothing was executed: an all-skipped graph would be empty.
 */
export function hideSkippedDefault(nodes: readonly GraphNode[]): boolean {
  if (nodes.length === 0) return false
  const skipped = nodes.filter(isSkipped).length
  return skipped < nodes.length && skipped * 3 > nodes.length
}

/**
 * The graph without its skipped tasks. An edge that ran through a skipped task is bridged to the
 * nearest executed ancestor (keeping the kind of the edge that entered the survivor), so that e.g. a
 * `finally` task whose parents were all skipped stays attached to the graph instead of floating alone.
 */
export function withoutSkipped(
  nodes: readonly GraphNode[],
  edges: readonly GraphEdge[],
): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const hidden = new Set(nodes.filter(isSkipped).map((n) => n.id))
  if (hidden.size === 0) return { nodes: nodes.slice(), edges: edges.slice() }

  const parents = new Map<string, string[]>()
  for (const e of edges) {
    const list = parents.get(e.to)
    if (list) list.push(e.from)
    else parents.set(e.to, [e.from])
  }

  // Executed ancestors reachable from a task through skipped tasks only.
  const executedAncestors = (id: string, seen: Set<string>): string[] => {
    if (!hidden.has(id)) return [id]
    if (seen.has(id)) return []
    seen.add(id)
    return (parents.get(id) ?? []).flatMap((p) => executedAncestors(p, seen))
  }

  const kept = nodes.filter((n) => !hidden.has(n.id))
  const out: GraphEdge[] = []
  const seenEdges = new Set<string>()
  const add = (from: string, to: string, kind: EdgeKind) => {
    const key = `${from}\u0000${to}\u0000${kind}`
    if (from === to || seenEdges.has(key)) return
    seenEdges.add(key)
    out.push({ from, to, kind })
  }
  for (const e of edges) {
    if (hidden.has(e.to)) continue
    for (const from of executedAncestors(e.from, new Set())) add(from, e.to, e.kind)
  }
  return { nodes: kept, edges: out }
}

/**
 * Maps the API graph onto a dagre graph: one node per task (sized by `nodeSize`), one edge per
 * connected pair (unknown endpoints and self loops dropped, duplicates merged by kind priority).
 */
export function buildDagreGraph(nodes: readonly GraphNode[], edges: readonly GraphEdge[]): Graph {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 14, ranksep: 60, marginx: MARGIN_X, marginy: MARGIN_Y, edgesep: 10 })
  g.setDefaultEdgeLabel(() => ({}))

  const ids = new Set<string>()
  for (const n of nodes) {
    ids.add(n.id)
    g.setNode(n.id, nodeSize(n))
  }

  const kinds = new Map<string, EdgeKind>()
  for (const e of edges) {
    if (e.from === e.to || !ids.has(e.from) || !ids.has(e.to)) continue
    const key = `${e.from}\u0000${e.to}`
    const prev = kinds.get(key)
    if (prev === undefined || KIND_PRIORITY[e.kind] < KIND_PRIORITY[prev]) kinds.set(key, e.kind)
  }
  for (const [key, kind] of kinds) {
    const [from, to] = key.split('\u0000') as [string, string]
    g.setEdge(from, to, { kind })
  }
  return g
}

/** Smooth path through the points (Catmull-Rom converted to cubic Béziers). */
export function pathFromPoints(pts: readonly Point[]): string {
  const first = pts[0]
  if (!first) return ''
  const f = (n: number) => Math.round(n * 10) / 10
  if (pts.length === 1) return `M${f(first.x)},${f(first.y)}`
  if (pts.length === 2) {
    const last = pts[1]!
    const mx = (first.x + last.x) / 2
    return `M${f(first.x)},${f(first.y)} C${f(mx)},${f(first.y)} ${f(mx)},${f(last.y)} ${f(last.x)},${f(last.y)}`
  }
  let d = `M${f(first.x)},${f(first.y)}`
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i]!
    const p1 = pts[i]!
    const p2 = pts[i + 1]!
    const p3 = pts[i + 2] ?? p2
    const c1 = { x: p1.x + (p2.x - p0.x) / 6, y: p1.y + (p2.y - p0.y) / 6 }
    const c2 = { x: p2.x - (p3.x - p1.x) / 6, y: p2.y - (p3.y - p1.y) / 6 }
    d += ` C${f(c1.x)},${f(c1.y)} ${f(c2.x)},${f(c2.y)} ${f(p2.x)},${f(p2.y)}`
  }
  return d
}

/** Lays the graph out left to right and returns everything the SVG needs. */
export function layoutGraph(nodes: readonly GraphNode[], edges: readonly GraphEdge[]): GraphLayout {
  if (nodes.length === 0) return { nodes: [], edges: [], width: 0, height: 0 }

  const g = buildDagreGraph(nodes, edges)
  dagre.layout(g)

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const laidOut: LayoutNode[] = []
  for (const id of g.nodes()) {
    const pos = g.node(id)
    const node = byId.get(id)
    if (!node) continue
    laidOut.push({
      id,
      x: pos.x - pos.width / 2,
      y: pos.y - pos.height / 2,
      width: pos.width,
      height: pos.height,
      label: nodeLabel(id),
      node,
    })
  }

  const laidEdges: LayoutEdge[] = g.edges().map((e: Edge) => {
    const label = g.edge(e) as { kind: EdgeKind; points?: Point[] }
    const points = label.points ?? []
    return { from: e.v, to: e.w, kind: label.kind, points, path: pathFromPoints(points) }
  })

  const gl = g.graph()
  return { nodes: laidOut, edges: laidEdges, width: gl.width ?? 0, height: gl.height ?? 0 }
}

/**
 * Zoom-to-fit by width: the largest scale (never above 1:1) at which the whole graph is as wide as the
 * container, so a long pipeline is never clipped on load. `width`/`height` are the scaled pixel size
 * of the drawing. Without a measurable container the graph stays at 1:1.
 */
export function fitToWidth(
  contentWidth: number,
  contentHeight: number,
  viewWidth: number,
): { scale: number; width: number; height: number } {
  const raw = contentWidth > 0 && viewWidth > 0 ? Math.min(1, viewWidth / contentWidth) : 1
  const scale = Math.max(MIN_SCALE, raw)
  return { scale, width: contentWidth * scale, height: contentHeight * scale }
}

/** `scale` multiplied by `factor`, kept inside the zoom limits. */
export function clampScale(scale: number, factor: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale * factor))
}

export type Direction = 'left' | 'right' | 'up' | 'down'

/**
 * The task nearest to `fromId` in an arrow-key direction: strictly beyond it along the axis, ranked by
 * distance along the axis plus a penalty for drifting off it. null when nothing lies that way.
 */
export function nearestNode(nodes: readonly LayoutNode[], fromId: string, dir: Direction): string | null {
  const from = nodes.find((n) => n.id === fromId)
  if (!from) return null
  const cx = from.x + from.width / 2
  const cy = from.y + from.height / 2
  let best: string | null = null
  let bestScore = Infinity
  for (const n of nodes) {
    if (n.id === fromId) continue
    const dx = n.x + n.width / 2 - cx
    const dy = n.y + n.height / 2 - cy
    let along: number
    let across: number
    switch (dir) {
      case 'right':
        along = dx
        across = dy
        break
      case 'left':
        along = -dx
        across = dy
        break
      case 'down':
        along = dy
        across = dx
        break
      case 'up':
        along = -dy
        across = dx
        break
    }
    if (along <= 0) continue
    const score = along + Math.abs(across) * 1.6
    if (score < bestScore) {
      bestScore = score
      best = n.id
    }
  }
  return best
}
