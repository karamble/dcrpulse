import { describe, expect, it } from 'vitest';
import { b64ToBytes, blobToB64, bytesToB64 } from './base64';

describe('base64', () => {
  it('round-trips bytes larger than one chunk', async () => {
    const bytes = new Uint8Array(0x8000 * 3 + 7).map((_, i) => (i * 31) & 0xff);
    const b64 = bytesToB64(bytes);
    expect(b64ToBytes(b64)).toEqual(bytes);
    // jsdom's Blob has no arrayBuffer(); every browser the dashboard targets does.
    const blob = { arrayBuffer: async () => bytes.buffer } as unknown as Blob;
    expect(await blobToB64(blob)).toBe(b64);
  });

  it('matches the browser encoder', () => {
    expect(bytesToB64(new TextEncoder().encode('dcrpulse'))).toBe(btoa('dcrpulse'));
  });

  it('refuses input that is not base64', () => {
    expect(() => b64ToBytes('not base64!')).toThrow();
  });
});
