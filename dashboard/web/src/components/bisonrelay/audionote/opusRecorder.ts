// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Records a voice note as raw Opus packets with the settings bruig's recorder
// uses (48 kHz mono, 20 ms frames, 40 kbps, VOIP). The capture chain mirrors
// the realtime call pipeline, whose capture is private to it; the one
// deliberate difference is no in-band FEC, which bruig does not enable and a
// stored note has no packet loss to spend it on.

// Served from our own origin: the document CSP allows scripts from 'self' only.
const MIC_TAP_WORKLET_URL = '/rtdt-mic-tap.worklet.js';

export const SAMPLE_RATE = 48000;
export const FRAME_SAMPLES = 960;
export const FRAME_US = 20000;
export const MAX_NOTE_SECONDS = 60;
export const MAX_NOTE_PACKETS = (MAX_NOTE_SECONDS * 1000) / 20;

export interface RecordedNote {
  packets: Uint8Array[];
  seconds: number;
  bytes: number;
}

export interface RecorderCallbacks {
  onPackets?: (count: number) => void;
  onLimit?: () => void;
  onError?: (message: string) => void;
}

export class OpusNoteRecorder {
  packets: Uint8Array[] = [];
  private stream: MediaStream | null = null;
  private ctx: AudioContext | null = null;
  private source: MediaStreamAudioSourceNode | null = null;
  private worklet: AudioWorkletNode | null = null;
  private encoder: AudioEncoder | null = null;
  private accum = new Float32Array(FRAME_SAMPLES);
  private filled = 0;
  private tsUs = 0;
  private stopped = false;

  constructor(private cb: RecorderCallbacks = {}) {}

  async start(): Promise<void> {
    try {
      this.stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          sampleRate: SAMPLE_RATE,
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
        video: false,
      });
      if (this.stopped) throw new Error('stopped');
      this.ctx = new AudioContext({ sampleRate: SAMPLE_RATE });
      if (this.ctx.state === 'suspended') {
        try { await this.ctx.resume(); } catch { /* ignore */ }
      }
      await this.ctx.audioWorklet.addModule(MIC_TAP_WORKLET_URL);
      if (this.stopped) throw new Error('stopped');
      this.encoder = this.createEncoder();
      this.source = this.ctx.createMediaStreamSource(this.stream);
      this.worklet = new AudioWorkletNode(this.ctx, 'rtdt-mic-tap', {
        numberOfInputs: 1,
        numberOfOutputs: 0,
        channelCount: 1,
        processorOptions: { epoch: 0, muted: false },
      });
      this.worklet.port.onmessage = (e) => this.encodeSamples(e.data.samples);
      this.source.connect(this.worklet);
    } catch (err) {
      this.dispose();
      throw err;
    }
  }

  // stop ends capture and waits for the encoder to hand back its last frame.
  async stop(): Promise<RecordedNote> {
    this.stopped = true;
    if (this.worklet) this.worklet.port.onmessage = null;
    try { this.source?.disconnect(); } catch { /* ignore */ }
    this.stream?.getTracks().forEach((track) => track.stop());
    const encoder = this.encoder;
    if (encoder && encoder.state === 'configured') {
      try { await encoder.flush(); } catch { /* ignore */ }
    }
    this.dispose();
    return this.note();
  }

  note(): RecordedNote {
    const packets = this.packets.slice(0, MAX_NOTE_PACKETS);
    return {
      packets,
      seconds: (packets.length * FRAME_US) / 1e6,
      bytes: packets.reduce((n, p) => n + p.length, 0),
    };
  }

  dispose(): void {
    this.stopped = true;
    const encoder = this.encoder;
    this.encoder = null;
    try { encoder?.close(); } catch { /* ignore */ }
    if (this.worklet) {
      this.worklet.port.onmessage = null;
      try { this.worklet.port.close(); } catch { /* ignore */ }
      try { this.worklet.disconnect(); } catch { /* ignore */ }
      this.worklet = null;
    }
    try { this.source?.disconnect(); } catch { /* ignore */ }
    this.source = null;
    this.stream?.getTracks().forEach((track) => track.stop());
    this.stream = null;
    const ctx = this.ctx;
    this.ctx = null;
    if (ctx) void ctx.close().catch(() => undefined);
  }

  private createEncoder(): AudioEncoder {
    const Enc = (window as any).AudioEncoder as typeof AudioEncoder;
    const encoder = new Enc({
      output: (chunk) => this.collect(chunk),
      error: (err) => this.cb.onError?.(`AudioEncoder error: ${err?.message ?? err}`),
    });
    encoder.configure({
      codec: 'opus', sampleRate: SAMPLE_RATE, numberOfChannels: 1, bitrate: 40000,
      opus: { application: 'voip', frameDuration: FRAME_US },
    } as AudioEncoderConfig);
    return encoder;
  }

  private encodeSamples(data: Float32Array): void {
    const encoder = this.encoder;
    if (!encoder || this.stopped) return;
    const AudioDataCtor = (window as any).AudioData as typeof AudioData;
    let offset = 0;
    while (offset < data.length) {
      const take = Math.min(this.accum.length - this.filled, data.length - offset);
      this.accum.set(data.subarray(offset, offset + take), this.filled);
      this.filled += take;
      offset += take;
      if (this.filled === this.accum.length) {
        const ad = new AudioDataCtor({
          format: 'f32-planar', sampleRate: SAMPLE_RATE,
          numberOfFrames: this.accum.length, numberOfChannels: 1,
          timestamp: this.tsUs, data: this.accum.slice(),
        });
        this.tsUs += FRAME_US;
        this.filled = 0;
        try { encoder.encode(ad); } finally { ad.close(); }
      }
    }
  }

  private collect(chunk: EncodedAudioChunk): void {
    if (this.packets.length >= MAX_NOTE_PACKETS) return;
    const bytes = new Uint8Array(chunk.byteLength);
    chunk.copyTo(bytes);
    this.packets.push(bytes);
    this.cb.onPackets?.(this.packets.length);
    if (this.packets.length === MAX_NOTE_PACKETS) this.cb.onLimit?.();
  }
}

// decodeNote turns the recorded packets into one AudioBuffer for preview,
// with the decoder configuration the call pipeline already uses for raw
// Opus (no container description needed).
export const decodeNote = async (packets: Uint8Array[]): Promise<AudioBuffer> => {
  const Dec = (window as any).AudioDecoder as typeof AudioDecoder;
  const Chunk = (window as any).EncodedAudioChunk as typeof EncodedAudioChunk;
  const frames: Float32Array[] = [];
  const decoder = new Dec({
    output: (ad) => {
      const buf = new Float32Array(ad.numberOfFrames);
      ad.copyTo(buf, { planeIndex: 0, format: 'f32-planar' });
      frames.push(buf);
      ad.close();
    },
    error: () => undefined,
  });
  try {
    decoder.configure({ codec: 'opus', sampleRate: SAMPLE_RATE, numberOfChannels: 1 } as AudioDecoderConfig);
    packets.forEach((p, i) => {
      decoder.decode(new Chunk({ type: 'key', timestamp: i * FRAME_US, data: p }));
    });
    await decoder.flush();
  } finally {
    try { decoder.close(); } catch { /* ignore */ }
  }
  const total = frames.reduce((n, f) => n + f.length, 0);
  const ctx = new OfflineAudioContext(1, Math.max(total, 1), SAMPLE_RATE);
  const out = ctx.createBuffer(1, Math.max(total, 1), SAMPLE_RATE);
  const channel = out.getChannelData(0);
  let off = 0;
  for (const f of frames) {
    channel.set(f, off);
    off += f.length;
  }
  return out;
};
