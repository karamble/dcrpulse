import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { WalletTransaction } from '../services/api';
import { TxCategoryIcon, formatTxAmount, txAmountColor, txCategoryLabel, txWhen } from './walletTxFormat';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const wtx = (fields: Partial<WalletTransaction>) => ({ category: 'send', txType: 'regular', ...fields }) as WalletTransaction;
const iconColor = (t: WalletTransaction) =>
  render(<TxCategoryIcon tx={t} className="h-4 w-4" />).container.querySelector('svg')!.getAttribute('class');

describe('wallet transaction rows', () => {
  it('labels every kind the same on both pages', () => {
    expect(txCategoryLabel(wtx({ category: 'send', isMixed: true }))).toBe('Sent (CoinJoin)');
    expect(txCategoryLabel(wtx({ category: 'immature' }))).toBe('Immature');
    expect(txCategoryLabel(wtx({ isChannelFunding: true }))).toBe('Channel Open');
    expect(txCategoryLabel(wtx({ txType: 'ticket' }))).toBe('Ticket Purchase');
  });

  it('shows the mix icon only for mixed sends and receives', () => {
    expect(iconColor(wtx({ category: 'send', isMixed: true }))).toContain('text-purple-500');
    expect(iconColor(wtx({ category: 'self', isMixed: true }))).toContain('text-muted-foreground');
  });

  it('colours mined amounts', () => {
    expect(txAmountColor(wtx({ category: 'generate' }))).toBe('text-primary');
  });

  it('keeps fee-sized amounts visible on the overview and 8 decimals on the history', () => {
    expect(formatTxAmount(-1.23456789, true)).toBe('-1.2346 DCR');
    expect(formatTxAmount(0.00012345, true)).toBe('+0.00012345 DCR');
    expect(formatTxAmount(1.5)).toBe('+1.50000000 DCR');
  });

  it('dates a mined transaction by its block', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-09-29T12:00:00Z'));
    const t = wtx({ blockTime: Date.parse('2026-09-29T10:00:00Z') / 1000, time: '2026-09-20T00:00:00Z' });
    expect(txWhen(t)).toBe('2h ago');
  });
});
