import { describe, expect, it } from 'vitest';
import { parseOggOpus } from './oggDuration';
import { oggOpusFile, oggPage, opusHead, opusTags } from './oggFixtures';

describe('parseOggOpus', () => {
  it('accepts the stream bruig writes and reads its duration from the last granule', () => {
    expect(parseOggOpus(oggOpusFile([960, 1920, 2880]))).toEqual({ seconds: 0.06, channels: 2 });
  });

  it('handles the 60 second maximum without precision loss', () => {
    expect(parseOggOpus(oggOpusFile([2_880_000]))?.seconds).toBe(60);
  });

  it('rejects bytes that only claim to be a voice note', () => {
    // Not Ogg at all, however the tag was labelled.
    expect(parseOggOpus(new Uint8Array([0x49, 0x44, 0x33, 4, 0, 0, 0, 0, 0, 0]))).toBeNull();
    // Ogg pages whose first packet is not an OpusHead (a Vorbis stream would look like this).
    const vorbis = [1, ...Array.from('vorbis', (c) => c.charCodeAt(0)), 0, 0, 0, 0, 2, 0x80, 0xbb, 0, 0];
    expect(parseOggOpus(oggOpusFile([960], vorbis))).toBeNull();
    // A header that is right in every byte except its magic.
    const wrongMagic = [...Array.from('NotOpus!', (c) => c.charCodeAt(0)), ...opusHead().slice(8)];
    expect(parseOggOpus(oggOpusFile([960], wrongMagic))).toBeNull();
    // Unknown OpusHead version, or a header claiming zero channels.
    expect(parseOggOpus(oggOpusFile([960], opusHead(2, 2)))).toBeNull();
    expect(parseOggOpus(oggOpusFile([960], opusHead(0)))).toBeNull();
    // First page not flagged beginning-of-stream.
    const noBOS = new Uint8Array([
      ...oggPage({ typ: 0, granule: 0, seq: 0, payload: opusHead() }),
      ...oggPage({ typ: 0, granule: 0, seq: 1, payload: opusTags() }),
      ...oggPage({ typ: 4, granule: 960, seq: 2, payload: [1] }),
    ]);
    expect(parseOggOpus(noBOS)).toBeNull();
    // Header pages with no audio behind them.
    expect(parseOggOpus(oggOpusFile([]))).toBeNull();
  });

  it('rejects a stream that is cut short or has bytes after its last page', () => {
    const good = oggOpusFile([960]);
    expect(parseOggOpus(good.slice(0, good.length - 1))).toBeNull();
    expect(parseOggOpus(new Uint8Array([...good, 0]))).toBeNull();
  });
});
