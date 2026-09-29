// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as br from '../../services/bisonrelayApi';
import { useTipStatus } from './useTipStatus';

const listeners = new Set<(e: br.BisonrelayLiveEvent) => void>();
vi.mock('./BisonrelayLiveProvider', () => ({
  useBisonrelayLive: () => ({
    addListener: (fn: (e: br.BisonrelayLiveEvent) => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
  }),
}));
vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...(await original<typeof import('../../services/bisonrelayApi')>()),
  tipBisonrelayContact: vi.fn(),
}));

const UID = 'aa'.repeat(32);
const emit = (type: string, payload: Record<string, unknown>) =>
  act(() => listeners.forEach((fn) => fn({ type, payload } as br.BisonrelayLiveEvent)));

beforeEach(() => {
  listeners.clear();
  vi.mocked(br.tipBisonrelayContact).mockResolvedValue(undefined as never);
});
afterEach(() => vi.clearAllMocks());

describe('useTipStatus', () => {
  it('follows a tip from the request to the daemon line', () => {
    const settled = vi.fn();
    const { result } = renderHook(() => useTipStatus(UID, 'alice', settled));
    act(() => result.current.submit(0.5));
    expect(br.tipBisonrelayContact).toHaveBeenCalledWith(UID, 0.5);
    expect(result.current.status).toEqual({ state: 'requesting', line: 'Requesting invoice for 0.5 DCR to tip alice...' });

    emit('tip-invoice-generated', { uid: UID, nick: 'alice-br' });
    expect(result.current.status).toEqual({ state: 'paying', line: 'Invoice received, paying tip to alice...' });

    emit('tip-sent', { recipient: 'bb'.repeat(32), line: 'Tip attempt of 1 DCR completed successfully!' });
    expect(result.current.status?.state).toBe('paying');
    expect(settled).not.toHaveBeenCalled();

    emit('tip-failed', { recipient: UID, line: 'Tip attempt of 0.5 DCR failed due to no route. Given up on attempting to tip.' });
    expect(result.current.status).toEqual({
      state: 'failed',
      line: 'Tip attempt of 0.5 DCR failed due to no route. Given up on attempting to tip.',
    });
    expect(settled).toHaveBeenCalledTimes(1);
  });

  it('upgrades to paying only from requesting', () => {
    const { result } = renderHook(() => useTipStatus(UID, 'alice'));
    emit('tip-invoice-generated', { uid: UID, nick: 'alice' });
    expect(result.current.status).toBeNull();
    act(() => result.current.submit(1));
    emit('tip-sent', { recipient: UID, line: 'Tip attempt of 1 DCR completed successfully!' });
    emit('tip-invoice-generated', { uid: UID, nick: 'alice' });
    expect(result.current.status?.state).toBe('sent');
  });

  it("reports a refused request in bruig's wording", async () => {
    vi.mocked(br.tipBisonrelayContact).mockRejectedValue(new Error('no such user'));
    const { result } = renderHook(() => useTipStatus(UID, 'alice'));
    act(() => result.current.submit(2));
    await waitFor(() =>
      expect(result.current.status).toEqual({
        state: 'failed',
        line: 'Tip attempt of 2 DCR failed due to no such user. Given up on attempting to tip.',
      }),
    );
  });
});
