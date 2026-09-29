import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { createPolledStore } from './polledStore';

describe('createPolledStore', () => {
  it('publishes loads, keeps the same snapshot when nothing changed, and applies the error policy', async () => {
    let n = 0;
    const load = vi.fn(async () => {
      n++;
      if (n === 3) throw new Error('down');
      return { count: n >= 2 ? 2 : 1, loading: false };
    });
    const store = createPolledStore({
      load, ms: 60000, initial: { count: 0, loading: true },
      onError: (_err, prev) => ({ ...prev, loading: false }),
    });
    const { result } = renderHook(() => store.use());
    await waitFor(() => expect(result.current).toEqual({ count: 1, loading: false }));
    await act(async () => store.refresh());
    await waitFor(() => expect(result.current.count).toBe(2));
    const before = result.current;
    await act(async () => store.refresh());
    await waitFor(() => expect(load).toHaveBeenCalledTimes(3));
    expect(result.current).toBe(before);
  });
});
