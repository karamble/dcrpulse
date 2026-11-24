import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { TxDexSwap, WalletTransaction } from '../services/api';
import { fmtMatch } from './dex/dexFormat';
import {
  DexSwapLine,
  TxCategoryIcon,
  dexPair,
  dexStatus,
  formatTxAmount,
  isFoldedTx,
  txAmountColor,
  txCategoryLabel,
  txWhen,
} from './walletTxFormat';

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
    const now = Date.now();
    vi.useFakeTimers();
    vi.setSystemTime(now);
    const hour = 3600_000;
    const t = wtx({ blockTime: Math.floor((now - 2 * hour) / 1000), time: new Date(now - 9 * 24 * hour).toISOString() });
    expect(txWhen(t)).toBe('2h ago');
  });
});

const dexSwap = (fields: Partial<TxDexSwap>): TxDexSwap => ({
  role: 'swap',
  orderId: 'o1',
  host: 'dex.example:7232',
  baseId: 42,
  quoteId: 0,
  baseSymbol: 'dcr',
  quoteSymbol: 'btc',
  baseConv: 1e8,
  quoteConv: 1e8,
  sell: true,
  matches: [{ qty: 40e8, rate: 17110, status: 'MatchConfirmed' }],
  ...fields,
});

describe('DEX rows', () => {
  it('names each bisonw tx type, ahead of the CoinJoin and VSP labels', () => {
    expect(txCategoryLabel(wtx({ dexSwap: dexSwap({}) }))).toBe('DEX Swap');
    expect(txCategoryLabel(wtx({ category: 'receive', dexSwap: dexSwap({ role: 'redeem', sell: false }) }))).toBe('DEX Redeem');
    expect(txCategoryLabel(wtx({ category: 'receive', dexSwap: dexSwap({ role: 'refund' }) }))).toBe('DEX Refund');
    expect(txCategoryLabel(wtx({ category: 'self', dexSwap: dexSwap({ role: 'funding', matches: [] }) }))).toBe('DEX Order Funding');
    expect(txCategoryLabel(wtx({ isMixed: true, dexSwap: dexSwap({}) }))).toBe('DEX Swap');
    expect(txCategoryLabel(wtx({ category: 'vspfee', dexSwap: dexSwap({}) }))).toBe('DEX Swap');
    expect(iconColor(wtx({ isMixed: true, dexSwap: dexSwap({}) }))).toContain('text-cyan-600');
  });

  it('keeps the send and receive amount colours', () => {
    expect(txAmountColor(wtx({ dexSwap: dexSwap({}) }))).toBe('text-red-500');
    expect(txAmountColor(wtx({ category: 'receive', dexSwap: dexSwap({ role: 'redeem' }) }))).toBe('text-success');
  });

  it('names the pair in trade direction', () => {
    expect(dexPair(dexSwap({}))).toBe('DCR → BTC');
    expect(dexPair(dexSwap({ sell: false }))).toBe('BTC → DCR');
  });

  it('shows one status, the worst across the matches, and none once settled', () => {
    const m = { qty: 40e8, rate: 17110, status: 'TakerSwapCast' };
    expect(dexStatus(dexSwap({}))).toBeNull();
    expect(dexStatus(dexSwap({ matches: [{ ...m, active: true }] }))?.word).toBe('Settling');
    expect(dexStatus(dexSwap({ matches: [{ ...m, active: true }, { ...m, revoked: true }] }))?.word).toBe('Revoked');
    expect(dexStatus(dexSwap({ matches: [{ ...m, revoked: true, refunded: true }] }))?.word).toBe('Refunded');
    expect(dexStatus(dexSwap({ role: 'refund', matches: [{ ...m, refunded: true }] }))).toBeNull();
  });

  it('renders matches as the DEX order detail does, without totals', () => {
    expect(fmtMatch(40e8, 17110, 1e8, 1e8, 'DCR', 'BTC')).toBe('40 DCR @ 0.00017110 BTC');
    const three = [17120, 17110, 17060].map((rate) => ({ qty: 40e8, rate, status: 'MatchConfirmed' }));
    const line = render(<DexSwapLine swap={dexSwap({ matches: three })} />).container.textContent;
    expect(line).toContain('DCR → BTC');
    expect(line).toContain('40 DCR @ 0.00017060 BTC');
    expect(line).not.toContain('120 DCR');
    const four = [...three, { qty: 40e8, rate: 17000, status: 'MatchConfirmed' }];
    cleanup();
    const capped = render(<DexSwapLine swap={dexSwap({ matches: four })} />).container.textContent;
    expect(capped).toContain('+2 more');
    expect(capped).not.toContain('0.00017060');
  });

  it('skips only the splits folded into a swap row', () => {
    expect(isFoldedTx(wtx({ dexSwap: dexSwap({ role: 'funding', foldedInto: 'abc' }) }))).toBe(true);
    expect(isFoldedTx(wtx({ dexSwap: dexSwap({ role: 'funding' }) }))).toBe(false);
    expect(isFoldedTx(wtx({}))).toBe(false);
  });
});
