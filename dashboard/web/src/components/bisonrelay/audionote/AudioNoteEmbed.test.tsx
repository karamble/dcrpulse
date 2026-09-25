import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EmbedRenderer } from '../embedRender';
import { parseEmbeds } from '../embedParser';
import { bytesToBase64 } from './packetFraming';
import { formatClock } from './AudioNoteEmbed';
import { oggOpusFile } from './oggFixtures';

const NOTE = 'embeds/0123456789abcdef/20260925_074458.oga';
const inlineTag = (data: string) =>
  `--embed[alt=Audio note,type=audio/ogg,filename=2026-09-24-20_33_52-audionote.opus,data=${data}]--`;
const loggedTag = `--embed[alt=Audio note,type=audio/ogg,filename=2026-09-25-07_44_54-audionote.opus,localfilename=${NOTE}]--`;
const segment = (tag: string) => parseEmbeds(tag)[0] as any;
const serve = (bytes: Uint8Array) => {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    headers: { get: () => String(bytes.length) },
    arrayBuffer: async () => bytes.buffer,
  }));
  (globalThis as any).fetch = fetchMock;
  return fetchMock;
};

beforeEach(() => {
  (URL as any).createObjectURL = vi.fn(() => 'blob:note');
  (URL as any).revokeObjectURL = vi.fn();
});
afterEach(cleanup);

describe('AudioNoteEmbed', () => {
  it('renders a bruig voice note as a player with its duration and a save link', () => {
    render(<EmbedRenderer embed={segment(inlineTag(bytesToBase64(oggOpusFile([2_880_000]))))} />);
    expect(screen.getByRole('button', { name: 'Play voice note' })).toBeTruthy();
    expect(screen.getByText(/0:00 \/ 1:00/)).toBeTruthy();
    const save = screen.getByRole('link', { name: 'Save voice note' }) as HTMLAnchorElement;
    expect(save.getAttribute('download')).toBe('2026-09-24-20_33_52-audionote.opus');
    expect(URL.createObjectURL).toHaveBeenCalledOnce();
  });

  it('fetches a received note that Bison Relay logged to disk', async () => {
    const fetchMock = serve(oggOpusFile([2_880_000]));
    render(<EmbedRenderer embed={segment(loggedTag)} />);
    expect(await screen.findByText(/0:00 \/ 1:00/)).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/br/embeds/0123456789abcdef/20260925_074458.oga',
      expect.objectContaining({ credentials: 'same-origin' }),
    );
    expect(screen.getByRole('button', { name: 'Play voice note' })).toBeTruthy();
    expect(screen.queryByText('2026-09-25-07_44_54-audionote.opus')).toBeNull();
  });

  it('does not play bytes that only claim to be a voice note', async () => {
    // A tag says audio/ogg, the bytes are something else: the plain chip, not a player.
    const notOgg = new Uint8Array([0x49, 0x44, 0x33, 4, 0, 0, 0, 0, 0, 0, 1, 2, 3]);
    render(<EmbedRenderer embed={segment(inlineTag(bytesToBase64(notOgg)))} />);
    expect(screen.queryByRole('button', { name: /voice note/i })).toBeNull();
    expect(screen.getByText('2026-09-24-20_33_52-audionote.opus')).toBeTruthy();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
    cleanup();

    // The same for a logged file, once it has been fetched and inspected.
    serve(notOgg);
    render(<EmbedRenderer embed={segment(loggedTag)} />);
    expect(await screen.findByText('2026-09-25-07_44_54-audionote.opus')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /voice note/i })).toBeNull();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });

  it('shows the chip for inline data that is not base64 instead of throwing', () => {
    render(<EmbedRenderer embed={segment(inlineTag('%%%'))} />);
    expect(screen.queryByRole('button', { name: /voice note/i })).toBeNull();
    expect(screen.getByText('2026-09-24-20_33_52-audionote.opus')).toBeTruthy();
  });

  it('leaves other inline files on the download chip', () => {
    render(<EmbedRenderer embed={segment('--embed[name=notes.txt,type=text/plain,data=QUJD]--')} />);
    expect(screen.queryByRole('button', { name: /voice note/i })).toBeNull();
    expect(screen.getByText('notes.txt')).toBeTruthy();
  });
});

describe('formatClock', () => {
  it('formats whole seconds as m:ss', () => {
    expect(formatClock(0)).toBe('0:00');
    expect(formatClock(59.9)).toBe('0:59');
    expect(formatClock(60)).toBe('1:00');
  });
});
