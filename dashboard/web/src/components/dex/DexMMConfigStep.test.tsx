import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DexAsset, DexMarket, DexWalletState, MMBotConfig, MMMarketReport } from '../../services/dcrdexApi';
import * as api from '../../services/dcrdexApi';
import { DexMMConfigStep } from './DexMMConfigStep';
import { defaultDraft, draftFromConfig, type BotType, type ConfigDraft } from './dexMMConfig';

vi.mock('../../services/dcrdexApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/dcrdexApi')>()),
  getMMAvailableBalances: vi.fn(),
  getDexWallets: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(api.getMMAvailableBalances).mockRejectedValue(new Error('no market-maker config saved yet'));
  vi.mocked(api.getDexWallets).mockResolvedValue([]);
});

const dcrBtc = {
  base: 'DCR',
  quote: 'BTC',
  baseID: 42,
  quoteID: 0,
  lotSize: 100_000_000,
  rateStep: 1,
  baseConvFactor: 100_000_000,
  quoteConvFactor: 100_000_000,
} as DexMarket;

const onSave = vi.fn(async (_: MMBotConfig) => {});

const open = (
  botType: BotType = 'basicmm',
  opts: {
    editing?: boolean;
    running?: boolean;
    initial?: ConfigDraft;
    report?: MMMarketReport;
    market?: DexMarket;
    catalog?: DexAsset[];
    cexMarket?: { baseMinWithdraw: number; quoteMinWithdraw: number };
  } = {},
) =>
  render(
    <DexMMConfigStep
      host="dex.example:7232"
      market={opts.market ?? dcrBtc}
      initial={opts.initial ?? defaultDraft(botType, botType === 'basicmm' ? undefined : 'Binance')}
      editing={!!opts.editing}
      running={opts.running}
      report={opts.report ?? null}
      catalog={opts.catalog ?? []}
      cexMarket={opts.cexMarket}
      onChangeMarket={() => {}}
      onChangeBotType={() => {}}
      onSave={onSave}
      onSaveAndStart={vi.fn()}
      onCancel={() => {}}
    />,
  );

const type = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });

const saved = async (): Promise<MMBotConfig> => {
  fireEvent.click(within(screen.getByRole('group', { name: 'Bot actions' })).getAllByRole('button')[1]);
  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  return onSave.mock.calls[0][0];
};

const value = (label: string) => (screen.getByLabelText(label) as HTMLInputElement).value;

const actions = () =>
  within(screen.getByRole('group', { name: 'Bot actions' }))
    .getAllByRole('button')
    .map((b) => b.textContent);

afterEach(cleanup);

describe('DexMMConfigStep actions', () => {
  it('offers Cancel, Create bot and Create & start, in that order, on both tabs', () => {
    open();
    expect(actions()).toEqual(['Cancel', 'Create bot', 'Create & start']);
    fireEvent.click(screen.getByRole('button', { name: 'advanced' }));
    expect(actions()).toEqual(['Cancel', 'Create bot', 'Create & start']);
  });

  it('saves an existing bot, or applies to a running one, without a start button', () => {
    open('basicmm', { editing: true });
    expect(actions()).toEqual(['Cancel', 'Save changes']);
    cleanup();
    open('basicmm', { editing: true, running: true });
    expect(actions()).toEqual(['Cancel', 'Apply to running bot']);
  });
});

describe('DexMMConfigStep Quick tab', () => {
  it("saves a new basic bot's shown profit threshold", async () => {
    open('basicmm');
    expect(value('Profit threshold')).toBe('2');
    const cfg = await saved();
    expect(cfg.basicMarketMakingConfig).toMatchObject({
      gapStrategy: 'percent-plus',
      buyPlacements: [{ lots: 1, gapFactor: 0.02 }],
      sellPlacements: [{ lots: 1, gapFactor: 0.02 }],
    });
  });

  it('saves the profit threshold as the arb market maker profit', async () => {
    open('arbmm');
    type('Profit threshold', '3');
    expect((await saved()).arbMarketMakingConfig?.profit).toBe(0.03);
  });

  it('saves the simple arbitrage lots and trigger', async () => {
    open('simplearb');
    type('Lots per side', '6');
    type('Profit threshold', '0.5');
    const cfg = await saved();
    expect(cfg.uiConfig?.simpleArbLots).toBe(6);
    expect(cfg.simpleArbConfig?.profitTrigger).toBe(0.005);
  });

  it("follows the market's lots step until the bot is edited", async () => {
    // One lot is 1 DCR at 12.5 USD, so about 100 USD is 8 lots.
    open('basicmm', { report: { price: 0, oracles: [], baseFiatRate: 12.5, quoteFiatRate: 60000 } });
    expect(value('Lots per level')).toBe('8');
    expect((await saved()).basicMarketMakingConfig?.buyPlacements).toEqual([{ lots: 8, gapFactor: 0.02 }]);
  });

  it('reopens a saved bot on the tab that made it', () => {
    const quickMade: MMBotConfig = {
      host: 'dex.example:7232',
      baseID: 42,
      quoteID: 0,
      basicMarketMakingConfig: {
        gapStrategy: 'percent-plus',
        driftTolerance: 0.002,
        buyPlacements: [
          { lots: 2, gapFactor: 0.01 },
          { lots: 2, gapFactor: 0.015 },
        ],
        sellPlacements: [
          { lots: 2, gapFactor: 0.01 },
          { lots: 2, gapFactor: 0.015 },
        ],
      },
    };
    open('basicmm', { editing: true, initial: draftFromConfig(quickMade) });
    expect(value('Profit threshold')).toBe('1');
    expect(value('Price increment')).toBe('0.5');
    expect(value('Price levels per side')).toBe('2');
    cleanup();
    const manual = { ...quickMade.basicMarketMakingConfig!, gapStrategy: 'multiplier' as const };
    open('basicmm', { editing: true, initial: draftFromConfig({ ...quickMade, basicMarketMakingConfig: manual }) });
    expect(screen.queryByLabelText('Profit threshold')).toBeNull();
    expect(screen.getByText('Gap strategy')).toBeTruthy();
  });

  it('reads manual placements back into Quick when switching tabs', () => {
    open('basicmm');
    fireEvent.click(screen.getByRole('button', { name: 'advanced' }));
    const buys = within(screen.getByRole('table', { name: 'Buy placements' }));
    fireEvent.click(buys.getByRole('button', { name: 'Remove' }));
    fireEvent.change(buys.getByLabelText('Buy lots'), { target: { value: '1' } });
    fireEvent.change(buys.getByLabelText('Buy percent'), { target: { value: '4' } });
    fireEvent.click(buys.getByRole('button', { name: 'Add buy placement' }));
    fireEvent.click(screen.getByRole('button', { name: 'quick' }));
    // The best buy (4%) and best sell (2%) average to a 3% profit threshold.
    expect(value('Profit threshold')).toBe('3');
  });
});

const has = (label: string) => screen.queryByLabelText(label) !== null;
const knobs = () => ['Drift tolerance', 'Order persistence', 'Max active arbs'].filter(has);
const toAdvanced = () => fireEvent.click(screen.getByRole('button', { name: 'advanced' }));

describe('DexMMConfigStep knobs', () => {
  it('shows each bot type its knobs on both tabs', () => {
    open('basicmm');
    expect(knobs()).toEqual(['Drift tolerance']);
    expect(screen.queryByText('Arbitrage rebalance')).toBeNull();
    toAdvanced();
    expect(knobs()).toEqual(['Drift tolerance']);
    cleanup();
    open('arbmm');
    expect(knobs()).toEqual(['Drift tolerance', 'Order persistence']);
    expect(screen.getByText('Arbitrage rebalance')).toBeTruthy();
    toAdvanced();
    expect(knobs()).toEqual(['Drift tolerance', 'Order persistence']);
    cleanup();
    open('simplearb');
    expect(knobs()).toEqual(['Order persistence', 'Max active arbs']);
    expect(screen.queryByRole('button', { name: 'advanced' })).toBeNull();
  });

  it("saves bisonw's knob defaults", async () => {
    open('arbmm');
    expect((await saved()).arbMarketMakingConfig).toMatchObject({ driftTolerance: 0.002, orderPersistence: 20 });
    cleanup();
    onSave.mockClear();
    open('simplearb');
    expect((await saved()).simpleArbConfig).toMatchObject({ numEpochsLeaveOpen: 20, maxActiveArbs: 100 });
  });

  it('takes drift tolerance in percent', async () => {
    open('basicmm');
    type('Drift tolerance', '0.5');
    expect((await saved()).basicMarketMakingConfig?.driftTolerance).toBe(0.005);
  });
});

const unit = (conversionFactor: number) => ({ conversionFactor }) as DexAsset['unitInfo'];
const catalog = [
  { id: 42, symbol: 'dcr', isAccountBased: false, unitInfo: unit(1e8), availableWallets: [] },
  { id: 0, symbol: 'btc', isAccountBased: false, unitInfo: unit(1e8), availableWallets: [] },
  {
    id: 60,
    symbol: 'eth',
    isAccountBased: true,
    unitInfo: unit(1e9),
    availableWallets: [],
    tokens: [{ id: 60001, parentID: 60, symbol: 'usdc.eth', unitInfo: unit(1e6), definition: {} }],
  },
] as unknown as DexAsset[];
const dcrUsdc = { ...dcrBtc, quote: 'USDC.ETH', quoteID: 60001, quoteConvFactor: 1e6 } as DexMarket;
const pane = (symbol: string) => within(screen.getByRole('region', { name: `${symbol} settings` }));
const fees = { max: { swap: 1000, redeem: 1000, refund: 1000 }, estimated: { swap: 500, redeem: 500, refund: 500 } };
const report: MMMarketReport = { price: 0, oracles: [], baseFiatRate: 20, quoteFiatRate: 60000, baseFees: fees, quoteFees: fees };

describe('DexMMConfigStep asset panes', () => {
  it("shows each asset's settings on both tabs", () => {
    open('basicmm', { market: dcrUsdc, catalog });
    const check = () => {
      expect(pane('DCR').queryByLabelText('Order reserves')).not.toBeNull();
      expect(pane('DCR').queryByLabelText('Slippage buffer')).toBeNull();
      expect(pane('DCR').queryByLabelText('Swap fee reserves')).toBeNull();
      expect(pane('USDC.ETH').queryByLabelText('Slippage buffer')).not.toBeNull();
      expect(pane('USDC.ETH').queryByLabelText('Swap fee reserves')).not.toBeNull();
    };
    check();
    toAdvanced();
    check();
    cleanup();
    open('simplearb', { market: dcrUsdc, catalog });
    expect(pane('DCR').queryByLabelText('Order reserves')).toBeNull();
  });

  it('sets the minimum transfer as a coin amount between the exchange minimum and the commit', async () => {
    open('arbmm', { catalog, report, cexMarket: { baseMinWithdraw: 10_000_000, quoteMinWithdraw: 1_000 } });
    const dcrMin = pane('DCR').getByLabelText('Minimum transfer');
    fireEvent.change(dcrMin, { target: { value: '0.1' } });
    const btcMin = pane('BTC').getByLabelText('Minimum transfer') as HTMLInputElement;
    fireEvent.change(btcMin, { target: { value: '1000' } });
    // Past the top of the range the field shows the most a save would store.
    fireEvent.blur(btcMin);
    expect(Number(btcMin.value)).toBeLessThan(1);
    const ui = (await saved()).uiConfig;
    expect(ui?.baseConfig.transferFactor).toBe(0);
    expect(ui?.quoteConfig.transferFactor).toBe(1);
  });

  it('hides the minimum transfer while rebalancing is off', () => {
    open('arbmm', { catalog, report, cexMarket: { baseMinWithdraw: 10_000_000, quoteMinWithdraw: 1_000 } });
    expect(pane('DCR').queryByLabelText('Minimum transfer')).not.toBeNull();
    fireEvent.click(screen.getByRole('checkbox', { name: /Arbitrage rebalance/ }));
    expect(pane('DCR').queryByLabelText('Minimum transfer')).toBeNull();
  });
});

const table = (side: 'Buy' | 'Sell') => within(screen.getByRole('table', { name: `${side} placements` }));
// around is the text of a placement table's section, where its errors show.
const around = (side: 'Buy' | 'Sell') =>
  screen.getByRole('table', { name: `${side} placements` }).parentElement!.textContent;
const headers = (side: 'Buy' | 'Sell') => table(side).getAllByRole('columnheader').map((h) => h.textContent);
const addRow = (side: 'Buy' | 'Sell', lots: string, gap: string, gapLabel = 'percent') => {
  fireEvent.change(table(side).getByLabelText(`${side} lots`), { target: { value: lots } });
  fireEvent.change(table(side).getByLabelText(`${side} ${gapLabel}`), { target: { value: gap } });
  fireEvent.click(table(side).getByRole('button', { name: `Add ${side.toLowerCase()} placement` }));
};

describe('DexMMConfigStep Advanced tab', () => {
  it("names each gap strategy and keeps each strategy's own placements", () => {
    open('basicmm');
    toAdvanced();
    const select = screen.getByLabelText('Gap strategy') as HTMLSelectElement;
    expect([...select.options].map((o) => o.text)).toEqual(['Percent Plus', 'Percent', 'Absolute Plus', 'Absolute', 'Multiplier']);
    expect(screen.getByText(/adds the specified spread, calculated as a percentage of the mid-gap price/)).toBeTruthy();
    expect(headers('Buy')).toEqual(['Priority', 'Lots', 'Percent', '']);
    expect(table('Buy').getByText('2 %')).toBeTruthy();

    fireEvent.change(select, { target: { value: 'multiplier' } });
    expect(screen.getByText(/the specified multiple of the break-even spread/)).toBeTruthy();
    expect(headers('Buy')).toEqual(['Priority', 'Lots', 'Multiplier', '']);
    expect(table('Buy').queryAllByRole('button', { name: 'Remove' })).toHaveLength(0);
    fireEvent.change(select, { target: { value: 'percent-plus' } });
    expect(table('Buy').getByText('2 %')).toBeTruthy();
  });

  it("checks a placement with bisonw's rules before adding it", async () => {
    open('basicmm');
    toAdvanced();
    addRow('Buy', '1', '2');
    expect(around('Buy')).toContain('Duplicate placement');
    addRow('Buy', '0', '3');
    expect(around('Buy')).toContain('Lots must be greater than 0');
    addRow('Buy', '1', '12');
    expect(around('Buy')).toContain('Percent must be between 0 and 10');
    addRow('Buy', '3', '1.5');
    expect((await saved()).basicMarketMakingConfig?.buyPlacements).toEqual([
      { lots: 1, gapFactor: 0.02 },
      { lots: 3, gapFactor: 0.015 },
    ]);
  });

  it('saves placements in the priority order set', async () => {
    open('basicmm');
    toAdvanced();
    addRow('Sell', '5', '3');
    fireEvent.click(table('Sell').getByRole('button', { name: 'Move up' }));
    expect((await saved()).basicMarketMakingConfig?.sellPlacements).toEqual([
      { lots: 5, gapFactor: 0.03 },
      { lots: 1, gapFactor: 0.02 },
    ]);
  });

  it('enters arb market maker placements as a match buffer multiplier and its profit in percent', async () => {
    open('arbmm');
    toAdvanced();
    expect(headers('Buy')).toEqual(['Priority', 'Lots', 'Match buffer', '']);
    expect(table('Buy').getByText('1 x')).toBeTruthy();
    addRow('Buy', '2', '0.5', 'match buffer');
    expect(around('Buy')).toContain('Multiplier must be between 1 and 100');
    addRow('Buy', '2', '1.5', 'match buffer');
    type('Profit threshold', '1.5');
    const arb = (await saved()).arbMarketMakingConfig;
    expect(arb?.profit).toBe(0.015);
    expect(arb?.buyPlacements).toEqual([
      { lots: 1, multiplier: 1 },
      { lots: 2, multiplier: 1.5 },
    ]);
  });
});

// row is the value beside a label in an asset pane's breakdown.
const row = (symbol: string, label: string) =>
  pane(symbol).getByText(label).closest('div')!.querySelector('dd')!.textContent;

describe('DexMMConfigStep breakdown', () => {
  it("itemises each asset's commit", () => {
    open('basicmm', { catalog, report });
    // One lot is 20 USD, so the lots step is 5 lots of 1 DCR.
    expect(row('DCR', 'Order inventory')).toBe('5 lots × 1 = 5 DCR');
    // Reserves show their amount beside their setting: 5 lots of 0.00033333 BTC
    // reserved once over, and 5% of book plus reserves held against slippage.
    expect(pane('BTC').getByText('= 0.00166667 BTC')).toBeTruthy();
    expect(pane('BTC').getByText('= 0.00016667 BTC')).toBeTruthy();
    expect(pane('DCR').getByText('= 5 DCR')).toBeTruthy();
    expect(pane('DCR').queryByText('CEX inventory')).toBeNull();
    expect(row('DCR', 'Total')).toMatch(/^[\d.,]+ DCR~ \$/);
  });

  it("shows what the daemon has free for the bot, split by venue", async () => {
    vi.mocked(api.getMMAvailableBalances).mockResolvedValue({
      dexBalances: { 42: 700_000_000 },
      cexBalances: { 42: 200_000_000 },
    } as unknown as Awaited<ReturnType<typeof api.getMMAvailableBalances>>);
    open('arbmm', { catalog, report });
    await waitFor(() => expect(row('DCR', 'Available')).toBe('9 DCR'));
    expect(row('DCR', '├─ DEX')).toBe('7 DCR');
    expect(row('DCR', '└─ CEX')).toBe('2 DCR');
    expect(pane('DCR').getByText('CEX inventory')).toBeTruthy();
  });

  it('falls back to the wallet balance before the bot is first saved', async () => {
    vi.mocked(api.getDexWallets).mockResolvedValue([{ assetID: 42, available: 3.5 } as DexWalletState]);
    open('basicmm', { catalog, report });
    await waitFor(() => expect(row('DCR', 'Available')).toBe('3.5 DCR'));
  });
});
