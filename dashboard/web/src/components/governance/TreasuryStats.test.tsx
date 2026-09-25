import { describe, expect, it } from 'vitest';
import { runwayText } from './TreasuryStats';

describe('runwayText', () => {
  it('writes whole months as years and months', () => {
    expect(runwayText(324)).toBe('27 years');
    expect(runwayText(325)).toBe('27 years 1 month');
    expect(runwayText(13)).toBe('1 year 1 month');
    expect(runwayText(5)).toBe('5 months');
    expect(runwayText(0)).toBe('0 months');
  });
});
