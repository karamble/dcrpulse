import { StrictMode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BisonrelayRealtime } from './BisonrelayRealtime';
import type { PipelineOptions } from './realtime/AudioPipeline';
import type { BisonrelayLiveEvent, RTDTSession } from '../../services/bisonrelayApi';
import { getBisonrelayRTDTMessages, listRTDTSessions } from '../../services/bisonrelayApi';

const state = vi.hoisted(() => ({
  instances: [] as {
    opts: PipelineOptions;
    muted: boolean;
    stop: ReturnType<typeof vi.fn>;
    setMuted: ReturnType<typeof vi.fn>;
  }[],
  listeners: new Set<(e: BisonrelayLiveEvent) => void>(),
  addListener: vi.fn(),
}));
vi.mock('./realtime/AudioPipeline', () => ({
  inSecureContext: () => true,
  supportsWebCodecsAudio: () => true,
  RealtimeAudioPipeline: class {
    muted: boolean;
    constructor(readonly opts: PipelineOptions) {
      this.muted = opts.initialMuted ?? false;
      state.instances.push(this);
    }
    start = vi.fn(async () => { this.opts.callbacks?.onConnected?.(); });
    stop = vi.fn();
    setMuted = vi.fn((value: boolean) => { this.muted = value; });
    outboundCounters = () => ({ sent: 0, rateLimited: 0 });
    livePeerIDs = () => [];
    dropPeer = vi.fn();
  },
}));
vi.mock('./BisonrelayLiveProvider', () => ({ useBisonrelayLive: () => ({ addListener: state.addListener }) }));
vi.mock('./realtime/IncomingInviteBanner', () => ({ IncomingInviteBanner: () => null }));
vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...await original<typeof import('../../services/bisonrelayApi')>(),
  listRTDTSessions: vi.fn(),
  getBisonrelayRTDTMessages: vi.fn(),
}));
const A = 'a'.repeat(64), B = 'b'.repeat(64);
const session = (rv: string, live = true): RTDTSession => ({
  rv, live, description: rv, size: 2, owner: 'owner', is_instant: true,
  local_peer_id: 1, is_admin: false, hot_audio: false,
  members: [{ uid: 'owner', peer_id: 1, accepted: true, publisher: true }],
  publishers: [{ uid: 'owner', peer_id: 1, alias: 'Owner' }],
});
const current = () => state.instances[state.instances.length - 1];
const emit = async (rv: string) => {
  await act(async () => {
    for (const fn of [...state.listeners]) fn({ type: 'rtdt-session-updated', payload: { sessRV: rv } });
  });
};

beforeEach(() => {
  state.instances.length = 0; state.listeners.clear();
  state.addListener.mockImplementation((fn) => { state.listeners.add(fn); return () => state.listeners.delete(fn); });
  vi.mocked(listRTDTSessions).mockReset().mockResolvedValue([session(A), session(B)]);
  vi.mocked(getBisonrelayRTDTMessages).mockReset().mockResolvedValue([]);
  Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() });
  window.history.replaceState({}, '', `#realtime/room/${A}`);
});
afterEach(cleanup);

describe('call mute UI lifecycle', () => {
  it('preserves mute when a live call replaces its pipeline', async () => {
    render(<StrictMode><BisonrelayRealtime /></StrictMode>);
    const button = await screen.findByRole('button', { name: 'Mic on' });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(button);
    const old = current();
    expect(old.muted).toBe(true);
    expect(screen.getByRole('button', { name: 'Muted' })).toBeTruthy();
    vi.mocked(listRTDTSessions).mockResolvedValue([session(A, false)]);
    await emit(A);
    expect(old.stop).toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Muted' })).toBeNull(); // waiting screen hides call controls
    vi.mocked(listRTDTSessions).mockResolvedValue([session(A)]);
    await emit(A);
    await waitFor(() => expect(current()).not.toBe(old));
    expect(current().opts.initialMuted).toBe(true);
    expect(current().muted).toBe(true);
    expect((screen.getByRole('button', { name: 'Muted' }) as HTMLButtonElement).disabled).toBe(false);
    // Effect cleanup also revokes old callbacks, independent of the class guard.
    act(() => {
      old.opts.callbacks?.onDisconnected?.();
      old.opts.callbacks?.onError?.('stale error');
    });
    expect(screen.queryByText('stale error')).toBeNull();
    expect((screen.getByRole('button', { name: 'Muted' }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('uses synchronous intent for rapid toggles before React rerenders', async () => {
    render(<BisonrelayRealtime />);
    const button = await screen.findByRole('button', { name: 'Mic on' });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    act(() => { button.click(); button.click(); });
    expect(current().setMuted.mock.calls.map(([value]) => value)).toEqual([true, false]);
    expect(current().muted).toBe(false);
    expect(screen.getByRole('button', { name: 'Mic on' })).toBeTruthy();
  });

  it('resets mute for a different room and stops the old pipeline on unmount', async () => {
    const view = render(<StrictMode><BisonrelayRealtime /></StrictMode>);
    const button = await screen.findByRole('button', { name: 'Mic on' });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(button);
    const old = current();
    act(() => {
      window.history.replaceState({}, '', `#realtime/room/${B}`);
      window.dispatchEvent(new HashChangeEvent('hashchange'));
    });
    await waitFor(() => expect(current().opts.rv).toBe(B));
    expect(old.stop).toHaveBeenCalled();
    expect(current().opts.initialMuted).toBe(false);
    expect(screen.getByRole('button', { name: 'Mic on' })).toBeTruthy();
    view.unmount();
    for (const p of state.instances) expect(p.stop).toHaveBeenCalled();
  });
});
