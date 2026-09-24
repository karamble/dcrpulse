import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EmbedRenderer } from '../embedRender';
import { parseEmbeds } from '../embedParser';
import { bytesToBase64 } from './packetFraming';
import { formatClock } from './AudioNoteEmbed';

// One Ogg page carrying a granule of 60 s; the chip reads only the granule.
const oggWithGranule = (granule: number): Uint8Array => {
  const g = [];
  let rest = granule;
  for (let i = 0; i < 8; i++) {
    g.push(rest % 256);
    rest = Math.floor(rest / 256);
  }
  return new Uint8Array([0x4f, 0x67, 0x67, 0x53, 0, 4, ...g, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0xaa]);
};

const bruigTag = (data: string) =>
  `--embed[alt=Audio note,type=audio/ogg,filename=2026-09-24-20_33_52-audionote.opus,data=${data}]--`;

beforeEach(() => {
  (URL as any).createObjectURL = vi.fn(() => 'blob:note');
  (URL as any).revokeObjectURL = vi.fn();
});
afterEach(cleanup);

describe('AudioNoteEmbed', () => {
  it('renders a bruig voice note as a player with its duration and a save link', () => {
    const seg = parseEmbeds(bruigTag(bytesToBase64(oggWithGranule(2_880_000))))[0];
    expect(seg.kind).toBe('embed');
    render(<EmbedRenderer embed={seg as any} />);
    expect(screen.getByRole('button', { name: 'Play voice note' })).toBeTruthy();
    expect(screen.getByText(/0:00 \/ 1:00/)).toBeTruthy();
    const save = screen.getByRole('link', { name: 'Save voice note' }) as HTMLAnchorElement;
    expect(save.getAttribute('download')).toBe('2026-09-24-20_33_52-audionote.opus');
    expect(URL.createObjectURL).toHaveBeenCalledOnce();
  });

  it('leaves other inline files on the download chip', () => {
    const seg = parseEmbeds('--embed[name=notes.txt,type=text/plain,data=QUJD]--')[0];
    render(<EmbedRenderer embed={seg as any} />);
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
