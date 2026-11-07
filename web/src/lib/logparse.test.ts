// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { formatLocation, LogParser, lineText, linkify, stripAnsi, type LogLine } from './logparse'

const ESC = '\x1b'

function parseLog(text: string, maxLines?: number): LogLine[] {
  const p = new LogParser(maxLines)
  p.push(text)
  p.flush()
  return p.lines
}

describe('ANSI colours', () => {
  it('maps the 16-colour palette and attributes to classes', () => {
    const [line] = parseLog(`${ESC}[1;31mboom${ESC}[0m ok`)
    expect(line!.segments.map((s) => [s.text, s.cls])).toEqual([
      ['boom', 'ansi-bold ansi-fg-1'],
      [' ok', ''],
    ])
  })

  it('maps bright colours to palette 8-15 and backgrounds', () => {
    const [line] = parseLog(`${ESC}[92;44mx`)
    expect(line!.segments[0]!.cls).toBe('ansi-fg-10 ansi-bg-4')
  })

  it('supports 256 and truecolor', () => {
    const [line] = parseLog(`${ESC}[38;5;196ma${ESC}[38;2;1;2;3mb${ESC}[48;5;244mc`)
    expect(line!.segments.map((s) => [s.text, s.color, s.bg])).toEqual([
      ['a', '#ff0000', ''],
      ['b', '#010203', ''],
      ['c', '#010203', '#808080'],
    ])
  })

  it('rejects out-of-range colour components', () => {
    const [line] = parseLog(`${ESC}[38;2;999;0;0mx`)
    expect(line!.segments[0]!.color).toBe('')
  })

  it('carries the style across lines until reset', () => {
    const lines = parseLog(`${ESC}[32mfirst\nsecond\n${ESC}[0mthird`)
    expect(lines.map((l) => l.segments[0]!.cls)).toEqual(['ansi-fg-2', 'ansi-fg-2', ''])
  })

  it('drops non-colour control sequences', () => {
    const [line] = parseLog(`a${ESC}[2K${ESC}[1Gb${ESC}]0;title\x07c`)
    expect(lineText(line!)).toBe('abc')
    expect(stripAnsi(`${ESC}[31mred${ESC}[0m`)).toBe('red')
  })

  it('keeps only the last carriage-return rewrite', () => {
    const [line] = parseLog('10%\r50%\r100%\r\n')
    expect(lineText(line!)).toBe('100%')
  })
})

describe('groups', () => {
  const log = ['before', '::group::Install', 'a', '::group::Nested', 'b', '::endgroup::', 'c', '::endgroup::', 'after'].join('\n')

  it('emits a header per group and nests paths', () => {
    const lines = parseLog(log)
    expect(lines.map((l) => [lineText(l), l.kind, l.path.join('/')])).toEqual([
      ['before', 'text', ''],
      ['Install', 'group', ''],
      ['a', 'text', '0'],
      ['Nested', 'group', '0'],
      ['b', 'text', '0/1'],
      ['c', 'text', '0'],
      ['after', 'text', ''],
    ])
    expect(lines[1]!.groupId).toBe(0)
    expect(lines[3]!.groupId).toBe(1)
  })

  it('tolerates a stray endgroup and an unclosed group', () => {
    const lines = parseLog('::endgroup::\n::group::open\nx')
    expect(lines.map((l) => l.kind)).toEqual(['group', 'text'])
    expect(lines[1]!.path).toEqual([0])
  })
})

describe('annotations', () => {
  it('marks levels and strips the command', () => {
    const lines = parseLog('::error::it broke\n::warning::careful\n::notice::fyi\n::debug::detail')
    expect(lines.map((l) => [l.level, lineText(l)])).toEqual([
      ['error', 'it broke'],
      ['warning', 'careful'],
      ['notice', 'fyi'],
      ['debug', 'detail'],
    ])
  })

  it('keeps the file, line and column apart from the message', () => {
    const [line] = parseLog('::error file=main.go,line=12,col=5::bad thing')
    expect(line!.level).toBe('error')
    expect(lineText(line!)).toBe('bad thing')
    expect(line!.location).toEqual({ file: 'main.go', line: '12', col: '5' })
    expect(formatLocation(line!.location)).toBe('main.go:12:5')
  })

  it('formats partial locations', () => {
    expect(formatLocation({ file: 'a.go', line: '3', col: '' })).toBe('a.go:3')
    expect(formatLocation({ file: 'a.go', line: '', col: '' })).toBe('a.go')
    expect(formatLocation({ file: '', line: '3', col: '1' })).toBe('')
    expect(parseLog('::error::plain')[0]!.location).toEqual({ file: '', line: '', col: '' })
  })

  it('leaves unknown commands alone', () => {
    const [line] = parseLog('::set-output name=x::y')
    expect(line!.level).toBe('')
    expect(lineText(line!)).toBe('::set-output name=x::y')
  })

  it('turns a step separator into a top-level group', () => {
    const lines = parseLog('==> step: build <==\nout\n==> step: test <==\n::group::inner\nx\n::endgroup::\ny')
    expect(lines.map((l) => [l.kind, l.step, l.path])).toEqual([
      ['group', true, []],
      ['text', false, [0]],
      ['group', true, []],
      ['group', false, [1]],
      ['text', false, [1, 2]],
      ['text', false, [1]],
    ])
    expect(lineText(lines[0]!)).toBe('build')
    expect(lines[0]!.no).toBe(1)
    expect(lines[2]!.no).toBe(3)
  })
})

describe('links', () => {
  it('linkifies http and https URLs and trims trailing punctuation', () => {
    expect(linkify('see https://example.com/a?b=1, or (http://x.org/p).')).toEqual([
      { text: 'see ', href: '' },
      { text: 'https://example.com/a?b=1', href: 'https://example.com/a?b=1' },
      { text: ', or (', href: '' },
      { text: 'http://x.org/p', href: 'http://x.org/p' },
      { text: ').', href: '' },
    ])
  })

  it('keeps balanced parentheses inside the URL', () => {
    expect(linkify('https://en.wikipedia.org/wiki/Foo_(bar)')[0]!.href).toBe('https://en.wikipedia.org/wiki/Foo_(bar)')
  })

  it('never links javascript:, data: or other schemes', () => {
    for (const evil of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,<script>alert(1)</script>', 'vbscript:x', 'ftp://example.com/x']) {
      expect(linkify(evil).every((p) => p.href === '')).toBe(true)
    }
    expect(linkify('[click](javascript:alert(1))').every((p) => p.href === '')).toBe(true)
  })

  it('does not link a scheme smuggled after a URL-looking prefix', () => {
    const parts = linkify('http://')
    expect(parts).toEqual([{ text: 'http://', href: '' }])
  })

  it('keeps HTML as inert text', () => {
    const lines = parseLog('<img src=x onerror=alert(1)> <script>alert(2)</script> https://ok.example/<b>')
    const line = lines[0]!
    expect(lineText(line)).toBe('<img src=x onerror=alert(1)> <script>alert(2)</script> https://ok.example/<b>')
    const links = line.segments.filter((s) => s.href)
    expect(links.map((s) => s.href)).toEqual(['https://ok.example/'])
    // Every segment is a plain string; the viewer interpolates it as text, never as markup.
    expect(line.segments.every((s) => typeof s.text === 'string')).toBe(true)
  })

  it('does not let escape sequences hide the link scheme', () => {
    const [line] = parseLog(`${ESC}[31mjava${ESC}[0mscript:alert(1)`)
    expect(line!.segments.every((s) => s.href === '')).toBe(true)
  })
})

describe('incremental parsing', () => {
  it('buffers partial lines across chunks', () => {
    const p = new LogParser()
    expect(p.push('hel')).toBe(0)
    expect(p.push('lo\nwor')).toBe(1)
    expect(p.push('ld')).toBe(0)
    expect(p.flush()).toBe(1)
    expect(p.lines.map(lineText)).toEqual(['hello', 'world'])
    expect(p.flush()).toBe(0)
  })

  it('splits an escape sequence-free chunking identically to a single push', () => {
    const text = `${ESC}[31mred\n::group::g\nx\n::endgroup::\n`
    const p = new LogParser()
    for (const ch of text) p.push(ch)
    p.flush()
    expect(p.lines).toEqual(parseLog(text))
  })

  it('numbers lines from the raw log', () => {
    const p = new LogParser()
    p.push('a\n::group::g\nb\n::endgroup::\nc\n')
    expect(p.lines.map((l) => l.no)).toEqual([1, 2, 3, 5])
    expect([...p.groups.keys()]).toEqual([0])
  })

  it('joins a very long line that arrives in many chunks', () => {
    const p = new LogParser()
    const piece = 'x'.repeat(1000)
    for (let i = 0; i < 500; i++) p.push(piece)
    expect(p.lines).toHaveLength(0)
    p.push('\ntail\n')
    expect(p.lines.map((l) => lineText(l).length)).toEqual([500_000, 4])
  })
})

describe('counts kept while parsing', () => {
  it('counts errors, warnings and the lines of every group, nested ones included', () => {
    const p = new LogParser()
    p.push('::error::a\n::warning::w\n::group::outer\nx\n::group::inner\n::error::b\ny\n::endgroup::\n::endgroup::\nz\n')
    expect([p.errors, p.warnings]).toEqual([2, 1])
    expect(p.groupLines(0)).toBe(4)
    expect(p.groupLines(1)).toBe(2)
  })

  it('opens the groups holding an error annotation, nested chain included', () => {
    const p = new LogParser()
    p.push('::group::outer\n::group::inner\n::error::deep\n::endgroup::\n::endgroup::\n::group::other\nfine\n')
    expect([0, 1, 2].map((id) => p.openByDefault(id))).toEqual([true, true, false])
  })

  it('opens groups with a FAIL line', () => {
    const p = new LogParser()
    p.push('::group::tests\n--- FAIL: TestX\n::endgroup::\n::group::other\nfine\n')
    expect([0, 1].map((id) => p.openByDefault(id))).toEqual([true, false])
  })

  it('opens the first group when the log has no problem, and stops once one shows up later', () => {
    const p = new LogParser()
    p.push('::group::one\na\n::endgroup::\n::group::two\nb\n::endgroup::\n')
    expect([0, 1].map((id) => p.openByDefault(id))).toEqual([true, false])
    p.push('::group::three\n::error::late\n')
    expect([0, 1, 2].map((id) => p.openByDefault(id))).toEqual([false, false, true])
  })

  it('opens nothing extra when the error sits outside any group, and handles logs without groups', () => {
    const p = new LogParser()
    p.push('::error::top level\n::group::g\nx\n::endgroup::\nplain\n')
    expect(p.openByDefault(0)).toBe(false)
    const q = new LogParser()
    q.push('plain\n')
    expect(q.groups.size).toBe(0)
  })
})

describe('line cap', () => {
  const log = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join('\n') + '\n'

  it('keeps the last lines, their raw numbers and a count of the dropped ones', () => {
    const p = new LogParser(100)
    p.push(log(250))
    expect(p.lines).toHaveLength(100)
    expect(p.dropped).toBe(150)
    expect(p.lines[0]!.no).toBe(151)
    expect(p.lines[99]!.no).toBe(250)
    expect(p.lines[1]!.id - p.lines[0]!.id).toBe(1)
  })

  it('keeps counts in step with the retained lines', () => {
    const p = new LogParser(3)
    p.push('::error::gone\n::group::g\na\nb\n')
    expect(p.errors).toBe(0)
    p.push('::warning::kept\n::error::kept\n')
    expect([p.errors, p.warnings]).toEqual([1, 1])
  })

  it('keeps a group open and countless once its header was dropped, so its lines never vanish behind a fold', () => {
    const p = new LogParser(3)
    p.push('::group::g\na\nb\nc\nd\n')
    expect(p.groups.size).toBe(0)
    expect(p.openByDefault(0)).toBe(true)
    expect(p.groupLines(0)).toBe(0)
    p.push('e\n')
    expect(p.groupLines(0)).toBe(0)
  })
})
