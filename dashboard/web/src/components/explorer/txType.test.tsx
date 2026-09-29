import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { TransactionSummary } from '../../services/explorerApi';
import { TxGroupSections, groupTxsByType, txTypeMeta } from './txType';

afterEach(cleanup);

const tx = (txid: string, type: string): TransactionSummary =>
  ({ txid, type, size: 250, totalValue: 1.5 }) as TransactionSummary;

describe('txTypeMeta', () => {
  it('names a CoinJoin as one and falls back to regular for anything else', () => {
    expect(txTypeMeta('coinjoin').name).toBe('CoinJoin');
    expect(txTypeMeta('coinjoin').color).toBe('text-purple-500');
    expect(txTypeMeta('ticket').name).toBe('Ticket Purchase (SSTx)');
    expect(txTypeMeta('something-new').name).toBe('Regular Transaction');
  });
});

describe('groupTxsByType', () => {
  it('puts treasury spends and additions together', () => {
    const g = groupTxsByType([tx('a', 'tspend'), tx('b', 'treasurybase'), tx('c', 'coinjoin'), tx('d', 'regular')]);
    expect(g.treasury.map((t) => t.txid)).toEqual(['a', 'b']);
    expect(g.coinjoin.map((t) => t.txid)).toEqual(['c']);
    expect(g.regular.map((t) => t.txid)).toEqual(['d']);
    expect(g.votes).toEqual([]);
  });
});

describe('TxGroupSections', () => {
  const groups = groupTxsByType([tx('aa11', 'tspend'), tx('bb22', 'vote'), tx('cc33', 'regular')]);

  it('lists sections in the given order and skips empty ones', () => {
    render(
      <MemoryRouter>
        <TxGroupSections groups={groups} order={['regular', 'votes', 'treasury', 'tickets']} />
      </MemoryRouter>,
    );
    const headings = screen.getAllByRole('heading').map((h) => h.textContent);
    expect(headings).toEqual(['Regular (1)', 'Votes (1)', 'Treasury (1)']);
    expect(screen.getByText('Treasury Spend')).toBeTruthy();
  });

  it('opens a transaction when its row is clicked', () => {
    render(
      <MemoryRouter initialEntries={['/explorer']}>
        <Routes>
          <Route path="/explorer" element={<TxGroupSections groups={groups} order={['votes']} />} />
          <Route path="/explorer/tx/:txid" element={<p>tx page</p>} />
        </Routes>
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByText('bb22'));
    expect(screen.getByText('tx page')).toBeTruthy();
  });
});
