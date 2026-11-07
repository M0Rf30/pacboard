// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import type { GraphEdge, GraphNode } from '@/api/types'
import {
  buildDagreGraph,
  clampScale,
  fitToWidth,
  hideSkippedDefault,
  layoutGraph,
  MAX_SCALE,
  MIN_SCALE,
  NODE_HEIGHT,
  NODE_WIDTH,
  nearestNode,
  nodeLabel,
  nodeSize,
  pathFromPoints,
  SKIPPED_HEIGHT,
  SKIPPED_WIDTH,
  withoutSkipped,
} from './graph'

function node(id: string, extra: Partial<GraphNode> = {}): GraphNode {
  return {
    id,
    taskRunName: `${id}-tr`,
    status: 'Succeeded',
    reason: '',
    finally: false,
    matrixCount: 1,
    taskRunNames: [`${id}-tr`],
    startTime: null,
    completionTime: null,
    steps: [],
    ...extra,
  }
}

const nodes = [node('clone'), node('build'), node('test'), node('lint'), node('report', { finally: true })]
const edges: GraphEdge[] = [
  { from: 'clone', to: 'build', kind: 'runAfter' },
  { from: 'clone', to: 'lint', kind: 'runAfter' },
  { from: 'build', to: 'test', kind: 'result' },
  { from: 'test', to: 'report', kind: 'finally' },
  { from: 'lint', to: 'report', kind: 'finally' },
]

describe('buildDagreGraph', () => {
  it('maps tasks to nodes and edges, left to right', () => {
    const g = buildDagreGraph(nodes, edges)
    expect(g.graph().rankdir).toBe('LR')
    expect([...g.nodes()].sort()).toEqual(['build', 'clone', 'lint', 'report', 'test'])
    expect(g.edges().map((e) => `${e.v}>${e.w}`).sort()).toEqual([
      'build>test',
      'clone>build',
      'clone>lint',
      'lint>report',
      'test>report',
    ])
    expect(g.edge('build', 'test')).toMatchObject({ kind: 'result' })
  })

  it('uses the spec sizes, widening only for long names', () => {
    expect(nodeSize(node('a'))).toEqual({ width: NODE_WIDTH, height: NODE_HEIGHT })
    expect(nodeSize(node('a', { status: 'Skipped' }))).toEqual({ width: SKIPPED_WIDTH, height: SKIPPED_HEIGHT })
    const long = 'a-very-long-pipeline-task-name-for-matrix'
    expect(nodeSize(node(long)).width).toBeGreaterThan(NODE_WIDTH)
    expect(buildDagreGraph([node('a'), node(long)], []).node(long).width).toBe(nodeSize(node(long)).width)
    expect(nodeLabel('lint-and-test')).toBe('lint-and-test')
    expect(nodeLabel('x'.repeat(80))).toHaveLength(Math.floor((300 - 44) / 7.4))
    expect(nodeLabel('x'.repeat(80)).endsWith('…')).toBe(true)
  })

  it('drops edges to unknown tasks and self loops, merges duplicates by kind priority', () => {
    const g = buildDagreGraph([node('a'), node('b')], [
      { from: 'a', to: 'ghost', kind: 'runAfter' },
      { from: 'b', to: 'b', kind: 'runAfter' },
      { from: 'a', to: 'b', kind: 'result' },
      { from: 'a', to: 'b', kind: 'runAfter' },
    ])
    expect(g.edgeCount()).toBe(1)
    expect(g.edge('a', 'b')).toMatchObject({ kind: 'runAfter' })
  })
})

describe('layoutGraph', () => {
  const layout = layoutGraph(nodes, edges)
  const at = (id: string) => layout.nodes.find((n) => n.id === id)!

  it('places dependencies to the left of their dependents', () => {
    expect(at('clone').x + at('clone').width).toBeLessThan(at('build').x)
    expect(at('build').x + at('build').width).toBeLessThan(at('test').x)
    expect(at('test').x + at('test').width).toBeLessThan(at('report').x)
  })

  it('keeps edge kinds and produces drawable paths', () => {
    expect(layout.edges).toHaveLength(5)
    const result = layout.edges.find((e) => e.from === 'build' && e.to === 'test')!
    expect(result.kind).toBe('result')
    expect(result.path.startsWith('M')).toBe(true)
    expect(result.points.length).toBeGreaterThanOrEqual(2)
  })

  it('handles an empty run', () => {
    expect(layoutGraph([], [])).toEqual({ nodes: [], edges: [], width: 0, height: 0 })
  })

  it('reports the extent of everything it placed', () => {
    for (const n of layout.nodes) {
      expect(n.x + n.width).toBeLessThanOrEqual(layout.width)
      expect(n.y + n.height).toBeLessThanOrEqual(layout.height)
      expect(n.x).toBeGreaterThanOrEqual(0)
      expect(n.y).toBeGreaterThanOrEqual(0)
    }
  })

  it('keeps matrix and status data on the laid-out node', () => {
    const l = layoutGraph([node('m', { matrixCount: 3, status: 'Failed' })], [])
    expect(l.nodes[0]!.node.matrixCount).toBe(3)
    expect(l.nodes[0]!.node.status).toBe('Failed')
  })
})

describe('pathFromPoints', () => {
  it('handles short and long point lists', () => {
    expect(pathFromPoints([])).toBe('')
    expect(pathFromPoints([{ x: 1, y: 2 }])).toBe('M1,2')
    expect(pathFromPoints([{ x: 0, y: 0 }, { x: 10, y: 10 }])).toBe('M0,0 C5,0 5,10 10,10')
    const d = pathFromPoints([{ x: 0, y: 0 }, { x: 5, y: 5 }, { x: 10, y: 0 }])
    expect(d.match(/C/g)).toHaveLength(2)
  })
})

describe('hideSkippedDefault', () => {
  const ns = (...status: GraphNode['status'][]) => status.map((st, i) => node(`n${i}`, { status: st }))

  it('is on when more than a third of the tasks are skipped', () => {
    expect(hideSkippedDefault(ns('Succeeded', 'Skipped', 'Skipped'))).toBe(true)
    expect(hideSkippedDefault(ns('Succeeded', 'Succeeded', 'Skipped', 'Skipped'))).toBe(true)
  })

  it('is off at exactly a third or below, and without skipped tasks', () => {
    expect(hideSkippedDefault(ns('Succeeded', 'Succeeded', 'Skipped'))).toBe(false)
    expect(hideSkippedDefault(ns('Succeeded', 'Failed'))).toBe(false)
    expect(hideSkippedDefault([])).toBe(false)
  })

  it('is off when nothing was executed, so the graph is never empty', () => {
    expect(hideSkippedDefault(ns('Skipped', 'Skipped'))).toBe(false)
  })
})

describe('withoutSkipped', () => {
  it('returns the graph unchanged without skipped tasks', () => {
    const r = withoutSkipped(nodes, edges)
    expect(r.nodes).toHaveLength(5)
    expect(r.edges).toEqual(edges)
  })

  it('drops skipped tasks and bridges through them, keeping the kind that entered the survivor', () => {
    const ns = [node('a'), node('b', { status: 'Skipped' }), node('c', { status: 'Skipped' }), node('fin', { finally: true })]
    const es: GraphEdge[] = [
      { from: 'a', to: 'b', kind: 'runAfter' },
      { from: 'b', to: 'c', kind: 'runAfter' },
      { from: 'c', to: 'fin', kind: 'finally' },
    ]
    const r = withoutSkipped(ns, es)
    expect(r.nodes.map((n) => n.id)).toEqual(['a', 'fin'])
    expect(r.edges).toEqual([{ from: 'a', to: 'fin', kind: 'finally' }])
  })

  it('does not loop on a cycle of skipped tasks', () => {
    const ns = [node('a', { status: 'Skipped' }), node('b', { status: 'Skipped' }), node('c')]
    const es: GraphEdge[] = [
      { from: 'a', to: 'b', kind: 'runAfter' },
      { from: 'b', to: 'a', kind: 'runAfter' },
      { from: 'b', to: 'c', kind: 'runAfter' },
    ]
    expect(withoutSkipped(ns, es).edges).toEqual([])
  })

  it('skips a task that depends on skipped tasks only through the surviving ones', () => {
    const ns = [node('x'), node('s', { status: 'Skipped' }), node('y')]
    const es: GraphEdge[] = [
      { from: 'x', to: 's', kind: 'runAfter' },
      { from: 's', to: 'y', kind: 'result' },
      { from: 'x', to: 'y', kind: 'runAfter' },
    ]
    const r = withoutSkipped(ns, es)
    expect(r.edges).toEqual([
      { from: 'x', to: 'y', kind: 'result' },
      { from: 'x', to: 'y', kind: 'runAfter' },
    ])
  })
})

describe('fitToWidth', () => {
  it('shrinks a wide graph to the container width', () => {
    const f = fitToWidth(1600, 400, 800)
    expect(f.scale).toBe(0.5)
    expect(f.width).toBe(800)
    expect(f.height).toBe(200)
  })

  it('never upscales and never goes below the minimum', () => {
    expect(fitToWidth(400, 100, 1200).scale).toBe(1)
    expect(fitToWidth(100000, 100, 800).scale).toBe(MIN_SCALE)
  })

  it('stays 1:1 when the container is not measurable yet', () => {
    expect(fitToWidth(900, 300, 0).scale).toBe(1)
    expect(fitToWidth(0, 0, 500).scale).toBe(1)
  })
})

describe('clampScale', () => {
  it('multiplies inside the limits', () => {
    expect(clampScale(1, 1.25)).toBe(1.25)
    expect(clampScale(2.9, 2)).toBe(MAX_SCALE)
    expect(clampScale(0.25, 0.1)).toBe(MIN_SCALE)
  })
})

describe('nearestNode', () => {
  const l = layoutGraph(nodes, edges).nodes

  it('moves along the dependency direction', () => {
    expect(nearestNode(l, 'clone', 'right')).toMatch(/^(build|lint)$/)
    expect(nearestNode(l, 'build', 'right')).toBe('test')
    expect(nearestNode(l, 'test', 'left')).toBe('build')
  })

  it('moves between tasks stacked in a column', () => {
    const col = layoutGraph(
      [node('root'), node('a'), node('b')],
      [
        { from: 'root', to: 'a', kind: 'runAfter' },
        { from: 'root', to: 'b', kind: 'runAfter' },
      ],
    ).nodes
    const [top, bottom] = [...col.filter((n) => n.id !== 'root')].sort((x, y) => x.y - y.y)
    expect(nearestNode(col, top!.id, 'down')).toBe(bottom!.id)
    expect(nearestNode(col, bottom!.id, 'up')).toBe(top!.id)
    expect(nearestNode(col, top!.id, 'up')).toBeNull()
  })

  it('returns null at the edge or for an unknown task', () => {
    expect(nearestNode(l, 'clone', 'left')).toBeNull()
    expect(nearestNode(l, 'ghost', 'right')).toBeNull()
  })
})
