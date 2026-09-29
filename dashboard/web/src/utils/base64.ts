// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Chunked because one String.fromCharCode.apply over megabytes of bytes
// overflows the call-stack argument limit.
const CHUNK = 0x8000;

export const bytesToB64 = (bytes: Uint8Array): string => {
  let bin = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    bin += String.fromCharCode.apply(null, bytes.subarray(i, i + CHUNK) as unknown as number[]);
  }
  return btoa(bin);
};

// b64ToBytes throws on input that is not base64.
export const b64ToBytes = (b64: string): Uint8Array<ArrayBuffer> => {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
};

export const blobToB64 = async (blob: Blob): Promise<string> =>
  bytesToB64(new Uint8Array(await blob.arrayBuffer()));
