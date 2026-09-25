// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Builders for the tests: real Ogg page framing with the OpusHead and
// OpusTags bruig writes, so the parser is exercised on structure rather than
// on a magic string. The CRC is left zero because the reader never checks it.

const bytesOf = (s: string): number[] => Array.from(s, (c) => c.charCodeAt(0));
const le32 = (n: number): number[] => [n & 255, (n >>> 8) & 255, (n >>> 16) & 255, (n >>> 24) & 255];

export const oggPage = (o: { typ: number; granule: number; seq: number; payload: number[]; serial?: number }): number[] => {
  const g: number[] = [];
  let rest = o.granule;
  for (let i = 0; i < 8; i++) {
    g.push(rest % 256);
    rest = Math.floor(rest / 256);
  }
  const lacing: number[] = [];
  let n = o.payload.length;
  while (n >= 255) {
    lacing.push(255);
    n -= 255;
  }
  lacing.push(n);
  return [
    ...bytesOf('OggS'), 0, o.typ, ...g, ...le32(o.serial ?? 1), ...le32(o.seq), 0, 0, 0, 0,
    lacing.length, ...lacing, ...o.payload,
  ];
};

export const opusHead = (channels = 2, version = 1): number[] =>
  [...bytesOf('OpusHead'), version, channels, 0, 0, ...le32(48000), 0, 0, 0];

export const opusTags = (): number[] => [...bytesOf('OpusTags'), ...le32(9), ...bytesOf('skynetbot'), ...le32(0)];

// oggOpusFile lays out header pages then one audio page per granule, the last
// flagged end-of-stream, as bruig's writer does.
export const oggOpusFile = (granules: number[], head: number[] = opusHead()): Uint8Array<ArrayBuffer> => {
  const pages = [
    oggPage({ typ: 0x02, granule: 0, seq: 0, payload: head }),
    oggPage({ typ: 0, granule: 0, seq: 1, payload: opusTags() }),
    ...granules.map((granule, i) =>
      oggPage({ typ: i === granules.length - 1 ? 0x04 : 0, granule, seq: i + 2, payload: [0xaa, 0xbb] }),
    ),
  ];
  return new Uint8Array(pages.flat());
};
