// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

import { describe, expect, it } from 'vitest'
import { LogParser, lineText, linkify, parseLog, stripAnsi, visibleLines } from './logparse'

const ESC = '\x1b'

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

  it('folds collapsed groups with their children only', () => {
    const lines = parseLog(log)
    expect(visibleLines(lines, new Set([1])).map(lineText)).toEqual(['before', 'Install', 'a', 'Nested', 'c', 'after'])
    expect(visibleLines(lines, new Set([0])).map(lineText)).toEqual(['before', 'Install', 'after'])
    expect(visibleLines(lines, new Set()).length).toBe(lines.length)
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

  it('prefixes the file and line properties', () => {
    const [line] = parseLog('::error file=main.go,line=12::bad thing')
    expect(line!.level).toBe('error')
    expect(lineText(line!)).toBe('main.go:12: bad thing')
  })

  it('leaves unknown commands alone', () => {
    const [line] = parseLog('::set-output name=x::y')
    expect(line!.level).toBe('')
    expect(lineText(line!)).toBe('::set-output name=x::y')
  })

  it('marks step separators', () => {
    const [line] = parseLog('==> step: build <==')
    expect(line!.kind).toBe('step')
    expect(lineText(line!)).toBe('build')
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

  it('numbers lines from the raw log and lists group ids', () => {
    const p = new LogParser()
    p.push('a\n::group::g\nb\n::endgroup::\nc\n')
    expect(p.lines.map((l) => l.no)).toEqual([1, 2, 3, 5])
    expect(p.groupIds()).toEqual([0])
  })
})
