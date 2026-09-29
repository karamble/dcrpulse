import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { BisonrelayPostBodySegment } from '../../services/bisonrelayApi';
import { toEmbedSegment } from './embedParser';
import { EmbedRenderer } from './embedRender';

vi.mock('./QuoteEmbedCard', () => ({
  QuoteEmbedCard: ({ resolved }: { resolved?: { available: boolean } | null }) => (
    <p>quote {resolved ? (resolved.available ? 'resolved' : 'unavailable') : 'self-resolving'}</p>
  ),
}));
vi.mock('./DownloadEmbed', () => ({ DownloadEmbed: ({ uid }: { uid: string }) => <p>download from {uid}</p> }));
vi.mock('./audionote/AudioNoteEmbed', () => ({ AudioNoteEmbed: () => <p>voice note</p> }));

afterEach(cleanup);

const QUOTE_UID = 'a'.repeat(64);
const QUOTE_PID = 'b'.repeat(64);
const PNG = 'iVBORw0KGgo=';

describe('toEmbedSegment', () => {
  it('maps a server segment onto the chat segment', () => {
    const seg = toEmbedSegment({
      kind: 'embed', name: 'a.png', mime: 'image/png', data_b64: PNG, size: 8, alt: 'pic',
      download: 'f'.repeat(64), cost: 5, filename: 'a.png', quote_from: QUOTE_UID, quote_post: QUOTE_PID,
    });
    expect(seg).toMatchObject({
      kind: 'embed', name: 'a.png', mime: 'image/png', dataB64: PNG, size: 8, alt: 'pic',
      download: 'f'.repeat(64), cost: 5, filename: 'a.png', quoteFrom: QUOTE_UID, quotePost: QUOTE_PID,
    });
  });
});

describe('EmbedRenderer for posts, comments and pages', () => {
  const server = (s: BisonrelayPostBodySegment) => toEmbedSegment(s);

  it('draws post images full width and chat images in the bounded box', () => {
    const img = server({ kind: 'embed', name: 'a.png', mime: 'image/png', data_b64: PNG });
    const { rerender } = render(<EmbedRenderer embed={img} wide />);
    expect(screen.getByRole('img').className).toContain('h-auto');
    rerender(<EmbedRenderer embed={img} />);
    expect(screen.getByRole('img').className).toContain('max-h-72');
  });

  it('shows a non-image file as a chip with its size', () => {
    render(<EmbedRenderer embed={server({ kind: 'embed', name: 'notes.txt', mime: 'text/plain', data_b64: 'aGVsbG8=' })} wide />);
    expect(screen.getByText('notes.txt')).toBeTruthy();
    expect(screen.getByText(/text\/plain · \d+ B/)).toBeTruthy();
  });

  it('plays a voice note', () => {
    render(<EmbedRenderer embed={server({ kind: 'embed', name: 'v.ogg', mime: 'audio/ogg', data_b64: 'T2dnUw==' })} wide />);
    expect(screen.getByText('voice note')).toBeTruthy();
  });

  it('hands a quote the server resolved to the card', () => {
    const q = server({ kind: 'embed', mime: 'quote', quote_from: QUOTE_UID, quote_post: QUOTE_PID });
    const { rerender } = render(<EmbedRenderer embed={q} quoteResolved={{ available: false } as never} />);
    expect(screen.getByText('quote unavailable')).toBeTruthy();
    rerender(<EmbedRenderer embed={q} />);
    expect(screen.getByText('quote self-resolving')).toBeTruthy();
  });

  it('fetches a shared file from the post or page host', () => {
    render(<EmbedRenderer embed={server({ kind: 'embed', download: 'f'.repeat(64), filename: 'x.zip' })} downloadUid="host" />);
    expect(screen.getByText('download from host')).toBeTruthy();
  });
});
