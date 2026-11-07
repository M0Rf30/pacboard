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

export interface LayoutGroup {
  label: string
  x: number
  y: number
  width: number
  height: number
}

export interface GraphLayout {
  nodes: LayoutNode[]
  edges: LayoutEdge[]
  /** Frame drawn around the `finally` tasks. */
  groups: LayoutGroup[]
  width: number
  height: number
}

export const NODE_HEIGHT = 52
export const MAX_LABEL = 30
const MIN_WIDTH = 150
const MAX_WIDTH = 280
const CHAR_WIDTH = 7.4
const GROUP_PAD = 14
const GROUP_HEADER = 18
const MARGIN_X = 24
const MARGIN_Y = 36

// When two edges join the same pair, the strongest kind wins the drawing.
const KIND_PRIORITY: Record<EdgeKind, number> = { runAfter: 0, finally: 1, result: 2 }

export function nodeLabel(id: string): string {
  return id.length > MAX_LABEL ? `${id.slice(0, MAX_LABEL - 1)}…` : id
}

export function nodeWidth(id: string): number {
  const w = Math.ceil(nodeLabel(id).length * CHAR_WIDTH + 64)
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, w))
}

/**
 * Maps the API graph onto a dagre graph: one node per task (sized from its label), one edge per
 * connected pair (unknown endpoints and self loops dropped, duplicates merged by kind priority).
 */
export function buildDagreGraph(nodes: readonly GraphNode[], edges: readonly GraphEdge[]): Graph {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 22, ranksep: 64, marginx: MARGIN_X, marginy: MARGIN_Y, edgesep: 12 })
  g.setDefaultEdgeLabel(() => ({}))

  const ids = new Set<string>()
  for (const n of nodes) {
    ids.add(n.id)
    g.setNode(n.id, { width: nodeWidth(n.id), height: NODE_HEIGHT })
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
  if (nodes.length === 0) return { nodes: [], edges: [], groups: [], width: 0, height: 0 }

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

  const groups: LayoutGroup[] = []
  const fin = laidOut.filter((n) => n.node.finally)
  if (fin.length > 0) {
    const x0 = Math.min(...fin.map((n) => n.x)) - GROUP_PAD
    const y0 = Math.min(...fin.map((n) => n.y)) - GROUP_PAD - GROUP_HEADER
    const x1 = Math.max(...fin.map((n) => n.x + n.width)) + GROUP_PAD
    const y1 = Math.max(...fin.map((n) => n.y + n.height)) + GROUP_PAD
    groups.push({ label: 'finally', x: x0, y: y0, width: x1 - x0, height: y1 - y0 })
  }

  const gl = g.graph()
  let width = gl.width ?? 0
  let height = gl.height ?? 0
  for (const grp of groups) {
    width = Math.max(width, grp.x + grp.width + MARGIN_X)
    height = Math.max(height, grp.y + grp.height + MARGIN_Y)
  }
  return { nodes: laidOut, edges: laidEdges, groups, width, height }
}
