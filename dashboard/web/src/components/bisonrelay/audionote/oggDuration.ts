// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const SAMPLE_RATE = 48000;

// oggDurationSeconds reads the duration of an Ogg/Opus note from its last
// page's granule position. Browsers may report Infinity for an Ogg stream
// until it is fully buffered, and the granule is exact.
export const oggDurationSeconds = (bytes: Uint8Array): number | null => {
  let off = 0;
  let granule: number | null = null;
  while (off + 27 <= bytes.length) {
    if (bytes[off] !== 0x4f || bytes[off + 1] !== 0x67 || bytes[off + 2] !== 0x67 || bytes[off + 3] !== 0x53) {
      return null;
    }
    const view = new DataView(bytes.buffer, bytes.byteOffset + off);
    const lo = view.getUint32(6, true);
    const hi = view.getUint32(10, true);
    const segments = bytes[off + 26];
    let payload = 0;
    for (let i = 0; i < segments; i++) payload += bytes[off + 27 + i];
    const next = off + 27 + segments + payload;
    if (next > bytes.length) return null;
    // Header pages carry granule 0; the last audio page carries the total.
    if (lo !== 0 || hi !== 0) granule = hi * 0x1_0000_0000 + lo;
    off = next;
  }
  return granule === null ? null : granule / SAMPLE_RATE;
};
