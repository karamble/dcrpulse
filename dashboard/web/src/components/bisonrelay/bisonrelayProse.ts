// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// BR_PROSE_CLASSES is the Tailwind utility chain used to render BR post
// bodies (and any other surface that goes through services.RenderMarkdownHTML
// → bluemonday). It targets every element goldmark emits + bluemonday
// allows in services/markdown.go's policy:
//
//   h1..h6, p, br, hr, strong, em, del, code, pre, blockquote,
//   ul, ol, li, table, thead, tbody, tr, th, td, a
//
// Kept as a flat utility chain (no @tailwindcss/typography plugin) so the
// styling lives in source and stays identical between the live Feed
// detail view and the editor's Preview tab.

export const BR_PROSE_CLASSES = [
  // Base text
  'text-sm text-foreground/90 break-words',

  // Paragraphs
  '[&_p]:my-3 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0 [&_p]:leading-relaxed',

  // Headings — proportional sizes with subtle underlines on h1/h2 so
  // section breaks read at a glance in long-form posts.
  '[&_h1]:text-2xl [&_h1]:font-bold [&_h1]:mt-6 [&_h1]:mb-3 [&_h1]:pb-2 [&_h1]:border-b [&_h1]:border-border/40 [&_h1]:text-foreground',
  '[&_h2]:text-xl [&_h2]:font-bold [&_h2]:mt-5 [&_h2]:mb-2 [&_h2]:pb-1 [&_h2]:border-b [&_h2]:border-border/30 [&_h2]:text-foreground',
  '[&_h3]:text-lg [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2 [&_h3]:text-foreground',
  '[&_h4]:text-base [&_h4]:font-semibold [&_h4]:mt-3 [&_h4]:mb-1.5 [&_h4]:text-foreground',
  '[&_h5]:text-sm [&_h5]:font-semibold [&_h5]:mt-3 [&_h5]:mb-1 [&_h5]:text-foreground',
  '[&_h6]:text-xs [&_h6]:font-semibold [&_h6]:mt-3 [&_h6]:mb-1 [&_h6]:uppercase [&_h6]:tracking-wide [&_h6]:text-muted-foreground',

  // Inline emphasis
  '[&_strong]:font-semibold [&_strong]:text-foreground',
  '[&_em]:italic',
  '[&_del]:line-through [&_del]:text-muted-foreground',

  // Links — Politeia + BR posts share the same link policy
  // (target=_blank + nofollow added by bluemonday for fully-qualified URLs).
  '[&_a]:text-primary [&_a]:underline [&_a]:underline-offset-2 [&_a:hover]:no-underline [&_a]:break-words',

  // Lists. nested lists tighten spacing so they don't double-pad.
  '[&_ul]:list-disc [&_ul]:pl-6 [&_ul]:my-3 [&_ul>li]:my-1',
  '[&_ol]:list-decimal [&_ol]:pl-6 [&_ol]:my-3 [&_ol>li]:my-1',
  '[&_li]:leading-relaxed [&_li>p]:my-1',
  '[&_ul_ul]:my-1 [&_ul_ol]:my-1 [&_ol_ul]:my-1 [&_ol_ol]:my-1',

  // Inline code vs code block. goldmark wraps fenced blocks as <pre><code>.
  '[&_code]:font-mono [&_code]:text-[0.85em] [&_code]:bg-muted/30 [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded',
  '[&_pre]:bg-muted/30 [&_pre]:border [&_pre]:border-border/40 [&_pre]:rounded-md [&_pre]:p-3 [&_pre]:my-3 [&_pre]:overflow-x-auto',
  '[&_pre>code]:bg-transparent [&_pre>code]:p-0 [&_pre>code]:text-xs [&_pre>code]:leading-relaxed [&_pre>code]:rounded-none',

  // Blockquotes — left accent bar + soft tint
  '[&_blockquote]:border-l-4 [&_blockquote]:border-primary/40 [&_blockquote]:bg-muted/10 [&_blockquote]:pl-3 [&_blockquote]:pr-2 [&_blockquote]:py-1 [&_blockquote]:my-3 [&_blockquote]:text-muted-foreground [&_blockquote]:italic',
  '[&_blockquote>p]:my-1',

  // Horizontal rule
  '[&_hr]:my-4 [&_hr]:border-0 [&_hr]:border-t [&_hr]:border-border/40',

  // GFM tables — goldmark+GFM emits <thead>/<tbody>; we border the cells
  // and tint headers so even a wide table stays readable.
  '[&_table]:my-3 [&_table]:w-full [&_table]:border-collapse [&_table]:text-xs',
  '[&_thead]:border-b [&_thead]:border-border/60',
  '[&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_th]:bg-muted/20 [&_th]:text-foreground',
  '[&_td]:px-3 [&_td]:py-2 [&_td]:border-t [&_td]:border-border/30 [&_td]:align-top',
  '[&_tbody_tr:hover_td]:bg-muted/10',
].join(' ');
