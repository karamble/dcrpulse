import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { linkifyChatText } from './chatLinkify';
import { ATX_RE, FENCE_RE, OL_RE, QUOTE_RE, UL_RE, parseBlocks } from './chatMarkdown';
import { parseEmbeds, stripEmbedTags, type MessageSegment } from './embedParser';

afterEach(cleanup);

// Peer text reaches these renderers unfiltered; each must finish in time
// linear in its length. The quadratic versions took seconds to minutes here.
const BUDGET_MS = 1000;
const fast = (fn: () => unknown) => {
  const start = performance.now();
  fn();
  expect(performance.now() - start).toBeLessThan(BUDGET_MS);
};

const renderInline = (text: string) => render(<p>{linkifyChatText(text)}</p>).container;

describe('linkifyChatText', () => {
  it('renders links and inline markdown as before', () => {
    const p = renderInline('see [docs](https://a.b/x) and https://a.b/c). **bold** *it* `code`');
    const links = Array.from(p.querySelectorAll('a')).map((a) => [a.textContent, a.getAttribute('href')]);
    expect(links).toEqual([
      ['docs', 'https://a.b/x'],
      ['https://a.b/c', 'https://a.b/c'],
    ]);
    expect(p.textContent).toContain('https://a.b/c).');
    expect(p.querySelector('strong')?.textContent).toBe('bold');
    expect(p.querySelector('em')?.textContent).toBe('it');
    expect(p.querySelector('code')?.textContent).toBe('code');
  });

  it('stays fast on unclosed brackets', () => {
    fast(() => linkifyChatText('['.repeat(200_000)));
  });

  it('stays fast on a url ending in a long punctuation run', () => {
    fast(() => linkifyChatText('https://x' + ')'.repeat(200_000) + 'a'));
  });
});

describe('heading closing hashes', () => {
  const heading = (src: string) => {
    const [b] = parseBlocks(src);
    return b.kind === 'heading' ? b.text : undefined;
  };

  it('strips closing hashes as before', () => {
    expect(heading('## Title ##')).toBe('Title');
    expect(heading('## Title ##   ')).toBe('Title');
    expect(heading('## foo#')).toBe('foo#');
    expect(heading('## ###')).toBe('###');
    expect(heading('## a # b')).toBe('a # b');
  });

  it('stays fast on a long whitespace run', () => {
    fast(() => parseBlocks('# a' + ' '.repeat(100_000) + '#x'));
  });
});

// The pre-fix parser, kept to check the linear scan splits every body the same way.
const TAG_RE = /--(embed|download)\[(.*?)\]--/g;
const reference = (body: string): string[][] => {
  if (!body) return [];
  const out: string[][] = [];
  let last = 0;
  TAG_RE.lastIndex = 0;
  for (let m = TAG_RE.exec(body); m !== null; m = TAG_RE.exec(body)) {
    if (m.index > last) out.push(['text', body.substring(last, m.index)]);
    out.push([m[1], m[0]]);
    last = m.index + m[0].length;
  }
  if (last < body.length) out.push(['text', body.substring(last)]);
  if (out.length === 0) out.push(['text', body]);
  return out;
};
const shape = (segs: MessageSegment[]) => segs.map((s) => (s.kind === 'text' ? ['text', s.text] : [s.kind, s.raw]));

describe('parseEmbeds', () => {
  it('splits bodies exactly as the regex parser did', () => {
    const parts = ['--embed[', '--download[', ']--', ']', '--', 'a', '\n', '\r', 'name=x.png', ',', '['];
    // Deterministic pseudo-random bodies built from tag fragments.
    let seed = 7;
    const next = () => (seed = (seed * 1103515245 + 12345) % 2147483648);
    for (let i = 0; i < 3000; i++) {
      let body = '';
      const n = next() % 12;
      for (let j = 0; j < n; j++) body += parts[next() % parts.length];
      expect(shape(parseEmbeds(body)), JSON.stringify(body)).toEqual(reference(body));
    }
  });

  it('parses text, embeds and downloads around each other', () => {
    const body = 'hi --embed[name=a.png,type=image/png,data=QUJD]-- mid --download[nick=bob,filename=f.txt,size=3,mime=text/plain]-- end';
    expect(parseEmbeds(body).map((s) => s.kind)).toEqual(['text', 'embed', 'text', 'download', 'text']);
  });

  it('stays fast on many unclosed tags', () => {
    fast(() => parseEmbeds('--embed['.repeat(100_000)));
  });
});

// The pre-fix block regexes, kept to check the rewrites capture every line
// without a CR, U+2028 or U+2029 the same way.
const OLD = {
  fence: /^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)\s*$/,
  atx: /^\s{0,3}(#{1,6})\s+(.*)$/,
  quote: /^\s{0,3}>\s?(.*)$/,
  ul: /^(\s*)[-*+]\s+(.*)$/,
  ol: /^(\s*)\d{1,9}[.)]\s+(.*)$/,
};
const NEW = { fence: FENCE_RE, atx: ATX_RE, quote: QUOTE_RE, ul: UL_RE, ol: OL_RE };

describe('parseBlocks', () => {
  it('reads fences, headings, quotes and lists as the old regexes did', () => {
    const parts = ['```', '~~~', '`', ' ', '  ', '\t', 'a', 'go', '#', '##', '>', '-', '*', '+', '1.', '2)', 'x y'];
    let seed = 11;
    const next = () => (seed = (seed * 1103515245 + 12345) % 2147483648);
    for (let i = 0; i < 5000; i++) {
      let line = '';
      const n = 1 + (next() % 6);
      for (let j = 0; j < n; j++) line += parts[next() % parts.length];
      for (const name of Object.keys(OLD) as (keyof typeof OLD)[]) {
        const was = line.match(OLD[name])?.slice(1) ?? null;
        const now = line.match(NEW[name])?.slice(1).map((g) => g ?? '') ?? null;
        expect(now, `${name} ${JSON.stringify(line)}`).toEqual(was);
      }
    }
  });

  it('renders headings, quotes and lists from a CRLF sender', () => {
    expect(parseBlocks('# Title\r\nbody\r\n')[0]).toMatchObject({ kind: 'heading', level: 1 });
    expect(parseBlocks('> hi\r\n')[0]).toMatchObject({ kind: 'quote' });
    expect(parseBlocks('- a\r\n- b\r\n')[0]).toMatchObject({ kind: 'list', ordered: false });
  });

  it('stays fast on long whitespace runs before a CR or a stray backtick', () => {
    const run = ' '.repeat(100_000);
    fast(() => parseBlocks('```' + run + 'a`'));
    fast(() => parseBlocks('# ' + run + '\ra\rb'));
    fast(() => parseBlocks('- ' + run + '\ra\rb'));
    fast(() => parseBlocks('1. ' + run + '\ra\rb'));
  });
});

describe('stripEmbedTags', () => {
  const oldStrip = (s: string) => s.replace(/--(embed|download)\[.*?\]--/g, ' ').replace(/\s+/g, ' ').trim();

  it('reduces bodies exactly as the regex strip did', () => {
    const parts = ['--embed[', '--download[', ']--', ']', '--', 'a', ' ', '\n', '\r', 'name=x.png', ',', '['];
    let seed = 5;
    const next = () => (seed = (seed * 1103515245 + 12345) % 2147483648);
    for (let i = 0; i < 3000; i++) {
      let body = '';
      const n = next() % 12;
      for (let j = 0; j < n; j++) body += parts[next() % parts.length];
      expect(stripEmbedTags(body), JSON.stringify(body)).toBe(oldStrip(body));
    }
  });

  it('keeps the text around embeds and downloads', () => {
    const body = 'look  --embed[name=a.png,type=image/png,data=QUJD]--\nhere --download[nick=bob,filename=f.txt,size=3]-- done';
    expect(stripEmbedTags(body)).toBe('look here done');
    expect(stripEmbedTags('a--embed[name=b.png]--c')).toBe('a c');
  });

  it('stays fast on many unclosed tags', () => {
    fast(() => stripEmbedTags('--embed['.repeat(100_000)));
  });
});
