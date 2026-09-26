import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { RecoveryDeposit } from '../../services/gamingApi';
import { GamingRecovery, recoveryGroups } from './GamingRecovery';

const served = new Blob(['{"format":1,"ledger":{"a": 1}}'], { type: 'application/json' });
vi.mock('../../hooks/useVisiblePoll', async () => {
  const { useEffect } = await import('react');
  return { useVisiblePoll: (fn: () => void) => useEffect(() => fn(), []) };
});
let rows: RecoveryDeposit[] = [];
const archived: [string, boolean][] = [];
const restored: File[] = [];
vi.mock('../../services/gamingApi', () => ({
  getGamingRecovery: async () => rows,
  archiveRecovery: async (id: string, on: boolean) => { archived.push([id, on]); },
  getGamingLedgerBackup: async () => served,
  restoreGamingLedger: async (f: File) => { restored.push(f); return { restored: true, unownedKeys: 2 }; },
  closeRecoveryTable: vi.fn(),
  quoteRecovery: vi.fn(),
  confirmRecovery: vi.fn(),
}));

afterEach(() => { cleanup(); vi.restoreAllMocks(); rows = []; archived.length = 0; restored.length = 0; });

describe('GamingRecovery', () => {
  it('saves the ledger backup exactly as served', async () => {
    const saved: Blob[] = [];
    URL.createObjectURL = vi.fn((b: Blob) => { saved.push(b); return 'blob:backup'; });
    URL.revokeObjectURL = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.download).toMatch(/^gaming-ledger-\d+\.json$/);
      expect(this.href).toBe('blob:backup');
    });
    render(<GamingRecovery />);
    fireEvent.click(screen.getByText('Download backup'));
    await waitFor(() => expect(click).toHaveBeenCalledTimes(1));
    expect(saved).toEqual([served]);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:backup');
  });
});

const dep = (over: Partial<RecoveryDeposit>): RecoveryDeposit => ({
  id: 'd', game: 'stakewars', table: 't1', kind: 'seatbond', atoms: 1000000, outpoint: 'op', lockBlocks: 2016,
  confirmations: 10, remainingBlocks: 0, state: 'spent', canRecover: false, closed: true, archived: false, ...over,
});

describe('recovery order', () => {
  it('puts actionable deposits first, locked ones by time left, finished last', () => {
    const groups = recoveryGroups([
      dep({ id: 'done', table: 'tA', state: 'spent' }),
      dep({ id: 'late', table: 'tB', state: 'locked', remainingBlocks: 900 }),
      dep({ id: 'soon', table: 'tC', state: 'locked', remainingBlocks: 10 }),
      dep({ id: 'ready', table: 'tD', state: 'recoverable', canRecover: true }),
      dep({ id: 'paying', table: 'tE', state: 'awaiting_payment' }),
      dep({ id: 'flying', table: 'tF', state: 'recovery_pending' }),
    ]);
    expect(groups.map((g) => g.items[0].id)).toEqual(['ready', 'soon', 'late', 'paying', 'flying', 'done']);
  });

  it("keeps a table's deposits together, led by its most urgent one", () => {
    const groups = recoveryGroups([
      dep({ id: 'bond', table: 't1', state: 'spent' }),
      dep({ id: 'other', table: 't2', state: 'locked', remainingBlocks: 5 }),
      dep({ id: 'stake', table: 't1', kind: 'stake', state: 'close_table' }),
    ]);
    expect(groups.map((g) => [g.table, g.items.map((i) => i.id)])).toEqual([['t1', ['stake', 'bond']], ['t2', ['other']]]);
  });
});

describe('archiving', () => {
  it('offers the trash only on finished deposits and archives through the ledger', async () => {
    rows = [dep({ id: 'done', state: 'spent' }), dep({ id: 'ready', table: 't2', state: 'recoverable', canRecover: true })];
    render(<GamingRecovery />);
    await screen.findByText('Ready to recover');
    const trash = screen.getAllByLabelText('Archive');
    expect(trash).toHaveLength(1);
    fireEvent.click(trash[0]);
    await waitFor(() => expect(archived).toEqual([['done', true]]));
  });

  it('hides archived deposits until asked, then restores them', async () => {
    rows = [dep({ id: 'gone', table: 'old-table', archived: true }), dep({ id: 'here', table: 'live-table', state: 'locked', remainingBlocks: 3 })];
    render(<GamingRecovery />);
    await screen.findByText('stakewars · table live-table');
    expect(screen.queryByText('stakewars · table old-table')).toBeNull();
    fireEvent.click(screen.getByText('Show archived (1)'));
    expect(await screen.findByText('stakewars · table old-table')).toBeTruthy();
    fireEvent.click(screen.getByText('Restore'));
    await waitFor(() => expect(archived).toEqual([['gone', false]]));
  });
});

describe('restoring the ledger', () => {
  it('restores the chosen file after confirmation and reports keys the wallet lacks', async () => {
    render(<GamingRecovery />);
    await screen.findByText('No deposits have been registered in the bridge ledger.');
    const file = new File(['{"format":1}'], 'gaming-ledger-1790422024.json', { type: 'application/json' });
    fireEvent.change(screen.getByLabelText('Backup file'), { target: { files: [file] } });
    expect(restored).toEqual([]);
    fireEvent.click(screen.getByText('Restore ledger'));
    await waitFor(() => expect(restored).toEqual([file]));
    expect(await screen.findByText(/2 deposit keys are not known to this wallet yet/)).toBeTruthy();
  });

  it('offers no restore while the ledger holds deposits', async () => {
    rows = [dep({ id: 'here', state: 'locked', remainingBlocks: 3 })];
    render(<GamingRecovery />);
    await screen.findByText('Time locked');
    expect(screen.queryByText('Restore backup')).toBeNull();
  });
});
