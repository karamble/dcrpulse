// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode } from 'react';

// Trailing punctuation that is unlikely to be part of a bare url.
const TRAILING_PUNCT = /[).,!?;:'"\]}]+$/;

const ANCHOR_CLASS =
  'text-primary underline underline-offset-2 hover:no-underline break-words';

const anchor = (key: string, href: string, label: ReactNode): ReactNode => (
  <a key={key} href={href} target="_blank" rel="noopener noreferrer" className={ANCHOR_CLASS}>
    {label}
  </a>
);

// One regex matching, in priority order at each position: inline code, a
// markdown https link [label](https://...), a bare https url, bold (**),
// strikethrough (~~), italic (*). Only https links are turned into anchors, so
// http/other schemes stay literal. Every pattern is linear (no nested
// quantifiers), so there is no catastrophic-backtracking risk; the message
// length bounds the work.
const TOKEN_RE =
  /(`[^`\n]+`)|(\[[^\]\n]+\]\(https:\/\/[^\s)]+\))|(https:\/\/[^\s]+)|(\*\*[\s\S]+?\*\*)|(~~[\s\S]+?~~)|(\*[^*\n]+?\*)/g;

// renderChatInline turns a chat text segment into React nodes, rendering inline
// markdown (bold/italic/inline-code/strikethrough) and https links. Everything
// is built as React elements with escaped text - never raw HTML - so there is
// no injection surface. Unmatched markers render literally; whitespace and
// newlines are preserved by the surrounding pre-wrap container.
const renderChatInline = (text: string, kp = 'i'): ReactNode[] => {
  const nodes: ReactNode[] = [];
  let last = 0;
  let n = 0;
  const re = new RegExp(TOKEN_RE);
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) nodes.push(text.slice(last, m.index));
    const key = `${kp}-${n++}`;
    const [full, code, mdLink, bareUrl, bold, strike, italic] = m;
    if (code) {
      nodes.push(
        <code key={key} className="px-1 py-0.5 rounded bg-muted/40 font-mono text-[0.85em]">
          {code.slice(1, -1)}
        </code>,
      );
    } else if (mdLink) {
      const close = mdLink.indexOf(']');
      const label = mdLink.slice(1, close);
      const url = mdLink.slice(mdLink.indexOf('](', close) + 2, -1);
      nodes.push(anchor(key, url, label));
    } else if (bareUrl) {
      const trailing = bareUrl.match(TRAILING_PUNCT)?.[0] ?? '';
      const url = trailing ? bareUrl.slice(0, -trailing.length) : bareUrl;
      nodes.push(anchor(key, url, url));
      if (trailing) nodes.push(trailing);
    } else if (bold) {
      nodes.push(<strong key={key}>{renderChatInline(bold.slice(2, -2), key)}</strong>);
    } else if (strike) {
      nodes.push(<del key={key}>{renderChatInline(strike.slice(2, -2), key)}</del>);
    } else if (italic) {
      nodes.push(<em key={key}>{renderChatInline(italic.slice(1, -1), key)}</em>);
    }
    last = m.index + full.length;
  }
  if (last < text.length) nodes.push(text.slice(last));
  return nodes;
};

// linkifyChatText keeps its name (MessageBody calls it per text segment); it now
// renders inline markdown in addition to https links.
export const linkifyChatText = (text: string): ReactNode[] => renderChatInline(text);
