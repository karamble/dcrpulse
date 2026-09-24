import { describe, expect, it } from 'vitest';
import { oggDurationSeconds } from './oggDuration';

// A minimal Ogg page: the parser reads only the capture pattern, the granule
// and the lacing table, so the CRC can stay zero here.
const page = (granule: number, payload: number[]): number[] => {
  const g = [];
  let rest = granule;
  for (let i = 0; i < 8; i++) {
    g.push(rest % 256);
    rest = Math.floor(rest / 256);
  }
  return [0x4f, 0x67, 0x67, 0x53, 0, 0, ...g, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, payload.length, ...payload];
};

describe('oggDurationSeconds', () => {
  it('reads the last audio page granule and ignores the headers', () => {
    const bytes = new Uint8Array([
      ...page(0, [1, 2, 3]),
      ...page(0, [4]),
      ...page(960, [5]),
      ...page(2880, [6, 7]),
    ]);
    expect(oggDurationSeconds(bytes)).toBe(0.06);
  });

  it('handles the 60 second maximum without precision loss', () => {
    expect(oggDurationSeconds(new Uint8Array(page(2_880_000, [1])))).toBe(60);
  });

  it('returns null for something that is not Ogg or is cut short', () => {
    expect(oggDurationSeconds(new Uint8Array([1, 2, 3]))).toBeNull();
    expect(oggDurationSeconds(new Uint8Array(page(960, [1, 2]).slice(0, -1)))).toBeNull();
    expect(oggDurationSeconds(new Uint8Array(page(0, [1])))).toBeNull();
  });
});
