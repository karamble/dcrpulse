// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// RealtimeAudioPipeline runs the browser side of the RTDT audio bridge.
//
// Phase 3 scope: outbound only (mic -> WebSocket -> brclientd ->
// SendSpeechPacket). Inbound playback (per-peer AudioDecoder + Web Audio
// scheduling + jitter buffer) lands in Phase 4. The class is structured
// so adding the inbound side is a localised change.
//
// Codec config mirrors bruig's gopus encoder exactly so other RTDT peers
// can decode our frames without renegotiation: 48 kHz mono 20 ms 40 kbps
// VOIP. The browser does the Opus encoding via WebCodecs AudioEncoder;
// brclientd is a pure byte-shuffler.
//
// Wire frame format (matches brclientd rtdt_audio_ws.go):
//   | ver (1) | dir (1) | peerID (4) BE | opus payload |
//   ver = 0x01
//   dir = 0x01 inbound, 0x02 outbound

export const FRAME_VERSION = 0x01;
export const FRAME_DIR_INBOUND = 0x01;
export const FRAME_DIR_OUTBOUND = 0x02;
export const FRAME_HEADER_LEN = 6;

export interface PipelineCallbacks {
  onError?: (msg: string) => void;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onReconnecting?: (attempt: number, nextDelayMs: number) => void;
  onInboundFrame?: (peerID: number, opus: Uint8Array, timestamp: number) => void;
}

export interface PipelineOptions {
  rv: string;
  callbacks?: PipelineCallbacks;
}

// supportsWebCodecsAudio returns whether the running browser has the
// AudioEncoder + AudioData primitives needed for our outbound path.
// Chrome/Edge stable >= 130, Firefox >= 130, Safari only partial as of
// 2026-05.
export const supportsWebCodecsAudio = (): boolean => {
  return (
    typeof window !== 'undefined' &&
    typeof (window as any).AudioEncoder !== 'undefined' &&
    typeof (window as any).AudioData !== 'undefined'
  );
};

interface OutboundState {
  stream: MediaStream;
  worklet: AudioWorkletNode;
  encoder: AudioEncoder;
  ts: number;          // frame-counted, +20 per encoded chunk
  muted: boolean;
  packetsSent: number;
  packetsRateLimited: number;
}

// PeerPlayback holds per-peer inbound state. Each remote RTDT peer gets its
// own decoder (Opus is stateful) and Web Audio gain node. The jitter
// buffer is per-peer too; one peer's network blips don't penalise the
// other peers.
interface PeerPlayback {
  decoder: AudioDecoder;
  gainNode: GainNode;
  // scheduledTime is the AudioContext time at which the NEXT decoded
  // frame should start playing. Advances by 20 ms per frame. Reset to
  // ctx.currentTime + bufferDepthSec on underrun (gap detection).
  scheduledTime: number;
  // Adaptive depth in seconds. Starts at 0.08 (4 frames), grows up to
  // 0.30 (15 frames) under network pressure, shrinks back when things
  // calm down. Mirrors bruig's internal/audio/streams.go:270-400.
  bufferDepthSec: number;
  // Recent stats fed into the adaptation loop.
  framesSinceShrink: number;
  framesReceived: number;
  lastFrameWallMs: number; // for the "speaking now" indicator
  // Encoder timestamp counter we feed AudioDecoder; the source frame
  // timestamp is opaque to us, but the decoder needs monotonic input.
  decodeTsUs: number;
}

export class RealtimeAudioPipeline {
  private rv: string;
  private cb: PipelineCallbacks;
  private ws: WebSocket | null = null;
  // Shared AudioContext used by both the outbound mic capture and the
  // inbound per-peer playback nodes. Created lazily on the first inbound
  // frame OR on outbound start (whichever comes first); both happen
  // within scope of the "Join" click so autoplay restrictions are
  // satisfied.
  private ctx: AudioContext | null = null;
  private out: OutboundState | null = null;
  private peers: Map<number, PeerPlayback> = new Map();
  // Rate limiter: hard cap on outbound packets per second so a runaway
  // pump cannot drain LN allowance. 50 packets/sec at 20 ms is the
  // nominal rate; we cap at 55 to allow for jitter / catch-up.
  private rateWindow: number[] = []; // monotonic timestamps in ms
  private static readonly RATE_LIMIT_PPS = 55;
  private static readonly RATE_WINDOW_MS = 1000;
  // Jitter buffer bounds (seconds).
  private static readonly BUFFER_MIN_SEC = 0.08;
  private static readonly BUFFER_MAX_SEC = 0.30;
  private static readonly BUFFER_GROW_STEP = 0.02;   // +20 ms on underrun
  private static readonly BUFFER_SHRINK_AFTER = 250; // frames (~5s) of calm
  private static readonly FRAME_DURATION_SEC = 0.02;
  // Reconnect backoff — two ladders. Pre-connect retries are short
  // because the most common cause is brclientd's audio handler 409ing
  // because the live UDP session isn't up yet (typically resolves in
  // 2-5s). Post-connect retries are longer because they imply network
  // trouble or brclientd restart.
  private static readonly PRECONNECT_DELAYS_MS = [500, 1000, 2000, 3000, 5000, 5000, 5000];
  private static readonly RECONNECT_DELAYS_MS = [1000, 2000, 5000, 10000, 30000];
  private stopped = false;
  private hadConnection = false;
  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(opts: PipelineOptions) {
    this.rv = opts.rv;
    this.cb = opts.callbacks ?? {};
  }

  // start opens the WebSocket and, on open, begins the outbound mic
  // pipeline. Throws if WebCodecs is missing or the user denies mic
  // permission.
  async start(): Promise<void> {
    if (!supportsWebCodecsAudio()) {
      throw new Error('WebCodecs AudioEncoder is required. Use Chrome 130+, Edge 130+, or Firefox 130+.');
    }
    this.stopped = false;
    this.openSocket();
  }

  // stop closes the WebSocket and tears down all audio resources. Once
  // called, reconnect is disabled even if a stale onclose fires later.
  stop(): void {
    this.stopped = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.teardownOutbound();
    this.teardownInbound();
    if (this.ctx) {
      try { this.ctx.close(); } catch { /* ignore */ }
      this.ctx = null;
    }
    if (this.ws) {
      try { this.ws.close(); } catch { /* ignore */ }
      this.ws = null;
    }
  }

  private openSocket(): void {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${window.location.host}/api/br/rtdt/sessions/${encodeURIComponent(this.rv)}/audio`;
    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';
    this.ws = ws;

    ws.onopen = () => {
      this.hadConnection = true;
      this.reconnectAttempt = 0;
      this.cb.onConnected?.();
      this.startOutbound().catch((err) => {
        this.cb.onError?.(err?.message ?? String(err));
      });
    };
    ws.onclose = (ev) => {
      this.cb.onDisconnected?.();
      this.teardownOutbound();
      this.teardownInbound();
      this.ws = null;
      if (this.stopped) return;
      // 1008 (policy violation) is brclientd's "already attached in
      // another tab" signal. Do not retry that.
      if (ev.code === 1008) {
        this.cb.onError?.('This call is already attached in another tab.');
        return;
      }
      this.scheduleReconnect();
    };
    ws.onerror = () => {
      this.cb.onError?.('WebSocket error');
    };
    ws.onmessage = (e) => this.handleInbound(e.data);
  }

  private scheduleReconnect(): void {
    if (this.stopped) return;
    const ladder = this.hadConnection
      ? RealtimeAudioPipeline.RECONNECT_DELAYS_MS
      : RealtimeAudioPipeline.PRECONNECT_DELAYS_MS;
    if (this.reconnectAttempt >= ladder.length) {
      const msg = this.hadConnection
        ? 'Connection lost. Click Leave and retry.'
        : 'Could not connect to RTDT audio. The live session may not be ready yet — click Leave and try again.';
      this.cb.onError?.(msg);
      return;
    }
    const delay = ladder[this.reconnectAttempt];
    this.reconnectAttempt++;
    this.cb.onReconnecting?.(this.reconnectAttempt, delay);
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (this.stopped) return;
      this.openSocket();
    }, delay);
  }

  // setPeerGain adjusts the per-peer volume in linear units (0 - 2).
  // Phase 5 wires this to the per-peer volume slider.
  setPeerGain(peerID: number, gain: number): void {
    const p = this.peers.get(peerID);
    if (!p) return;
    p.gainNode.gain.value = Math.max(0, Math.min(2, gain));
  }

  // hasRecentSpeech returns true if the peer sent audio within the
  // window (default 250 ms). Mirrors bruig's "packet arrived recently"
  // heuristic for the speaking-now indicator.
  hasRecentSpeech(peerID: number, windowMs = 250): boolean {
    const p = this.peers.get(peerID);
    if (!p) return false;
    return performance.now() - p.lastFrameWallMs < windowMs;
  }

  livePeerIDs(): number[] {
    return Array.from(this.peers.keys());
  }

  // peerStats exposes per-peer telemetry for the UI (jitter buffer
  // depth, frames received). Useful for the "buffered_count" idiom from
  // bruig's RTDTLivePeerModel.
  peerStats(peerID: number): { framesReceived: number; bufferDepthMs: number } | null {
    const p = this.peers.get(peerID);
    if (!p) return null;
    return {
      framesReceived: p.framesReceived,
      bufferDepthMs: Math.round(p.bufferDepthSec * 1000),
    };
  }

  // setMuted gates the mic without tearing down the encoder pipeline.
  // Muted = we still listen for inbound, but stop encoding outbound.
  setMuted(muted: boolean): void {
    if (this.out) {
      this.out.muted = muted;
    }
  }

  isMuted(): boolean {
    return this.out?.muted ?? true;
  }

  outboundCounters(): { sent: number; rateLimited: number } {
    return {
      sent: this.out?.packetsSent ?? 0,
      rateLimited: this.out?.packetsRateLimited ?? 0,
    };
  }

  private async ensureCtx(): Promise<AudioContext> {
    if (this.ctx) {
      if (this.ctx.state === 'suspended') {
        try { await this.ctx.resume(); } catch { /* ignore */ }
      }
      return this.ctx;
    }
    this.ctx = new AudioContext({ sampleRate: 48000 });
    if (this.ctx.state === 'suspended') {
      try { await this.ctx.resume(); } catch { /* ignore */ }
    }
    return this.ctx;
  }

  private async startOutbound(): Promise<void> {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        sampleRate: 48000,
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
      video: false,
    });

    const ctx = await this.ensureCtx();
    await ctx.audioWorklet.addModule(workletURL());
    const source = ctx.createMediaStreamSource(stream);
    const worklet = new AudioWorkletNode(ctx, 'rtdt-mic-tap', {
      numberOfInputs: 1,
      numberOfOutputs: 0,
      channelCount: 1,
    });
    source.connect(worklet);

    const Enc = (window as any).AudioEncoder as typeof AudioEncoder;
    const AudioDataCtor = (window as any).AudioData as typeof AudioData;
    const encoder = new Enc({
      output: (chunk) => this.sendChunk(chunk),
      error: (err) => this.cb.onError?.(`AudioEncoder error: ${err?.message ?? err}`),
    });
    // Bruig: gopus VOIP application, 48k mono, 20 ms frame, 40 kbps.
    encoder.configure({
      codec: 'opus',
      sampleRate: 48000,
      numberOfChannels: 1,
      bitrate: 40000,
      opus: {
        application: 'voip',
        frameDuration: 20000, // microseconds; 20 ms
        useinbandfec: true,
      },
    } as AudioEncoderConfig);

    this.out = {
      stream,
      worklet,
      encoder,
      ts: 0,
      muted: false,
      packetsSent: 0,
      packetsRateLimited: 0,
    };

    // Forward PCM frames from the worklet to the encoder. The worklet
    // posts {samples: Float32Array, timestamp: number} on each render
    // quantum (128 samples = ~2.67 ms at 48 kHz); we accumulate into
    // 20 ms chunks of 960 samples before feeding the encoder.
    const samplesPerFrame = 960;
    const accum = new Float32Array(samplesPerFrame);
    let filled = 0;
    let captureTsUs = 0;
    worklet.port.onmessage = (e) => {
      if (!this.out || this.out.muted) return;
      const data: Float32Array = e.data.samples;
      let offset = 0;
      while (offset < data.length) {
        const room = samplesPerFrame - filled;
        const take = Math.min(room, data.length - offset);
        accum.set(data.subarray(offset, offset + take), filled);
        filled += take;
        offset += take;
        if (filled === samplesPerFrame) {
          const ad = new AudioDataCtor({
            format: 'f32-planar',
            sampleRate: 48000,
            numberOfFrames: samplesPerFrame,
            numberOfChannels: 1,
            timestamp: captureTsUs,
            data: accum.slice(),
          });
          captureTsUs += 20000;
          try {
            this.out!.encoder.encode(ad);
          } finally {
            ad.close();
          }
          filled = 0;
        }
      }
    };
  }

  private sendChunk(chunk: EncodedAudioChunk): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || !this.out) return;

    // Hard rate limit (LN allowance safety).
    const now = performance.now();
    this.rateWindow = this.rateWindow.filter((t) => now - t < RealtimeAudioPipeline.RATE_WINDOW_MS);
    if (this.rateWindow.length >= RealtimeAudioPipeline.RATE_LIMIT_PPS) {
      this.out.packetsRateLimited++;
      return;
    }
    this.rateWindow.push(now);

    const opus = new Uint8Array(chunk.byteLength);
    chunk.copyTo(opus);
    const buf = new Uint8Array(FRAME_HEADER_LEN + opus.byteLength);
    buf[0] = FRAME_VERSION;
    buf[1] = FRAME_DIR_OUTBOUND;
    // peerID is unused outbound; brclientd ignores it.
    buf[2] = 0;
    buf[3] = 0;
    buf[4] = 0;
    buf[5] = 0;
    buf.set(opus, FRAME_HEADER_LEN);
    try {
      this.ws.send(buf);
      this.out.packetsSent++;
      this.out.ts += 20;
    } catch (err: any) {
      this.cb.onError?.(`WS send: ${err?.message ?? err}`);
    }
  }

  private handleInbound(data: ArrayBuffer | string): void {
    if (typeof data === 'string') return; // ignore text frames
    if (data.byteLength < FRAME_HEADER_LEN) return;
    const u8 = new Uint8Array(data);
    if (u8[0] !== FRAME_VERSION) return;
    if (u8[1] !== FRAME_DIR_INBOUND) return;
    const peerID = new DataView(data, 2, 4).getUint32(0, false);
    const opus = u8.subarray(FRAME_HEADER_LEN);
    if (opus.byteLength === 0) return;

    // Lazily build the playback state for this peer; ensureCtx is
    // synchronous after first use so the audio path stays low-latency.
    let peer = this.peers.get(peerID);
    if (!peer) {
      const created = this.createPeer(peerID);
      if (!created) {
        // Context init failed; nothing we can do per-frame.
        return;
      }
      peer = created;
    }

    peer.framesReceived++;
    peer.lastFrameWallMs = performance.now();

    const Chunk = (window as any).EncodedAudioChunk as typeof EncodedAudioChunk;
    const chunk = new Chunk({
      type: 'key',
      timestamp: peer.decodeTsUs,
      duration: 20000,
      data: opus,
    });
    peer.decodeTsUs += 20000;
    try {
      peer.decoder.decode(chunk);
    } catch (err: any) {
      this.cb.onError?.(`AudioDecoder.decode peer=${peerID}: ${err?.message ?? err}`);
    }

    this.cb.onInboundFrame?.(peerID, opus, 0);
  }

  private createPeer(peerID: number): PeerPlayback | null {
    if (!this.ctx) {
      // We don't have a context yet AND we have no synchronous way to
      // create one (autoplay rules require a gesture, but the gesture
      // for inbound is the click that started outbound). If outbound
      // hasn't started yet, drop the frame; the next one will land
      // after the ctx is up.
      void this.ensureCtx().catch(() => { /* ignore */ });
      return null;
    }
    const ctx = this.ctx;
    const gainNode = ctx.createGain();
    gainNode.gain.value = 1.0;
    gainNode.connect(ctx.destination);

    const Dec = (window as any).AudioDecoder as typeof AudioDecoder;
    const peer: PeerPlayback = {
      decoder: undefined as unknown as AudioDecoder, // assigned below
      gainNode,
      scheduledTime: 0,
      bufferDepthSec: RealtimeAudioPipeline.BUFFER_MIN_SEC,
      framesSinceShrink: 0,
      framesReceived: 0,
      lastFrameWallMs: 0,
      decodeTsUs: 0,
    };
    peer.decoder = new Dec({
      output: (ad) => this.playDecoded(peerID, ad),
      error: (err) => this.cb.onError?.(`AudioDecoder peer=${peerID}: ${err?.message ?? err}`),
    });
    peer.decoder.configure({
      codec: 'opus',
      sampleRate: 48000,
      numberOfChannels: 1,
    } as AudioDecoderConfig);
    this.peers.set(peerID, peer);
    return peer;
  }

  // playDecoded schedules a decoded PCM frame onto the per-peer gain
  // node, advancing the per-peer playhead. Adaptive jitter buffer:
  // - When scheduledTime falls behind ctx.currentTime + safety, treat
  //   it as an underrun and grow bufferDepthSec by 20 ms (capped 0.30).
  // - After BUFFER_SHRINK_AFTER consecutive frames without underruns,
  //   shrink bufferDepthSec back toward 0.08.
  private playDecoded(peerID: number, ad: AudioData): void {
    const peer = this.peers.get(peerID);
    if (!peer || !this.ctx) {
      ad.close();
      return;
    }
    const ctx = this.ctx;
    const frames = ad.numberOfFrames;
    if (frames === 0) {
      ad.close();
      return;
    }

    // Copy planar f32 into an AudioBuffer for scheduling.
    const buf = ctx.createBuffer(1, frames, 48000);
    const channelData = new Float32Array(frames);
    try {
      ad.copyTo(channelData, { planeIndex: 0, format: 'f32-planar' });
    } catch (err: any) {
      this.cb.onError?.(`AudioData.copyTo peer=${peerID}: ${err?.message ?? err}`);
      ad.close();
      return;
    }
    ad.close();
    buf.getChannelData(0).set(channelData);

    const now = ctx.currentTime;
    const minStart = now + peer.bufferDepthSec;
    let startAt: number;
    if (peer.scheduledTime < minStart) {
      // Underrun: either first frame, or playhead drifted past us.
      // Grow the buffer (bounded), reset the playhead.
      if (peer.scheduledTime > 0) {
        peer.bufferDepthSec = Math.min(
          RealtimeAudioPipeline.BUFFER_MAX_SEC,
          peer.bufferDepthSec + RealtimeAudioPipeline.BUFFER_GROW_STEP,
        );
        peer.framesSinceShrink = 0;
      }
      startAt = minStart;
    } else {
      startAt = peer.scheduledTime;
      peer.framesSinceShrink++;
      // Calm period: shrink the buffer back toward the floor.
      if (
        peer.framesSinceShrink >= RealtimeAudioPipeline.BUFFER_SHRINK_AFTER &&
        peer.bufferDepthSec > RealtimeAudioPipeline.BUFFER_MIN_SEC
      ) {
        peer.bufferDepthSec = Math.max(
          RealtimeAudioPipeline.BUFFER_MIN_SEC,
          peer.bufferDepthSec - RealtimeAudioPipeline.BUFFER_GROW_STEP,
        );
        peer.framesSinceShrink = 0;
      }
    }

    const src = ctx.createBufferSource();
    src.buffer = buf;
    src.connect(peer.gainNode);
    try {
      src.start(startAt);
    } catch {
      // startAt may be in the past on extreme drift; drop the frame.
      return;
    }
    peer.scheduledTime = startAt + RealtimeAudioPipeline.FRAME_DURATION_SEC;
  }

  private teardownInbound(): void {
    for (const peer of this.peers.values()) {
      try { peer.decoder.close(); } catch { /* ignore */ }
      try { peer.gainNode.disconnect(); } catch { /* ignore */ }
    }
    this.peers.clear();
  }

  private teardownOutbound(): void {
    if (!this.out) return;
    try { this.out.encoder.close(); } catch { /* ignore */ }
    try { this.out.worklet.disconnect(); } catch { /* ignore */ }
    try { this.out.stream.getTracks().forEach((t) => t.stop()); } catch { /* ignore */ }
    this.out = null;
    // Note: the shared AudioContext is owned by the pipeline and closed
    // in stop(), not here.
  }
}

// workletURL builds a data: URL for the mic-tap AudioWorklet so we don't
// have to ship a separate static file. The worklet emits Float32Array
// buffers on each render quantum to the main thread.
let cachedWorkletURL: string | null = null;
const workletURL = (): string => {
  if (cachedWorkletURL) return cachedWorkletURL;
  const src = `
class RTDTMicTap extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch && ch.length) {
      this.port.postMessage({ samples: ch.slice() });
    }
    return true;
  }
}
registerProcessor('rtdt-mic-tap', RTDTMicTap);
  `;
  const blob = new Blob([src], { type: 'application/javascript' });
  cachedWorkletURL = URL.createObjectURL(blob);
  return cachedWorkletURL;
};
