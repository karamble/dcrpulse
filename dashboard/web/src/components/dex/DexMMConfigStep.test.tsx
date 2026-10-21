import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DexMarket } from '../../services/dcrdexApi';
import { DexMMConfigStep } from './DexMMConfigStep';
import { defaultDraft, type BotType } from './dexMMConfig';

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

const open = (botType: BotType = 'basicmm', opts: { editing?: boolean; running?: boolean } = {}) =>
  render(
    <DexMMConfigStep
      host="dex.example:7232"
      market={dcrBtc}
      initial={defaultDraft(botType, botType === 'basicmm' ? undefined : 'Binance')}
      editing={!!opts.editing}
      running={opts.running}
      report={null}
      catalog={[]}
      onChangeMarket={() => {}}
      onChangeBotType={() => {}}
      onSave={vi.fn()}
      onSaveAndStart={vi.fn()}
      onCancel={() => {}}
    />,
  );

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
