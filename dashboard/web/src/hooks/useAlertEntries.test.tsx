import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AlertEntry } from '../services/api';
import { getAlerts, markAlertRead } from '../services/api';
import { useAlertEntries } from './useAlertEntries';

vi.mock('../services/api', () => ({
  getAlerts: vi.fn(),
  markAlertRead: vi.fn(),
  markAllAlertsRead: vi.fn(),
  getAlertsSummary: vi.fn(async () => ({})),
}));
vi.mock('../components/bisonrelay/BisonrelayLiveProvider', () => ({ useBisonrelayLive: () => ({ addListener: () => () => {} }) }));

const entry = (id: string, read = false) => ({ id, read, kind: 'event', active: false }) as unknown as AlertEntry;

beforeEach(() => {
  vi.mocked(getAlerts).mockReset().mockResolvedValue([entry('a'), entry('b'), entry('c')]);
  vi.mocked(markAlertRead).mockReset();
});

describe('useAlertEntries', () => {
  it('narrows the list with pick', async () => {
    const { result } = renderHook(() => useAlertEntries({ pick: (all) => all.slice(0, 2) }));
    await waitFor(() => expect(result.current.entries.map((e) => e.id)).toEqual(['a', 'b']));
    expect(result.current.loading).toBe(false);
  });

  it('marks a read at once and reloads when the server refuses it', async () => {
    vi.mocked(markAlertRead).mockRejectedValue(new Error('no'));
    const { result } = renderHook(() => useAlertEntries());
    await waitFor(() => expect(result.current.entries).toHaveLength(3));
    await act(async () => result.current.readOne('b'));
    expect(markAlertRead).toHaveBeenCalledWith('b');
    await waitFor(() => expect(getAlerts).toHaveBeenCalledTimes(2));
  });
});
