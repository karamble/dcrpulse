import { describe, expect, it } from 'vitest';
import { base64ToBytes, bytesToBase64, framePackets } from './packetFraming';

describe('framePackets', () => {
  it('prefixes every packet with its big-endian length', () => {
    const out = framePackets([new Uint8Array([7, 8, 9]), new Uint8Array(300).fill(1)]);
    expect(Array.from(out.subarray(0, 5))).toEqual([0, 3, 7, 8, 9]);
    expect(Array.from(out.subarray(5, 7))).toEqual([1, 44]);
    expect(out.length).toBe(2 + 3 + 2 + 300);
  });

  it('refuses what the server would refuse', () => {
    expect(() => framePackets([new Uint8Array(0)])).toThrow();
    expect(() => framePackets([new Uint8Array(1276)])).toThrow();
    expect(() => framePackets([new Uint8Array(1275)])).not.toThrow();
  });
});

describe('base64', () => {
  it('round-trips binary that crosses the chunk boundary', () => {
    const bytes = new Uint8Array(70_000);
    for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 31) & 0xff;
    expect(base64ToBytes(bytesToBase64(bytes))).toEqual(bytes);
  });
});
