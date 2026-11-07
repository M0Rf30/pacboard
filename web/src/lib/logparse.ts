// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Turns raw CI log text into structured lines: ANSI colours, `::group::` folding, `::error::`-style
// annotations and safe links. The output carries plain strings only; the viewer renders them through
// Vue text interpolation, so nothing from the log is ever interpreted as HTML.

export interface Segment {
  text: string
  /** Space-separated CSS class names (ansi-fg-N, ansi-bg-N, ansi-bold, ...). */
  cls: string
  /** Colour for 256-colour / truecolor foregrounds, as #rrggbb. */
  color: string
  /** Same for backgrounds. */
  bg: string
  /** http(s) URL when this segment is a link, otherwise "". */
  href: string
}

export type LogLevel = '' | 'error' | 'warning' | 'notice' | 'debug'

export interface LogLine {
  id: number
  /** 1-based line number in the raw log. */
  no: number
  kind: 'text' | 'group' | 'step'
  level: LogLevel
  segments: Segment[]
  /** Group id when kind is "group", otherwise -1. */
  groupId: number
  /** Ids of the enclosing groups, outermost first. */
  path: readonly number[]
}

interface SgrState {
  fg: string
  bg: string
  bold: boolean
  dim: boolean
  italic: boolean
  underline: boolean
}

// CSI sequences (final byte captured), OSC sequences and lone two-byte escapes.
// eslint-disable-next-line no-control-regex
const ESCAPES = /\x1b\[([0-9;:?]*)([\x20-\x2f]*[\x40-\x7e])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]/g
const ANNOTATION = /^::(error|warning|notice|debug)(?: ([^:]*))?::(.*)$/
const GROUP = /^::group::(.*)$/
const ENDGROUP = /^::endgroup::\s*$/
const STEP = /^==> step: (.*) <==$/
const URL_CANDIDATE = /https?:\/\/[^\s<>"'`\u0000-\u001f]+/gi

/** Removes every ANSI escape sequence. */
export function stripAnsi(s: string): string {
  return s.replace(ESCAPES, '')
}

function hex(n: number): string {
  return n.toString(16).padStart(2, '0')
}

/** xterm 256-colour index → "ansi-N" palette token (0-15) or #rrggbb. */
function color256(n: number): string {
  if (n < 0 || n > 255 || !Number.isInteger(n)) return ''
  if (n < 16) return `p${n}`
  if (n < 232) {
    const i = n - 16
    const lv = (v: number) => (v === 0 ? 0 : 55 + v * 40)
    return `#${hex(lv(Math.floor(i / 36)))}${hex(lv(Math.floor(i / 6) % 6))}${hex(lv(i % 6))}`
  }
  const g = 8 + (n - 232) * 10
  return `#${hex(g)}${hex(g)}${hex(g)}`
}

function rgb(r: number, g: number, b: number): string {
  for (const v of [r, g, b]) if (!Number.isInteger(v) || v < 0 || v > 255) return ''
  return `#${hex(r)}${hex(g)}${hex(b)}`
}

function freshState(): SgrState {
  return { fg: '', bg: '', bold: false, dim: false, italic: false, underline: false }
}

function applySgr(st: SgrState, params: string): void {
  const p = params === '' ? [0] : params.split(/[;:]/).map((x) => (x === '' ? 0 : Number(x)))
  for (let i = 0; i < p.length; i++) {
    const c = p[i]!
    if (c === 0) Object.assign(st, freshState())
    else if (c === 1) st.bold = true
    else if (c === 2) st.dim = true
    else if (c === 3) st.italic = true
    else if (c === 4) st.underline = true
    else if (c === 22) st.bold = st.dim = false
    else if (c === 23) st.italic = false
    else if (c === 24) st.underline = false
    else if (c >= 30 && c <= 37) st.fg = `p${c - 30}`
    else if (c >= 90 && c <= 97) st.fg = `p${c - 90 + 8}`
    else if (c >= 40 && c <= 47) st.bg = `p${c - 40}`
    else if (c >= 100 && c <= 107) st.bg = `p${c - 100 + 8}`
    else if (c === 39) st.fg = ''
    else if (c === 49) st.bg = ''
    else if (c === 38 || c === 48) {
      let col = ''
      if (p[i + 1] === 5) {
        col = color256(p[i + 2] ?? -1)
        i += 2
      } else if (p[i + 1] === 2) {
        col = rgb(p[i + 2] ?? -1, p[i + 3] ?? -1, p[i + 4] ?? -1)
        i += 4
      }
      if (c === 38) st.fg = col
      else st.bg = col
    }
  }
}

function classesOf(st: SgrState): string {
  const c: string[] = []
  if (st.bold) c.push('ansi-bold')
  if (st.dim) c.push('ansi-dim')
  if (st.italic) c.push('ansi-italic')
  if (st.underline) c.push('ansi-underline')
  if (st.fg.startsWith('p')) c.push(`ansi-fg-${st.fg.slice(1)}`)
  if (st.bg.startsWith('p')) c.push(`ansi-bg-${st.bg.slice(1)}`)
  return c.join(' ')
}

function trimUrl(u: string): string {
  let s = u
  for (;;) {
    const last = s[s.length - 1]
    if (last === undefined) return s
    if ('.,;:!?*'.includes(last)) {
      s = s.slice(0, -1)
      continue
    }
    const open = { ')': '(', ']': '[', '}': '{' }[last]
    if (open && s.split(last).length > s.split(open).length) {
      s = s.slice(0, -1)
      continue
    }
    return s
  }
}

export interface TextPart {
  text: string
  href: string
}

/** Splits text into plain parts and http(s) link parts. Only parseable http/https URLs become links. */
export function linkify(text: string): TextPart[] {
  const parts: TextPart[] = []
  let last = 0
  for (const m of text.matchAll(URL_CANDIDATE)) {
    const start = m.index
    const candidate = trimUrl(m[0])
    let href = ''
    try {
      const url = new URL(candidate)
      if ((url.protocol === 'http:' || url.protocol === 'https:') && url.hostname) href = url.href
    } catch {
      // not a URL: stays text
    }
    if (!href) continue
    if (start > last) parts.push({ text: text.slice(last, start), href: '' })
    parts.push({ text: candidate, href })
    last = start + candidate.length
  }
  if (last < text.length) parts.push({ text: text.slice(last), href: '' })
  return parts
}

function annotationText(props: string | undefined, message: string): string {
  if (!props) return message
  const kv: Record<string, string> = {}
  for (const pair of props.split(',')) {
    const eq = pair.indexOf('=')
    if (eq > 0) kv[pair.slice(0, eq).trim()] = pair.slice(eq + 1).trim()
  }
  if (!kv.file) return message
  return `${kv.file}${kv.line ? `:${kv.line}` : ''}: ${message}`
}

/** Incremental parser: push() raw chunks as they stream in, then flush() at the end. */
export class LogParser {
  readonly lines: LogLine[] = []

  private buf = ''
  private state = freshState()
  private path: readonly number[] = []
  private groupCount = 0
  private rawLines = 0

  /** Feeds a chunk; returns how many new lines were completed. */
  push(chunk: string): number {
    this.buf += chunk
    const before = this.lines.length
    let from = 0
    for (let nl = this.buf.indexOf('\n', from); nl >= 0; nl = this.buf.indexOf('\n', from)) {
      this.addLine(this.buf.slice(from, nl))
      from = nl + 1
    }
    this.buf = this.buf.slice(from)
    return this.lines.length - before
  }

  /** Completes a trailing line without newline. */
  flush(): number {
    if (this.buf === '') return 0
    const before = this.lines.length
    const rest = this.buf
    this.buf = ''
    this.addLine(rest)
    return this.lines.length - before
  }

  /** Ids of every group seen so far. */
  groupIds(): number[] {
    return this.lines.filter((l) => l.kind === 'group').map((l) => l.groupId)
  }

  private addLine(rawLine: string): void {
    this.rawLines++
    let line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine
    // Progress bars rewrite the line with \r: only the last rewrite is what the terminal shows.
    const cr = line.lastIndexOf('\r')
    if (cr >= 0 && cr < line.length - 1) line = line.slice(cr + 1)

    const plain = stripAnsi(line)

    if (ENDGROUP.test(plain)) {
      if (this.path.length > 0) this.path = this.path.slice(0, -1)
      return
    }

    let kind: LogLine['kind'] = 'text'
    let level: LogLevel = ''
    let groupId = -1
    const path = this.path

    const g = GROUP.exec(plain)
    const a = g ? null : ANNOTATION.exec(plain)
    const s = g || a ? null : STEP.exec(plain)
    if (g) {
      kind = 'group'
      groupId = this.groupCount++
      this.path = [...this.path, groupId]
      line = g[1]!.trim() || 'group'
    } else if (a) {
      level = a[1] as LogLevel
      line = annotationText(a[2], a[3]!)
    } else if (s) {
      kind = 'step'
      line = s[1]!
    }

    this.lines.push({
      id: this.lines.length,
      no: this.rawLines,
      kind,
      level,
      segments: this.segmentsOf(line),
      groupId,
      path,
    })
  }

  private segmentsOf(line: string): Segment[] {
    const out: Segment[] = []
    const emit = (text: string) => {
      if (text === '') return
      const cls = classesOf(this.state)
      const color = this.state.fg.startsWith('#') ? this.state.fg : ''
      const bg = this.state.bg.startsWith('#') ? this.state.bg : ''
      for (const part of linkify(text)) out.push({ text: part.text, cls, color, bg, href: part.href })
    }
    let last = 0
    for (const m of line.matchAll(ESCAPES)) {
      emit(line.slice(last, m.index))
      last = m.index + m[0].length
      if (m[2] === 'm') applySgr(this.state, m[1] ?? '')
    }
    emit(line.slice(last))
    return out
  }
}

/** Parses a whole log at once. */
export function parseLog(text: string): LogLine[] {
  const p = new LogParser()
  p.push(text)
  p.flush()
  return p.lines
}

/** Concatenated plain text of a line. */
export function lineText(l: LogLine): string {
  return l.segments.map((s) => s.text).join('')
}

/** Lines not hidden inside a collapsed group. */
export function visibleLines(lines: readonly LogLine[], collapsed: ReadonlySet<number>): LogLine[] {
  if (collapsed.size === 0) return lines.slice()
  return lines.filter((l) => !l.path.some((id) => collapsed.has(id)))
}
