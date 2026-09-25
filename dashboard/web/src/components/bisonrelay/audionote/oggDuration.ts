// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const SAMPLE_RATE = 48000;

// A voice note is decided by its bytes, not by the type= and filename= a
// peer wrote into the tag. The stream must open with a beginning-of-stream
// page whose first packet is an OpusHead (RFC 7845), continue with the
// OpusTags page, and carry at least one audio page; anything else is not
// played. The last page's granule position gives the duration exactly, where
// browsers may report Infinity for Ogg until it is fully buffered.

export interface OggOpusInfo {
  seconds: number;
  channels: number;
}

const ascii = (b: Uint8Array, off: number, s: string): boolean => {
  if (off + s.length > b.length) return false;
  for (let i = 0; i < s.length; i++) if (b[off + i] !== s.charCodeAt(i)) return false;
  return true;
};

export const parseOggOpus = (bytes: Uint8Array): OggOpusInfo | null => {
  let off = 0;
  let index = 0;
  let granule: number | null = null;
  let channels = 0;
  while (off + 27 <= bytes.length) {
    if (!ascii(bytes, off, 'OggS') || bytes[off + 4] !== 0) return null;
    const typ = bytes[off + 5];
    const view = new DataView(bytes.buffer, bytes.byteOffset + off);
    const lo = view.getUint32(6, true);
    const hi = view.getUint32(10, true);
    const segments = bytes[off + 26];
    let size = 0;
    for (let i = 0; i < segments; i++) size += bytes[off + 27 + i];
    const start = off + 27 + segments;
    const next = start + size;
    if (next > bytes.length) return null;
    if (index === 0) {
      if (!(typ & 0x02) || lo !== 0 || hi !== 0) return null;
      if (size < 19 || !ascii(bytes, start, 'OpusHead') || bytes[start + 8] !== 1 || bytes[start + 9] === 0) return null;
      channels = bytes[start + 9];
    } else if (index === 1) {
      if (!ascii(bytes, start, 'OpusTags')) return null;
    } else if (lo !== 0 || hi !== 0) {
      granule = hi * 0x1_0000_0000 + lo;
    }
    index++;
    off = next;
  }
  if (index < 3 || granule === null || off !== bytes.length) return null;
  return { seconds: granule / SAMPLE_RATE, channels };
};
