// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Fragment, ReactNode, useMemo } from 'react';
import { linkifyChatText } from './chatLinkify';

// Block-level markdown for chat and feed bodies. bruig parses the same wire
// bytes with the GitHub-flavored block syntaxes, so a table a peer sends from
// bruig has to render here too. Inline spans stay with linkifyChatText.

export type Align = 'left' | 'center' | 'right';

export interface ListItem {
  text: string;
  children: Block[];
}

export type Block =
  | { kind: 'para'; text: string }
  | { kind: 'code'; lang: string; text: string }
  | { kind: 'heading'; level: number; text: string }
  | { kind: 'rule' }
  | { kind: 'table'; head: string[]; align: Align[]; rows: string[][] }
  | { kind: 'quote'; blocks: Block[] }
  | { kind: 'list'; ordered: boolean; items: ListItem[] };

const FENCE_RE = /^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)\s*$/;
const ATX_RE = /^\s{0,3}(#{1,6})\s+(.*)$/;
const RULE_RE = /^\s{0,3}(?:(?:\*\s*){3,}|(?:-\s*){3,}|(?:_\s*){3,})$/;
// Three or more, unlike CommonMark: a lone "-" in chat is a stray bullet, not
// a heading underline.
const SETEXT_RE = /^\s{0,3}(={3,}|-{3,})\s*$/;
const QUOTE_RE = /^\s{0,3}>\s?(.*)$/;
const UL_RE = /^(\s*)[-*+]\s+(.*)$/;
const OL_RE = /^(\s*)\d{1,9}[.)]\s+(.*)$/;
const DELIM_CELL_RE = /^:?-+:?$/;

// stripClosingHashes drops an ATX heading's closing "#" run and the whitespace
// around it, which must be preceded by whitespace ("## foo#" keeps its "#").
// A backward scan, where the equivalent regex is quadratic on long whitespace.
const stripClosingHashes = (s: string): string => {
  const ws = (c: string) => /\s/.test(c);
  let end = s.length;
  while (end > 0 && ws(s[end - 1])) end--;
  let h = end;
  while (h > 0 && s[h - 1] === '#') h--;
  if (h === end) return s;
  let w = h;
  while (w > 0 && ws(s[w - 1])) w--;
  return w < h ? s.slice(0, w) : s;
};

// Nested quotes and lists recurse; cap the depth so a pathological run of ">"
// cannot exhaust the stack.
const MAX_DEPTH = 6;

// splitRow splits one table line into trimmed cells, honouring "\|" escapes and
// dropping the optional leading and trailing pipe.
const splitRow = (line: string): string[] => {
  let s = line.trim();
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|') && !s.endsWith('\\|')) s = s.slice(0, -1);
  const cells: string[] = [];
  let cur = '';
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && s[i + 1] === '|') {
      cur += '|';
      i++;
    } else if (s[i] === '|') {
      cells.push(cur.trim());
      cur = '';
    } else {
      cur += s[i];
    }
  }
  cells.push(cur.trim());
  return cells;
};

const delimAlign = (cells: string[]): Align[] | null => {
  const out: Align[] = [];
  for (const c of cells) {
    if (!DELIM_CELL_RE.test(c)) return null;
    const left = c.startsWith(':');
    const right = c.endsWith(':');
    out.push(left && right ? 'center' : right ? 'right' : 'left');
  }
  return out;
};

const parseList = (
  lines: string[],
  start: number,
  depth: number,
): { block: Block; next: number } => {
  const ordered = OL_RE.test(lines[start]);
  const first = lines[start].match(ordered ? OL_RE : UL_RE)!;
  const baseIndent = first[1].length;
  const items: ListItem[] = [];
  let i = start;
  while (i < lines.length) {
    if (lines[i].trim() === '') {
      const nxt = lines[i + 1];
      const cont = nxt !== undefined && (nxt.match(UL_RE) ?? nxt.match(OL_RE));
      if (!cont || cont[1].length < baseIndent) break;
      i++;
      continue;
    }
    const m = lines[i].match(UL_RE) ?? lines[i].match(OL_RE);
    if (!m) break;
    const indent = m[1].length;
    if (indent < baseIndent) break;
    if (indent > baseIndent) {
      if (depth >= MAX_DEPTH) break;
      const sub = parseList(lines, i, depth + 1);
      if (items.length === 0) items.push({ text: '', children: [] });
      items[items.length - 1].children.push(sub.block);
      i = sub.next;
      continue;
    }
    if (OL_RE.test(lines[i]) !== ordered) break;
    items.push({ text: m[2], children: [] });
    i++;
  }
  return { block: { kind: 'list', ordered, items }, next: i };
};

// parseBlocks groups lines into blocks. Precedence is the whole correctness
// story: fenced code first so pipes inside code survive, then tables, which
// need a delimiter row whose cell count matches the header.
export const parseBlocks = (src: string, depth = 0): Block[] => {
  const lines = src.split('\n');
  const blocks: Block[] = [];
  let para: string[] = [];

  const flushPara = () => {
    if (para.length > 0) {
      blocks.push({ kind: 'para', text: para.join('\n') });
      para = [];
    }
  };

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    const fence = line.match(FENCE_RE);
    if (fence) {
      flushPara();
      const marker = fence[1][0];
      const width = fence[1].length;
      const body: string[] = [];
      i++;
      for (; i < lines.length; i++) {
        const close = lines[i].match(FENCE_RE);
        if (close && close[1][0] === marker && close[1].length >= width && close[2] === '') break;
        body.push(lines[i]);
      }
      if (i < lines.length) i++;
      blocks.push({ kind: 'code', lang: fence[2], text: body.join('\n') });
      continue;
    }

    if (line.trim() === '') {
      flushPara();
      i++;
      continue;
    }

    if (line.includes('|') && i + 1 < lines.length && lines[i + 1].includes('|')) {
      const head = splitRow(line);
      const align = delimAlign(splitRow(lines[i + 1]));
      if (align && align.length === head.length) {
        flushPara();
        const rows: string[][] = [];
        i += 2;
        for (; i < lines.length; i++) {
          if (lines[i].trim() === '' || !lines[i].includes('|')) break;
          const cells = splitRow(lines[i]);
          while (cells.length < head.length) cells.push('');
          rows.push(cells.slice(0, head.length));
        }
        blocks.push({ kind: 'table', head, align, rows });
        continue;
      }
    }

    const atx = line.match(ATX_RE);
    if (atx) {
      flushPara();
      blocks.push({
        kind: 'heading',
        level: atx[1].length,
        text: stripClosingHashes(atx[2]),
      });
      i++;
      continue;
    }

    // A rule directly under a paragraph is a setext underline, not a rule.
    const setext = line.match(SETEXT_RE);
    if (setext && para.length > 0) {
      blocks.push({
        kind: 'heading',
        level: setext[1][0] === '=' ? 1 : 2,
        text: para.join('\n'),
      });
      para = [];
      i++;
      continue;
    }

    if (RULE_RE.test(line)) {
      flushPara();
      blocks.push({ kind: 'rule' });
      i++;
      continue;
    }

    if (QUOTE_RE.test(line) && depth < MAX_DEPTH) {
      flushPara();
      const inner: string[] = [];
      for (; i < lines.length; i++) {
        const m = lines[i].match(QUOTE_RE);
        if (!m) break;
        inner.push(m[1]);
      }
      blocks.push({ kind: 'quote', blocks: parseBlocks(inner.join('\n'), depth + 1) });
      continue;
    }

    if ((UL_RE.test(line) || OL_RE.test(line)) && depth < MAX_DEPTH) {
      flushPara();
      const { block, next } = parseList(lines, i, depth);
      blocks.push(block);
      i = next;
      continue;
    }

    para.push(line);
    i++;
  }
  flushPara();
  return blocks;
};

const ALIGN_CLASS: Record<Align, string> = {
  left: 'text-left',
  center: 'text-center',
  right: 'text-right',
};

// Message headings start at h3 so a chat body cannot outrank the page outline.
const headingTag = (level: number) => `h${Math.min(6, level + 2)}` as 'h3';

const HEADING_CLASS: Record<number, string> = {
  1: 'text-[1.15em] font-semibold mt-1',
  2: 'text-[1.08em] font-semibold mt-1',
  3: 'text-[1em] font-semibold mt-1',
  4: 'text-[1em] font-semibold',
  5: 'text-[1em] font-semibold',
  6: 'text-[1em] font-semibold',
};

const renderBlock = (b: Block, key: string): ReactNode => {
  switch (b.kind) {
    case 'para':
      return (
        <p key={key} className="whitespace-pre-wrap break-words">
          {linkifyChatText(b.text)}
        </p>
      );
    case 'code':
      return (
        <pre key={key} className="overflow-x-auto rounded bg-muted/40 px-2 py-1.5">
          <code className="font-mono text-[0.85em] whitespace-pre">{b.text}</code>
        </pre>
      );
    case 'heading': {
      const Tag = headingTag(b.level);
      return (
        <Tag key={key} className={`break-words ${HEADING_CLASS[b.level]}`}>
          {linkifyChatText(b.text)}
        </Tag>
      );
    }
    case 'rule':
      return <hr key={key} className="border-border" />;
    case 'table':
      return (
        <div key={key} className="overflow-x-auto">
          <table className="border-collapse text-[0.92em]">
            <thead>
              <tr className="border-b border-border">
                {b.head.map((c, j) => (
                  <th
                    key={j}
                    className={`px-2 py-1 font-semibold whitespace-nowrap ${ALIGN_CLASS[b.align[j]]}`}
                  >
                    {linkifyChatText(c)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {b.rows.map((r, ri) => (
                <tr key={ri} className="border-b border-border/40 last:border-0">
                  {r.map((c, ci) => (
                    <td key={ci} className={`px-2 py-1 align-top ${ALIGN_CLASS[b.align[ci]]}`}>
                      {linkifyChatText(c)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case 'quote':
      return (
        <blockquote
          key={key}
          className="border-l-2 border-border pl-2 text-muted-foreground space-y-1"
        >
          {renderBlocks(b.blocks, key)}
        </blockquote>
      );
    case 'list': {
      const Tag = b.ordered ? 'ol' : 'ul';
      return (
        <Tag
          key={key}
          className={`pl-5 space-y-0.5 ${b.ordered ? 'list-decimal' : 'list-disc'}`}
        >
          {b.items.map((it, j) => (
            <li key={j} className="break-words">
              {linkifyChatText(it.text)}
              {it.children.length > 0 && (
                <div className="space-y-0.5">{renderBlocks(it.children, `${key}-${j}`)}</div>
              )}
            </li>
          ))}
        </Tag>
      );
    }
  }
};

const renderBlocks = (blocks: Block[], kp: string): ReactNode[] =>
  blocks.map((b, i) => <Fragment key={`${kp}-${i}`}>{renderBlock(b, `${kp}-${i}`)}</Fragment>);

// ChatMarkdown renders one text segment. Everything is built as React elements
// with escaped text, never raw HTML, so there is no injection surface.
export const ChatMarkdown = ({ text, className }: { text: string; className?: string }) => {
  const blocks = useMemo(() => parseBlocks(text), [text]);
  return <div className={className ? `space-y-1 ${className}` : 'space-y-1'}>{renderBlocks(blocks, 'b')}</div>;
};
