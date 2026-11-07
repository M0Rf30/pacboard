// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import type { GraphEdge, GraphNode } from '@/api/types'
import { buildDagreGraph, layoutGraph, nodeLabel, nodeWidth, pathFromPoints } from './graph'

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

  it('sizes nodes from their label', () => {
    const g = buildDagreGraph([node('a'), node('a-very-long-pipeline-task-name-for-matrix')], [])
    expect(g.node('a').width).toBe(nodeWidth('a'))
    expect(g.node('a-very-long-pipeline-task-name-for-matrix').width).toBeGreaterThan(g.node('a').width)
    expect(nodeLabel('a-very-long-pipeline-task-name-for-matrix')).toHaveLength(30)
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

  it('draws a frame around the finally tasks only', () => {
    expect(layout.groups).toHaveLength(1)
    const grp = layout.groups[0]!
    const rep = at('report')
    expect(grp.label).toBe('finally')
    expect(grp.x).toBeLessThan(rep.x)
    expect(grp.x + grp.width).toBeGreaterThan(rep.x + rep.width)
    expect(grp.y).toBeLessThan(rep.y)
    expect(grp.y + grp.height).toBeGreaterThan(rep.y + rep.height)
    for (const id of ['clone', 'build', 'test', 'lint']) {
      const n = at(id)
      const inside = n.x < grp.x + grp.width && n.x + n.width > grp.x && n.y < grp.y + grp.height && n.y + n.height > grp.y
      expect(inside).toBe(false)
    }
    expect(layout.width).toBeGreaterThanOrEqual(grp.x + grp.width)
    expect(layout.height).toBeGreaterThanOrEqual(grp.y + grp.height)
  })

  it('has no frame without finally tasks, and handles an empty run', () => {
    expect(layoutGraph([node('a')], []).groups).toEqual([])
    expect(layoutGraph([], [])).toEqual({ nodes: [], edges: [], groups: [], width: 0, height: 0 })
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
