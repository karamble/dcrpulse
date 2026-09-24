import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { GovernanceDashboard } from './GovernanceDashboard';
import { authFetch } from '../services/api';
import { triggerTSpendScan } from '../services/treasuryApi';

vi.mock('../services/api', () => ({ authFetch: vi.fn() }));
vi.mock('../hooks/useVisiblePoll', () => ({ useVisiblePoll: () => {} }));
vi.mock('../services/treasuryStorage', () => ({
  getScanStatus: () => null, getLastSyncHeight: () => 600000,
  syncWithSnapshot: async () => ({ success: true, synced: 0 }),
  saveTSpends: vi.fn(), saveScanStatus: vi.fn(), updateLastSyncHeight: vi.fn(),
}));
vi.mock('../components/governance/TreasuryValueCard', () => ({ TreasuryValueCard: () => null }));
vi.mock('../components/governance/TreasuryPaymentsCard', () => ({ TreasuryPaymentsCard: () => null }));
vi.mock('../components/governance/TreasuryStats', () => ({ TreasuryStats: () => null }));
vi.mock('../components/governance/ActiveTreasuryVotes', () => ({ ActiveTreasuryVotes: () => null }));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });
beforeEach(() => { vi.mocked(authFetch).mockReset(); });

describe('treasury scan admission errors', () => {
  it('shows the above-tip rejection in the existing dashboard alert', async () => {
    const reason = 'invalid treasury scan start height: requested 600001 exceeds current chain tip 600000';
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    const alert = vi.spyOn(window, 'alert').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(authFetch).mockImplementation(async (url) => {
      if (String(url).endsWith('/scan-history')) return new Response(`${reason}\n`, { status: 400 });
      return new Response(JSON.stringify(String(url).endsWith('/scan-progress') ? { isScanning: false, tspendFound: 0 } : []));
    });
    render(<GovernanceDashboard />);
    fireEvent.click(screen.getByRole('button', { name: 'Scan Historical TSpends' }));
    await waitFor(() => expect(alert).toHaveBeenCalledWith(reason));
    expect(authFetch).toHaveBeenCalledWith('/api/treasury/scan-history', expect.objectContaining({ body: '{"startHeight":600001}' }));
    expect(screen.queryByText('Scanning...')).toBeNull();
  });

  it('retains a useful fallback when the server returns no reason', async () => {
    vi.mocked(authFetch).mockResolvedValue(new Response('', { status: 500 }));
    await expect(triggerTSpendScan(600001)).rejects.toThrow('Failed to trigger TSpend scan');
  });

  it('preserves the successful response contract', async () => {
    const result = { success: true, message: 'Historical TSpend scan started from block 552448' };
    vi.mocked(authFetch).mockResolvedValue(new Response(JSON.stringify(result)));
    await expect(triggerTSpendScan()).resolves.toEqual(result);
    expect(authFetch).toHaveBeenCalledWith('/api/treasury/scan-history', expect.objectContaining({ body: '{"startHeight":552448}' }));
  });
});
