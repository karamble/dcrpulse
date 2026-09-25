import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { linkifyChatText } from './chatLinkify';
import { parseBlocks } from './chatMarkdown';
import { parseEmbeds, type MessageSegment } from './embedParser';

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
