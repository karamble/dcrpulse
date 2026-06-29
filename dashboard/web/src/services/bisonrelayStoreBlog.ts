// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Auto blog index for hosted Pages. Articles live under `articles/` as plain
// markdown; this builds `index.md` as a feed-style landing that lists each
// article as a card (title, first embedded image, intro before --endofpost--),
// in a --grid-- (3-column) Featured section and a --grid2-- (one-per-row) Latest
// section. Pages carry no metadata, so title/image/intro are derived from the
// article markdown - the same idea the posts feed uses.

import {
  getBisonrelayLocalPage,
  listBisonrelayLocalPages,
  saveBisonrelayLocalPage,
} from './bisonrelayApi';

const BLOG_MARKER = '<!-- dcrpulse-blog';
const INDEX = 'index.md';

interface BlogArticle {
  name: string; // e.g. articles/ledger.md
  title: string;
  image: string; // localfilename of the first embedded image, or ''
  intro: string;
}

const altText = (s: string): string => encodeURIComponent(s);

const mimeOf = (f: string): string => {
  const e = f.toLowerCase();
  if (e.endsWith('.png')) return 'image/png';
  if (e.endsWith('.webp')) return 'image/webp';
  if (e.endsWith('.gif')) return 'image/gif';
  return 'image/jpeg';
};

const firstHeading = (md: string): string => {
  const m = md.match(/^#\s+(.+)$/m);
  return m ? m[1].trim() : '';
};

const firstEmbedImage = (md: string): string => {
  const re = /--embed\[[^\]]*localfilename=([^,\]]+)[^\]]*\]--/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(md))) {
    const f = m[1].trim();
    if (/\.(jpe?g|png|webp|gif)$/i.test(f)) return f;
  }
  return '';
};

// introText is the full article body before the --endofpost-- cutoff (the
// intro), with the leading title heading and any embed/marker lines removed. It
// is NOT truncated: the overview shows everything up to the cutoff.
const introText = (md: string): string => {
  const idx = md.search(/(^|\n)--endofpost--/);
  const before = idx >= 0 ? md.slice(0, idx) : md;
  const kept = before.split('\n').filter((l) => {
    const t = l.trim();
    if (t.startsWith('#')) return false; // title / headings
    if (/^--[a-z/].*--$/.test(t)) return false; // --embed--/--section--/etc on their own line
    return true;
  });
  return kept.join('\n').replace(/\n{3,}/g, '\n\n').trim();
};

// stripPageMarkers removes inline BR page block markers (embeds, forms, grid and
// section markers) from article-derived text, so a crafted article title/intro
// cannot inject an interactive form or break the generated index's layout. Line
// breaks are preserved (markers never span a line).
const stripPageMarkers = (s: string): string =>
  s
    .replace(/--embed\[[^\]]*\]--/gi, ' ')
    .replace(/--\/?(?:form|grid2|grid|section|endofpost)[^\n]*?--/gi, ' ');

// sanitizeTitle additionally drops markdown link/breakout characters, since the
// title is placed inside a `[title](link)` in the generated card.
const sanitizeTitle = (s: string): string =>
  stripPageMarkers(s)
    .replace(/[[\]()]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

const card = (a: BlogArticle): string => {
  const head = a.image
    ? `--embed[alt=${altText(a.title)},type=${mimeOf(a.image)},localfilename=${a.image}]--\n`
    : '';
  return `${head}**[${a.title}](${a.name})**\n\n${a.intro}\n`;
};

const buildIndex = (arts: BlogArticle[]): string => {
  const cards = arts.map(card).join('\n');
  return (
    `${BLOG_MARKER} generated from articles/ -->\n# The Sovereign Press\n\n` +
    `_A demonstration blog for the dcrpulse pages feature._\n\n` +
    `## Featured\n\n--grid--\n${cards}--/grid--\n\n` +
    `## Latest\n\n--grid2--\n${cards}--/grid2--\n`
  );
};

// isBlogManaged reports whether index.md is a generated blog landing (so saving
// an article should refresh it).
export const isBlogManaged = async (): Promise<boolean> => {
  try {
    return (await getBisonrelayLocalPage(INDEX)).includes(BLOG_MARKER);
  } catch {
    return false;
  }
};

const listBlogArticles = async (): Promise<BlogArticle[]> => {
  const pages = await listBisonrelayLocalPages();
  const names = pages
    .map((p) => p.name)
    .filter((n) => /^articles\/.+\.md$/.test(n))
    .sort();
  const arts: BlogArticle[] = [];
  for (const name of names) {
    const md = await getBisonrelayLocalPage(name);
    arts.push({
      name,
      title: sanitizeTitle(firstHeading(md) || name),
      image: firstEmbedImage(md),
      intro: stripPageMarkers(introText(md)).trim(),
    });
  }
  return arts;
};

// rebuildBlogIndex regenerates index.md from the articles/ pages. Returns the
// number of articles listed.
export const rebuildBlogIndex = async (): Promise<number> => {
  const arts = await listBlogArticles();
  await saveBisonrelayLocalPage(INDEX, buildIndex(arts));
  return arts.length;
};

// isArticlePath reports whether a saved page lives under articles/ (so a managed
// blog index should be refreshed after it is saved).
export const isArticlePath = (name: string): boolean => /^articles\/.+\.md$/.test(name);
