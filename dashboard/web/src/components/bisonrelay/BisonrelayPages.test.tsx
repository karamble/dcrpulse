import { afterEach, describe, expect, it } from 'vitest';
import { decodeSegments, readHash, resolvePageLink } from './BisonrelayPages';

const UID = 'ab'.repeat(32);

afterEach(() => {
  window.location.hash = '';
});

describe('decodeSegments', () => {
  it('decodes percent-encoded segments', () => {
    expect(decodeSegments(['a%20b', 'c'])).toEqual(['a b', 'c']);
  });

  it('rejects a malformed escape instead of throwing', () => {
    expect(decodeSegments(['ok', '%E0'])).toBeNull();
  });

  it('drops invisible direction and zero-width characters', () => {
    expect(decodeSegments(['a‮b', '%E2%80%AEx', 'z%E2%80%8Bw'])).toEqual(['ab', 'x', 'zw']);
  });
});

describe('readHash', () => {
  it('falls back to My Pages for a route that does not decode', () => {
    window.location.hash = `#pages/visit/${UID}/%E0`;
    expect(readHash()).toEqual({ kind: 'mine' });
    window.location.hash = '#pages/edit/%';
    expect(readHash()).toEqual({ kind: 'mine' });
  });

  it('still parses a valid visit route', () => {
    window.location.hash = `#pages/visit/${UID}/shop/item%201.md`;
    expect(readHash()).toEqual({ kind: 'visit', uid: UID, path: ['shop', 'item 1.md'] });
  });
});

describe('resolvePageLink', () => {
  it('ignores a link that does not decode', () => {
    expect(resolvePageLink(`br://${UID}/%E0`, UID)).toBeNull();
    expect(resolvePageLink('%', UID)).toBeNull();
  });

  it('resolves valid br:// and relative links', () => {
    expect(resolvePageLink(`br://${UID}/a%20b.md`, 'me')).toEqual({ uid: UID, path: ['a b.md'] });
    expect(resolvePageLink('/docs/x.md', UID)).toEqual({ uid: UID, path: ['docs', 'x.md'] });
  });
});
