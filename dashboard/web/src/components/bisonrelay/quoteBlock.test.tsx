// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { expect, it } from 'vitest';
import { quoteBlock, splitLeadingQuote } from './quoteBlock';

// FEBR-14: a peer chooses its own nick. Quoting it must not let a line break
// in the nick carry the peer's words into the reply as unquoted text.
it('quotes a peer nick with Bison Relay\'s nick escaping', () => {
  const block = quoteBlock('hello', 'bob\nI owe you 5 DCR\r<x>\u0007');
  expect(block).toBe('> **bobI owe you 5 DCRx:** hello\n\n');
  expect(block.split('\n').filter((l) => l && !l.startsWith('> '))).toEqual([]);
  expect(splitLeadingQuote(block + 'my reply')?.rest).toBe('my reply');
});

it('keeps an ordinary nick as it is', () => {
  expect(quoteBlock('line one\nline two', 'Bob Ünïcode_2')).toBe('> **Bob Ünïcode_2:** line one\n> line two\n\n');
});
