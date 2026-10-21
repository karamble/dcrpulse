import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DexMarket, MMBotConfig, MMBotStatus } from '../../services/dcrdexApi';
import * as api from '../../services/dcrdexApi';
import { DexMMWizard } from './DexMMWizard';

vi.mock('../../services/dcrdexApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/dcrdexApi')>()),
  getDexAssetCatalog: vi.fn(),
  getMMMarketReport: vi.fn(),
  getMMAvailableBalances: vi.fn(),
  getDexWallets: vi.fn(),
  updateMMBotConfig: vi.fn(),
  removeMMBotConfig: vi.fn(),
  startMMBot: vi.fn(),
}));

const HOST = 'dex.example:7232';
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
const saved: MMBotConfig = {
  host: HOST,
  baseID: 42,
  quoteID: 0,
  basicMarketMakingConfig: {
    gapStrategy: 'percent-plus',
    driftTolerance: 0.002,
    buyPlacements: [{ lots: 2, gapFactor: 0.01 }],
    sellPlacements: [{ lots: 2, gapFactor: 0.01 }],
  },
};
const bot = (running: boolean): MMBotStatus => ({
  config: saved,
  running,
  runStats: null,
  latestEpoch: null,
  cexProblems: null,
});
const onDone = vi.fn();

const open = (bots: MMBotStatus[] = []) =>
  render(
    <DexMMWizard
      host={HOST}
      markets={[dcrBtc]}
      cexes={{}}
      bots={bots}
      refresh={() => {}}
      onCexSaved={() => {}}
      onDone={onDone}
    />,
  );

const pickMarket = () => fireEvent.click(screen.getByRole('button', { name: /DCR\/BTC/ }));
const actions = () => within(screen.getByRole('group', { name: 'Bot actions' }));
const profit = () => (screen.getByLabelText('Profit threshold') as HTMLInputElement).value;

beforeEach(() => {
  vi.mocked(api.getDexAssetCatalog).mockResolvedValue([]);
  vi.mocked(api.getMMMarketReport).mockResolvedValue({ price: 0, oracles: [], baseFiatRate: 0, quoteFiatRate: 0 });
  vi.mocked(api.getMMAvailableBalances).mockRejectedValue(new Error('no config'));
  vi.mocked(api.getDexWallets).mockResolvedValue([]);
  vi.mocked(api.updateMMBotConfig).mockResolvedValue();
  vi.mocked(api.removeMMBotConfig).mockResolvedValue();
});
afterEach(cleanup);

describe('DexMMWizard', () => {
  it('cancels from the market and bot type steps', () => {
    open();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onDone).toHaveBeenCalledTimes(1);
    pickMarket();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onDone).toHaveBeenCalledTimes(2);
  });

  it('opens the saved bot of a market instead of starting a new one', () => {
    open([bot(false)]);
    pickMarket();
    expect(actions().getByRole('button', { name: 'Save changes' })).toBeTruthy();
    expect(actions().queryByRole('button', { name: 'Create & start' })).toBeNull();
    expect(profit()).toBe('1');
  });

  it('resets to the saved settings', () => {
    open([bot(false)]);
    pickMarket();
    fireEvent.change(screen.getByLabelText('Profit threshold'), { target: { value: '5' } });
    fireEvent.click(actions().getByRole('button', { name: 'Reset settings' }));
    expect(profit()).toBe('1');
  });

  it('deletes a bot only once confirmed', async () => {
    open([bot(false)]);
    pickMarket();
    fireEvent.click(actions().getByRole('button', { name: 'Delete bot' }));
    const dialog = within(screen.getByRole('dialog', { name: 'Delete Bot' }));
    expect(
      dialog.getByText(`Are you sure you want to delete this bot for the DCR-BTC market on ${HOST}?`),
    ).toBeTruthy();
    fireEvent.click(dialog.getByRole('button', { name: 'Cancel' }));
    expect(api.removeMMBotConfig).not.toHaveBeenCalled();
    fireEvent.click(actions().getByRole('button', { name: 'Delete bot' }));
    fireEvent.click(within(screen.getByRole('dialog', { name: 'Delete Bot' })).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(api.removeMMBotConfig).toHaveBeenCalledWith(HOST, 42, 0));
    expect(onDone).toHaveBeenCalled();
  });

  it('offers neither reset nor delete for a running bot', () => {
    open([bot(true)]);
    pickMarket();
    expect(actions().getByRole('button', { name: 'Apply to running bot' })).toBeTruthy();
    expect(actions().queryByRole('button', { name: 'Reset settings' })).toBeNull();
    expect(actions().queryByRole('button', { name: 'Delete bot' })).toBeNull();
  });

  it('shows why a start failed in the dialog that started it', async () => {
    vi.mocked(api.startMMBot).mockRejectedValue(new Error('insufficient DEX balance for dcr: 1 < 2'));
    open();
    pickMarket();
    fireEvent.click(screen.getByRole('button', { name: /Continue/ }));
    fireEvent.click(actions().getByRole('button', { name: 'Create & start' }));
    await screen.findByText(/Fund DCR\/BTC bot/);
    fireEvent.change(screen.getAllByRole('textbox')[0], { target: { value: '1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Start bot' }));
    expect(await screen.findByText('insufficient DEX balance for dcr: 1 < 2')).toBeTruthy();
    expect(onDone).not.toHaveBeenCalled();
  });
});
