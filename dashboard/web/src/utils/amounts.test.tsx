// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { describe, expect, it } from 'vitest';
import { formatAtomsDcr, formatDcr, isDcrAmountInput } from './amounts';

// Each helper must print exactly what the local copies it replaced printed.
describe('formatAtomsDcr', () => {
  it('matches (atoms / 1e8).toFixed(8) + " DCR"', () => {
    for (const atoms of [0, 1, 12345678, 100000000, 2_100_000_000_000_000]) {
      expect(formatAtomsDcr(atoms)).toBe((atoms / 1e8).toFixed(8) + ' DCR');
    }
    expect(formatAtomsDcr(12345678)).toBe('0.12345678 DCR');
  });
});

describe('formatDcr', () => {
  it('matches v.toFixed(8), or the decimals asked for', () => {
    for (const dcr of [0, 0.5, 12.3456789, 21000000]) {
      expect(formatDcr(dcr)).toBe(dcr.toFixed(8));
      expect(formatDcr(dcr, 4)).toBe(dcr.toFixed(4));
    }
    expect(formatDcr(12.3456789, 4)).toBe('12.3457');
  });
});

describe('isDcrAmountInput', () => {
  it('accepts partial amounts on the way to a valid one', () => {
    for (const raw of ['', '0', '0.', '.', '.5', '1.00000001', '21000000']) {
      expect(isDcrAmountInput(raw)).toBe(true);
    }
  });

  it('refuses what an amount can never become', () => {
    for (const raw of ['1e3', '-1', '0x10', '+5', '0.000000001', '1,5', '1.2.3']) {
      expect(isDcrAmountInput(raw)).toBe(false);
    }
  });
});
