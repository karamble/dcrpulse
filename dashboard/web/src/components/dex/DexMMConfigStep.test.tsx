import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DexMarket, MMBotConfig, MMMarketReport } from '../../services/dcrdexApi';
import { DexMMConfigStep } from './DexMMConfigStep';
import { defaultDraft, draftFromConfig, type BotType, type ConfigDraft } from './dexMMConfig';

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
  opts: { editing?: boolean; running?: boolean; initial?: ConfigDraft; report?: MMMarketReport } = {},
) =>
  render(
    <DexMMConfigStep
      host="dex.example:7232"
      market={dcrBtc}
      initial={opts.initial ?? defaultDraft(botType, botType === 'basicmm' ? undefined : 'Binance')}
      editing={!!opts.editing}
      running={opts.running}
      report={opts.report ?? null}
      catalog={[]}
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
    fireEvent.change(screen.getAllByPlaceholderText('gap')[0], { target: { value: '0.04' } });
    fireEvent.click(screen.getByRole('button', { name: 'quick' }));
    // The best buy (4%) and best sell (2%) average to a 3% profit threshold.
    expect(value('Profit threshold')).toBe('3');
  });
});
