import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { DexMarket, MMCexStatus } from '../../services/dcrdexApi';
import { DexMMBotTypeSelector } from './DexMMBotTypeSelector';

const dcrBtc = { base: 'DCR', quote: 'BTC', baseID: 42, quoteID: 0 } as DexMarket;

const open = (cexes: Record<string, MMCexStatus>) =>
  render(
    <DexMMBotTypeSelector
      market={dcrBtc}
      cexes={cexes}
      onContinue={() => {}}
      onChangeMarket={() => {}}
      onCexSaved={() => {}}
      onCancel={() => {}}
    />,
  );

afterEach(cleanup);

describe('DexMMBotTypeSelector exchanges', () => {
  it('lets keys be set up when no exchange is configured yet', () => {
    open({});
    expect(
      screen.getByText(
        'Only the basic market making bot is available. To enable arbitrage options, configure your exchange API.',
      ),
    ).toBeTruthy();
    expect((screen.getByRole('button', { name: /Market Maker \+ Arbitrage/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByText('Binance US'));
    expect(screen.getByRole('button', { name: 'Save Binance US keys' })).toBeTruthy();
    expect((screen.getByPlaceholderText('API key') as HTMLInputElement).type).toBe('password');
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByPlaceholderText('API key')).toBeNull();
  });

  it('points to other exchanges when a configured one lacks the market', () => {
    open({ Binance: { config: { name: 'Binance' }, connected: true, markets: {} } });
    expect(
      screen.getByText(
        'Only the basic market making bot is available for this market. Configure other exchanges to maximize your options.',
      ),
    ).toBeTruthy();
  });
});
