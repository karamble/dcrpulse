import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { GamingRecovery } from './GamingRecovery';

const served = new Blob(['{"format":1,"ledger":{"a": 1}}'], { type: 'application/json' });
vi.mock('../../hooks/useVisiblePoll', async () => {
  const { useEffect } = await import('react');
  return { useVisiblePoll: (fn: () => void) => useEffect(() => fn(), []) };
});
vi.mock('../../services/gamingApi', () => ({
  getGamingRecovery: async () => [],
  getGamingLedgerBackup: async () => served,
  closeRecoveryTable: vi.fn(),
  quoteRecovery: vi.fn(),
  confirmRecovery: vi.fn(),
}));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

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
