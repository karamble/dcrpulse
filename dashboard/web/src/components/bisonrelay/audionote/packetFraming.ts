// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// The browser hands the server raw Opus packets, one per 20 ms, and the server
// muxes them. On the wire each packet is preceded by its big-endian uint16
// length; services.SplitAudioNotePackets is the reader.

export const MAX_PACKET_BYTES = 1275;

export const framePackets = (packets: Uint8Array[]): Uint8Array => {
  let total = 0;
  for (const p of packets) {
    if (p.length === 0 || p.length > MAX_PACKET_BYTES) {
      throw new Error(`opus packet of ${p.length} bytes cannot be framed`);
    }
    total += 2 + p.length;
  }
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of packets) {
    out[off] = p.length >> 8;
    out[off + 1] = p.length & 0xff;
    out.set(p, off + 2);
    off += 2 + p.length;
  }
  return out;
};

export const bytesToBase64 = (bytes: Uint8Array): string => {
  let s = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    s += String.fromCharCode.apply(null, Array.from(bytes.subarray(i, i + chunk)));
  }
  return btoa(s);
};

export const base64ToBytes = (b64: string): Uint8Array<ArrayBuffer> => {
  const s = atob(b64);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
};
