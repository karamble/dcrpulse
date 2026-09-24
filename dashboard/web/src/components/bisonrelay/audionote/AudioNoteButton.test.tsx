import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AudioNoteButton } from './AudioNoteButton';
import { base64ToBytes } from './packetFraming';

const recorded = { packets: [new Uint8Array([1, 2, 3]), new Uint8Array([4, 5])], seconds: 0.04, bytes: 5 };
const started = vi.fn();
const stopped = vi.fn();
const disposed = vi.fn();

vi.mock('../realtime/AudioPipeline', () => ({
  inSecureContext: () => true,
  supportsWebCodecsAudio: () => true,
}));
vi.mock('./opusRecorder', async (original) => ({
  ...await original<typeof import('./opusRecorder')>(),
  OpusNoteRecorder: class {
    constructor(private cb: { onPackets?: (n: number) => void }) {}
    async start() { started(); this.cb.onPackets?.(2); }
    async stop() { stopped(); return recorded; }
    dispose() { disposed(); }
  },
  decodeNote: vi.fn(),
}));

beforeEach(() => { started.mockClear(); stopped.mockClear(); disposed.mockClear(); });
afterEach(cleanup);

const click = (name: RegExp | string) => fireEvent.click(screen.getByRole('button', { name }));

describe('AudioNoteButton', () => {
  it('records, previews, and only sends when asked', async () => {
    const onSend = vi.fn(async (_packetsB64: string) => true);
    render(<AudioNoteButton onSend={onSend} />);

    await act(async () => { click('Record a voice note'); });
    expect(started).toHaveBeenCalledOnce();
    expect(screen.getByText(/0:00 \/ 1:00/)).toBeTruthy();
    expect(onSend).not.toHaveBeenCalled();

    await act(async () => { click('Stop recording'); });
    expect(stopped).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', { name: /Send/ })).toBeTruthy();
    expect(onSend).not.toHaveBeenCalled();

    await act(async () => { click(/Send/); });
    expect(onSend).toHaveBeenCalledOnce();
    // The server receives exactly the recorded packets, length-prefixed.
    const blob = base64ToBytes(onSend.mock.calls[0][0]);
    expect(Array.from(blob)).toEqual([0, 3, 1, 2, 3, 0, 2, 4, 5]);
    expect(screen.queryByRole('button', { name: /Send/ })).toBeNull();
  });

  it('discards a take without sending it', async () => {
    const onSend = vi.fn(async () => true);
    render(<AudioNoteButton onSend={onSend} />);
    await act(async () => { click('Record a voice note'); });
    await act(async () => { click('Stop recording'); });
    click('Discard voice note');
    expect(onSend).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /Send/ })).toBeNull();
    expect(screen.getByRole('button', { name: 'Record a voice note' })).toBeTruthy();
  });

  it('keeps the take when the send fails', async () => {
    const onSend = vi.fn(async () => false);
    render(<AudioNoteButton onSend={onSend} />);
    await act(async () => { click('Record a voice note'); });
    await act(async () => { click('Stop recording'); });
    await act(async () => { click(/Send/); });
    expect(screen.getByRole('button', { name: /Send/ })).toBeTruthy();
  });
});
