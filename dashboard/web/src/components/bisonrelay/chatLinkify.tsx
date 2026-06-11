// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode } from 'react';

// Matches a markdown https link [label](https://...) or a bare https url.
// Only https is matched, so http and other schemes are never turned into
// links. Clicks on the rendered anchors are caught by the app-wide
// ExternalLinkGuard, which routes them through the leaving-site warning modal.
const LINK_RE = /\[([^\]]+)\]\((https:\/\/[^\s)]+)\)|(https:\/\/[^\s]+)/g;

// Trailing punctuation that is unlikely to be part of a bare url.
const TRAILING_PUNCT = /[).,!?;:'"\]}]+$/;

const ANCHOR_CLASS =
  'text-primary underline underline-offset-2 hover:no-underline break-words';

const anchor = (key: number, href: string, label: string): ReactNode => (
  <a key={key} href={href} target="_blank" rel="noopener noreferrer" className={ANCHOR_CLASS}>
    {label}
  </a>
);

// linkifyChatText turns https urls (bare or markdown-formatted) in a chat text
// segment into clickable anchors, leaving all other text verbatim so that
// whitespace and newlines are preserved by the surrounding pre-wrap container.
export const linkifyChatText = (text: string): ReactNode[] => {
  const nodes: ReactNode[] = [];
  let lastIndex = 0;
  let key = 0;
  LINK_RE.lastIndex = 0;

  let m: RegExpExecArray | null;
  while ((m = LINK_RE.exec(text)) !== null) {
    if (m.index > lastIndex) {
      nodes.push(text.slice(lastIndex, m.index));
    }
    const [full, mdLabel, mdUrl, bareUrl] = m;
    if (mdUrl) {
      nodes.push(anchor(key++, mdUrl, mdLabel));
    } else if (bareUrl) {
      const trailing = bareUrl.match(TRAILING_PUNCT)?.[0] ?? '';
      const url = trailing ? bareUrl.slice(0, bareUrl.length - trailing.length) : bareUrl;
      nodes.push(anchor(key++, url, url));
      if (trailing) nodes.push(trailing);
    }
    lastIndex = m.index + full.length;
  }
  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
};
