// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as br from '../../services/bisonrelayApi';
import * as ln from '../../services/lightningApi';
import { usePaidDownload } from './usePaidDownload';

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
  getBisonrelayManageDownloads: vi.fn(),
  startBisonrelayContentGet: vi.fn(),
}));
vi.mock('../../services/lightningApi', async (original) => ({
  ...(await original<typeof import('../../services/lightningApi')>()),
  failedLnPaymentHashes: vi.fn(),
  findNewFailedLnPayment: vi.fn(),
}));

const UID = 'aa'.repeat(32);
const FID = 'bb'.repeat(32);
const emit = (type: string, payload: Record<string, unknown>) =>
  act(() => listeners.forEach((fn) => fn({ type, payload } as br.BisonrelayLiveEvent)));
const item = (over: Partial<br.BisonrelayDownloadItem>) =>
  ({ uid: UID, fid: FID, total_chunks: 4, missing_chunks: 4, disk_path: '', ...over }) as br.BisonrelayDownloadItem;

beforeEach(() => {
  listeners.clear();
  vi.mocked(br.getBisonrelayManageDownloads).mockResolvedValue([]);
  vi.mocked(br.startBisonrelayContentGet).mockResolvedValue(undefined as never);
  vi.mocked(ln.failedLnPaymentHashes).mockResolvedValue(new Set(['old']));
  vi.mocked(ln.findNewFailedLnPayment).mockResolvedValue(null);
});
afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe('usePaidDownload', () => {
  it('asks before a paid file and pays at most the amount on screen', async () => {
    const order: string[] = [];
    vi.mocked(ln.failedLnPaymentHashes).mockImplementation(async () => {
      order.push('baseline');
      return new Set();
    });
    vi.mocked(br.startBisonrelayContentGet).mockImplementation(async () => {
      order.push('get');
      return undefined as never;
    });
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID }));
    act(() => {
      result.current.request(50_000_000);
    });
    expect(result.current.phase).toBe('confirm');
    expect(br.startBisonrelayContentGet).not.toHaveBeenCalled();
    await act(() => result.current.start(50_000_000));
    expect(br.startBisonrelayContentGet).toHaveBeenCalledWith(UID, FID, 50_000_000);
    expect(order).toEqual(['baseline', 'get']);
    expect(result.current.phase).toBe('downloading');
  });

  it('starts a free file at once with a zero cap', async () => {
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID }));
    await act(async () => {
      await result.current.request(0);
    });
    expect(br.startBisonrelayContentGet).toHaveBeenCalledWith(UID, FID, 0);
  });

  it('turns a higher real price into a second confirm, for this file only', async () => {
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID }));
    await act(() => result.current.start(10));
    emit('file-download-cost-rejected', { uid: UID, fid: 'cc'.repeat(32), cost_atoms: 99 });
    expect(result.current.phase).toBe('downloading');
    emit('file-download-cost-rejected', { uid: UID, fid: FID, cost_atoms: 70_000_000 });
    expect(result.current.phase).toBe('mismatch');
    expect(result.current.realCost).toBe(70_000_000);
  });

  it('finishes on the completed event', async () => {
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID }));
    await act(() => result.current.start(0));
    emit('file-download-completed', { uid: UID, fid: FID, disk_path: '/d/f' });
    expect(result.current.phase).toBe('done');
  });

  it('reports progress, then a new failed payment as the error', async () => {
    vi.useFakeTimers();
    vi.mocked(br.getBisonrelayManageDownloads).mockResolvedValue([item({ missing_chunks: 1 })]);
    vi.mocked(ln.findNewFailedLnPayment).mockResolvedValue({
      failureReason: 'FAILURE_REASON_NO_ROUTE',
    } as ln.LightningPayment);
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID }));
    await act(() => result.current.start(10));
    await act(() => vi.advanceTimersByTimeAsync(800));
    expect(ln.findNewFailedLnPayment).toHaveBeenCalledWith(new Set(['old']));
    expect(result.current.progress).toEqual({ done: 3, total: 4 });
    expect(result.current.phase).toBe('error');
    expect(result.current.err).toBe('Payment failed: no route found to pay the recipient');
  });

  it('starts a saved file as done', () => {
    const { result } = renderHook(() => usePaidDownload({ uid: UID, fid: FID, saved: true }));
    expect(result.current.phase).toBe('done');
  });

  it('finds a file already downloaded, and only a complete one', async () => {
    vi.mocked(br.getBisonrelayManageDownloads).mockResolvedValue([item({ missing_chunks: 0, disk_path: '/d/f' })]);
    const done = renderHook(() => usePaidDownload({ uid: UID, fid: FID, lookupSaved: true }));
    await waitFor(() => expect(done.result.current.phase).toBe('done'));

    vi.mocked(br.getBisonrelayManageDownloads).mockResolvedValue([
      item({ missing_chunks: 2 }),
      item({ uid: 'cc'.repeat(32), missing_chunks: 0, disk_path: '/d/other' }),
    ]);
    const partial = renderHook(() => usePaidDownload({ uid: UID, fid: FID, lookupSaved: true }));
    await act(async () => {});
    expect(partial.result.current.phase).toBe('idle');
  });
});
