// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { parseEmbeds } from './embedParser';
import { EmbedRenderer, ImageViewerOpenFn } from './embedRender';
import { linkifyChatText } from './chatLinkify';

// The in-band reply convention Bison Relay clients exchange: a leading run
// of "> " lines, the first carrying "**nick:** ", then a blank line and the
// typed reply. bruig renders it as a markdown blockquote; dcrpulse renders
// the same wire bytes as a styled inset panel.

export interface LeadingQuote {
  nick: string;
  // Quoted content with the "> " prefixes stripped; inline embed tags stay
  // intact so quoted image thumbnails render inside the panel.
  text: string;
  // The reply body following the quote block.
  rest: string;
}

const QUOTE_NICK_RE = /^\*\*(.+?):\*\* ?/;

// splitLeadingQuote detects a quote block at the very start of a message.
// It works on raw lines before embed parsing because a quoted image tag
// lives inside a "> " line. Messages not starting with a quote line return
// null and render exactly as before (mid-text ">" is never touched).
export function splitLeadingQuote(body: string): LeadingQuote | null {
  const lines = body.split('\n');
  if (lines.length === 0 || !(lines[0].startsWith('> ') || lines[0] === '>')) {
    return null;
  }
  let i = 0;
  const quoted: string[] = [];
  for (; i < lines.length; i++) {
    const l = lines[i];
    if (l.startsWith('> ')) {
      quoted.push(l.slice(2));
    } else if (l === '>') {
      quoted.push('');
    } else {
      break;
    }
  }
  // One blank separator line belongs to the convention, not the reply.
  if (i < lines.length && lines[i].trim() === '') i++;
  let nick = '';
  if (quoted.length > 0) {
    const m = quoted[0].match(QUOTE_NICK_RE);
    if (m) {
      nick = m[1];
      quoted[0] = quoted[0].slice(m[0].length);
    }
  }
  return { nick, text: quoted.join('\n').trim(), rest: lines.slice(i).join('\n') };
}

// QuoteBlock is the inset panel inside a chat bubble: theme-accent left bar,
// tinted background, the quoted nick, and the quoted content clamped short -
// the original stays one scroll away in the thread.
export const QuoteBlock = ({
  nick,
  text,
  openViewer,
}: {
  nick: string;
  text: string;
  openViewer?: ImageViewerOpenFn;
}) => (
  <div className="border-l-2 border-primary/70 bg-primary/10 rounded-md px-2 py-1 mb-1">
    {nick && <p className="text-xs font-semibold text-primary">{nick}</p>}
    {parseEmbeds(text).map((seg, i) => {
      if (seg.kind === 'text') {
        if (!seg.text.trim()) return null;
        return (
          <p
            key={i}
            className="text-[13px] text-muted-foreground whitespace-pre-wrap break-words line-clamp-3"
          >
            {linkifyChatText(seg.text)}
          </p>
        );
      }
      if (seg.kind === 'embed') {
        return (
          <div key={i} className="max-w-[12rem] py-0.5">
            <EmbedRenderer embed={seg} openViewer={openViewer} />
          </div>
        );
      }
      return (
        <p key={i} className="text-[11px] text-muted-foreground italic">
          [file {seg.filename || 'unnamed'}]
        </p>
      );
    })}
  </div>
);
