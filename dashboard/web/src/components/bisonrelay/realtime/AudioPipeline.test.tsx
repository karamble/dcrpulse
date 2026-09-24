import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeAudioPipeline } from './AudioPipeline';
import workletSource from '../../../../public/rtdt-mic-tap.worklet.js?raw';

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
};
const last = <T,>(items: T[]): T => items[items.length - 1];
const settle = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };

class FakeSocket {
  static OPEN = 1;
  static instances: FakeSocket[] = [];
  readyState = 0;
  binaryType = '';
  onopen: (() => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((e: { data: ArrayBuffer }) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => { this.readyState = 3; });
  constructor(readonly url: string) { FakeSocket.instances.push(this); }
  open() { this.readyState = 1; this.onopen?.(); }
  disconnect(code = 1006) { this.readyState = 3; this.onclose?.({ code }); }
}
class FakeEncoder {
  static instances: FakeEncoder[] = [];
  static configureError: Error | null = null;
  state = 'unconfigured';
  configure = vi.fn(() => {
    if (FakeEncoder.configureError) throw FakeEncoder.configureError;
    this.state = 'configured';
  });
  encode = vi.fn();
  close = vi.fn(() => { this.state = 'closed'; });
  constructor(readonly callbacks: { output: (chunk: any) => void; error: (e: Error) => void }) {
    FakeEncoder.instances.push(this);
  }
  output() {
    // Permit callbacks even after close to exercise ownership checks.
    this.callbacks.output({ byteLength: 2, copyTo: (out: Uint8Array) => out.set([42, 43]) });
  }
}
class FakeAudioData {
  close = vi.fn();
  constructor(readonly init: { data: Float32Array }) {}
}
class FakeDecoder {
  static instances: FakeDecoder[] = [];
  configure = vi.fn();
  decode = vi.fn();
  close = vi.fn();
  constructor(readonly callbacks: any) { FakeDecoder.instances.push(this); }
}
class FakeContext {
  static instances: FakeContext[] = [];
  static modulePromise: Promise<void> | undefined;
  static resumePromise: Promise<void> | undefined;
  state = FakeContext.resumePromise ? 'suspended' : 'running';
  audioWorklet = { addModule: vi.fn(() => FakeContext.modulePromise ?? Promise.resolve()) };
  source = { connect: vi.fn(), disconnect: vi.fn() };
  createMediaStreamSource = vi.fn(() => this.source);
  createGain = vi.fn(() => ({ gain: { value: 1 }, connect: vi.fn(), disconnect: vi.fn() }));
  resume = vi.fn(() => FakeContext.resumePromise ?? Promise.resolve());
  close = vi.fn(async () => { this.state = 'closed'; });
  constructor() { FakeContext.instances.push(this); }
}
class FakeWorklet {
  static instances: FakeWorklet[] = [];
  messages: any[] = [];
  processor: any;
  port = {
    onmessage: null as null | ((e: { data: any }) => void),
    postMessage: vi.fn((data: any) => this.processor.port.onmessage?.({ data })),
    close: vi.fn(),
  };
  disconnect = vi.fn();
  constructor(_ctx?: unknown, _name?: string, options?: unknown) {
    const messages = this.messages;
    class ProcessorBase {
      port = { onmessage: null, postMessage: (data: any) => messages.push(data) };
    }
    let Processor: any;
    // Run the shipped processor, not a second mock of its epoch protocol.
    new Function('AudioWorkletProcessor', 'registerProcessor', workletSource)(
      ProcessorBase, (_name: string, ctor: any) => { Processor = ctor; },
    );
    this.processor = new Processor(options);
    FakeWorklet.instances.push(this);
  }
  capture(size = 960, value = 0.5) { this.processor.process([[new Float32Array(size).fill(value)]]); }
  deliver() { for (const data of this.messages.splice(0)) this.port.onmessage?.({ data }); }
  frame(size = 960, value = 0.5) { this.capture(size, value); this.deliver(); }
}
const streams: { getTracks: () => any[]; getAudioTracks: () => any[]; track: any }[] = [];
const makeStream = () => {
  const track = { enabled: true, stop: vi.fn() };
  const stream = { getTracks: () => [track], getAudioTracks: () => [track], track };
  streams.push(stream);
  return stream;
};
const gum = vi.fn();
const pipelines: RealtimeAudioPipeline[] = [];
const pipeline = (options: Partial<ConstructorParameters<typeof RealtimeAudioPipeline>[0]> = {}) => {
  const p = new RealtimeAudioPipeline({ rv: 'a'.repeat(64), ...options });
  pipelines.push(p);
  return p;
};
const connect = async (p: RealtimeAudioPipeline) => {
  await p.start();
  const ws = last(FakeSocket.instances);
  ws.open();
  await settle();
  return ws;
};
const reconnect = async (ws: FakeSocket) => {
  ws.disconnect();
  await vi.advanceTimersByTimeAsync(1000);
  const next = last(FakeSocket.instances);
  expect(next).not.toBe(ws);
  next.open();
  await settle();
  return next;
};

beforeEach(() => {
  vi.useFakeTimers();
  FakeSocket.instances = []; FakeEncoder.instances = []; FakeDecoder.instances = [];
  FakeContext.instances = []; FakeWorklet.instances = []; streams.length = 0;
  FakeContext.modulePromise = undefined; FakeContext.resumePromise = undefined;
  FakeEncoder.configureError = null;
  gum.mockReset().mockImplementation(async () => makeStream());
  vi.stubGlobal('WebSocket', FakeSocket);
  vi.stubGlobal('AudioContext', FakeContext);
  vi.stubGlobal('AudioWorkletNode', FakeWorklet);
  vi.stubGlobal('AudioEncoder', FakeEncoder);
  vi.stubGlobal('AudioData', FakeAudioData);
  vi.stubGlobal('AudioDecoder', FakeDecoder);
  vi.stubGlobal('EncodedAudioChunk', class { constructor(readonly init: any) {} });
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: gum } });
});
afterEach(() => {
  for (const p of pipelines.splice(0)) p.stop();
  vi.useRealTimers(); vi.unstubAllGlobals();
});

describe('call microphone consent', () => {
  it('sends when enabled, including after reconnect', async () => {
    const p = pipeline(); const first = await connect(p);
    last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output();
    expect(first.send).toHaveBeenCalledTimes(1);
    const next = await reconnect(first);
    last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output();
    expect(next.send).toHaveBeenCalledTimes(1);
    expect(Array.from(next.send.mock.calls[0][0])).toEqual([1, 2, 0, 0, 0, 0, 42, 43]);
  });

  it('preserves mute across reconnect and suppresses encoded output', async () => {
    const p = pipeline(); const first = await connect(p);
    p.setMuted(true);
    const next = await reconnect(first);
    expect(p.isMuted()).toBe(true);
    last(FakeWorklet.instances).frame();
    for (const enc of FakeEncoder.instances) enc.output();
    expect(next.send).not.toHaveBeenCalled();
  });

  it('drops an encoded chunk delivered after mute', async () => {
    const p = pipeline(); const ws = await connect(p);
    FakeWorklet.instances[0].frame();
    const enc = FakeEncoder.instances[0];
    expect(enc.encode).toHaveBeenCalledTimes(1);
    p.setMuted(true); enc.output();
    expect(ws.send).not.toHaveBeenCalled();
  });

  it('honors mute requested while microphone permission is pending', async () => {
    const pending = deferred<ReturnType<typeof makeStream>>(); gum.mockReturnValue(pending.promise);
    const p = pipeline(); const ws = await connect(p);
    p.setMuted(true);
    pending.resolve(makeStream()); await settle();
    expect(p.isMuted()).toBe(true);
    last(FakeWorklet.instances).frame();
    for (const enc of FakeEncoder.instances) enc.output();
    expect(ws.send).not.toHaveBeenCalled();
  });
});

describe('capture ownership and epochs', () => {
  it('starts muted without encoding, keeps playback, then explicitly unmutes', async () => {
    const inbound = vi.fn();
    const p = pipeline({ initialMuted: true, callbacks: { onInboundFrame: inbound } });
    const ws = await connect(p);
    expect(p.isMuted()).toBe(true);
    expect(streams[0].track.enabled).toBe(false);
    const tap = last(FakeWorklet.instances);
    tap.frame();
    expect(FakeEncoder.instances).toHaveLength(0);
    ws.onmessage?.({ data: new Uint8Array([1, 1, 0, 0, 0, 7, 42]).buffer });
    expect(inbound).toHaveBeenCalledTimes(1);
    expect(last(FakeDecoder.instances).decode).toHaveBeenCalledTimes(1);
    p.setMuted(false);
    expect(streams[0].track.enabled).toBe(true);
    tap.frame(); last(FakeEncoder.instances).output();
    expect(ws.send).toHaveBeenCalledTimes(1);
    expect(last(FakeEncoder.instances).configure).toHaveBeenCalledWith({
      codec: 'opus', sampleRate: 48000, numberOfChannels: 1, bitrate: 40000,
      opus: { application: 'voip', frameDuration: 20000, useinbandfec: true },
    });
  });

  it('keeps mute intent while disconnected and uses the latest choice on reconnect', async () => {
    const p = pipeline(); const ws = await connect(p);
    ws.disconnect(); p.setMuted(true);
    expect(p.isMuted()).toBe(true);
    await vi.advanceTimersByTimeAsync(1000);
    last(FakeSocket.instances).open(); await settle();
    expect(last(streams).track.enabled).toBe(false);
    p.setMuted(false); last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output();
    expect(last(FakeSocket.instances).send).toHaveBeenCalledTimes(1);
  });

  it('rejects queued PCM and encoder output even after unmuting, and clears partial samples', async () => {
    const p = pipeline(); const ws = await connect(p); const tap = last(FakeWorklet.instances);
    const oldEncoder = last(FakeEncoder.instances);
    tap.frame(480, 0.1); // half a frame must never survive a mute cycle
    tap.capture(960, 0.2); // produced before mute, delivered after unmute
    p.setMuted(true); tap.capture(960, 0.3);
    p.setMuted(false);
    tap.deliver(); oldEncoder.output();
    const encoder = last(FakeEncoder.instances);
    expect(encoder).not.toBe(oldEncoder);
    expect(oldEncoder.close).toHaveBeenCalled();
    expect(encoder.encode).not.toHaveBeenCalled();
    expect(ws.send).not.toHaveBeenCalled();
    tap.frame(480, 0.4); expect(encoder.encode).not.toHaveBeenCalled();
    tap.frame(480, 0.4); expect(encoder.encode).toHaveBeenCalledTimes(1);
    const ad = encoder.encode.mock.calls[0][0] as FakeAudioData;
    expect(Array.from(ad.init.data).every((v) => Math.abs(v - 0.4) < 0.00001)).toBe(true);
    expect(ad.close).toHaveBeenCalled();
    encoder.output(); expect(ws.send).toHaveBeenCalledTimes(1);
  });

  it('honors mute and unmute during worklet loading', async () => {
    const module = deferred<void>(); FakeContext.modulePromise = module.promise;
    const p = pipeline(); const ws = await connect(p);
    p.setMuted(true); expect(streams[0].track.enabled).toBe(false);
    p.setMuted(false); p.setMuted(true);
    module.resolve(); await settle();
    expect(p.isMuted()).toBe(true);
    expect(streams[0].track.enabled).toBe(false);
    last(FakeWorklet.instances).frame(); expect(ws.send).not.toHaveBeenCalled();
    p.setMuted(false); last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output();
    expect(ws.send).toHaveBeenCalledTimes(1);
  });

  for (const stage of ['permission', 'resume', 'module'] as const) {
    it(`stops a pending ${stage} acquisition and ignores its late completion`, async () => {
      const pendingStream = deferred<ReturnType<typeof makeStream>>();
      const pending = deferred<void>();
      if (stage === 'permission') gum.mockReturnValueOnce(pendingStream.promise);
      if (stage === 'resume') FakeContext.resumePromise = pending.promise;
      if (stage === 'module') FakeContext.modulePromise = pending.promise;
      const error = vi.fn(); const p = pipeline({ callbacks: { onError: error } });
      const ws = await connect(p);
      p.stop();
      if (stage !== 'permission') expect(streams[0].track.stop).toHaveBeenCalled();
      if (stage === 'permission') pendingStream.resolve(makeStream()); else pending.resolve();
      await settle();
      expect(streams[0].track.stop).toHaveBeenCalled();
      expect(FakeWorklet.instances).toHaveLength(0);
      expect(FakeEncoder.instances).toHaveLength(0);
      expect(error).not.toHaveBeenCalled();
      expect(ws.send).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(60000);
      expect(FakeSocket.instances).toHaveLength(1);
    });
  }

  it('does not install an old microphone result over a new connection', async () => {
    const firstMic = deferred<ReturnType<typeof makeStream>>(); gum.mockReturnValueOnce(firstMic.promise);
    const p = pipeline(); const first = await connect(p);
    const next = await reconnect(first);
    const currentTap = last(FakeWorklet.instances); const currentEncoder = last(FakeEncoder.instances);
    const late = makeStream(); firstMic.resolve(late); await settle();
    expect(late.track.stop).toHaveBeenCalled();
    expect(FakeWorklet.instances).toHaveLength(1);
    currentTap.frame(); currentEncoder.output();
    expect(next.send).toHaveBeenCalledTimes(1);
  });

  it('ignores old socket, worklet, and encoder callbacks after replacement', async () => {
    const error = vi.fn(), disconnected = vi.fn(), inbound = vi.fn();
    const p = pipeline({ callbacks: { onError: error, onDisconnected: disconnected, onInboundFrame: inbound } });
    const first = await connect(p);
    const oldTap = last(FakeWorklet.instances); const oldPCM = oldTap.port.onmessage!;
    const oldEncoder = last(FakeEncoder.instances);
    oldTap.capture(); const oldMessage = oldTap.messages[0];
    const next = await reconnect(first);
    const currentTrack = last(streams).track;
    first.onclose?.({ code: 1006 }); first.onopen?.(); first.onerror?.();
    first.onmessage?.({ data: new Uint8Array([1, 1, 0, 0, 0, 7, 42]).buffer });
    oldPCM({ data: oldMessage }); oldEncoder.output(); oldEncoder.callbacks.error(new Error('late'));
    expect(next.send).not.toHaveBeenCalled();
    expect(currentTrack.stop).not.toHaveBeenCalled();
    expect(disconnected).toHaveBeenCalledTimes(1);
    expect(error).not.toHaveBeenCalled(); expect(inbound).not.toHaveBeenCalled();
    last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output();
    expect(next.send).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(30000); expect(FakeSocket.instances).toHaveLength(2);
  });

  it('cleans up partial setup when module loading or encoder configuration fails', async () => {
    const module = deferred<void>(); FakeContext.modulePromise = module.promise;
    const error = vi.fn(); const p = pipeline({ callbacks: { onError: error } });
    await connect(p); module.reject(new Error('module failed')); await settle();
    expect(streams[0].track.stop).toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith('module failed');
    p.stop();
    FakeContext.modulePromise = undefined; FakeEncoder.configureError = new Error('codec failed');
    await connect(p); await settle();
    expect(last(streams).track.stop).toHaveBeenCalled();
    expect(last(FakeEncoder.instances).close).toHaveBeenCalled();
    expect(last(FakeWorklet.instances).disconnect).toHaveBeenCalled();
    expect(last(FakeContext.instances).source.disconnect).toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith('codec failed');
  });

  it('ignores a late setup rejection after stop', async () => {
    const pending = deferred<ReturnType<typeof makeStream>>(); gum.mockReturnValue(pending.promise);
    const error = vi.fn(); const p = pipeline({ callbacks: { onError: error } });
    await connect(p); p.stop(); pending.reject(new Error('permission denied')); await settle();
    expect(error).not.toHaveBeenCalled();
  });

  it('keeps the packet rate limit and stops scheduled retries', async () => {
    const p = pipeline(); const ws = await connect(p);
    for (let i = 0; i < 60; i++) { last(FakeWorklet.instances).frame(); last(FakeEncoder.instances).output(); }
    expect(ws.send).toHaveBeenCalledTimes(55);
    expect(p.outboundCounters()).toEqual({ sent: 55, rateLimited: 5 });
    ws.disconnect(); p.stop(); await vi.advanceTimersByTimeAsync(60000);
    expect(FakeSocket.instances).toHaveLength(1);
  });

  it('does not retry policy close and makes repeated start idempotent', async () => {
    const error = vi.fn(); const p = pipeline({ callbacks: { onError: error } });
    const ws = await connect(p); await p.start();
    expect(FakeSocket.instances).toHaveLength(1);
    ws.disconnect(1008); await vi.advanceTimersByTimeAsync(60000);
    expect(FakeSocket.instances).toHaveLength(1);
    expect(error).toHaveBeenCalledWith('This call is already attached in another tab.');
  });
});
