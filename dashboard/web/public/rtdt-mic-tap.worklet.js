// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Mic tap for Bison Relay realtime calls: forwards each render quantum to the
// main thread, which encodes it to Opus. Shipped as a file rather than built
// as a blob: URL at runtime, because the document CSP allows scripts from
// 'self' only.
class RTDTMicTap extends AudioWorkletProcessor {
  constructor(options) {
    super();
    this.epoch = options?.processorOptions?.epoch ?? 0;
    this.muted = options?.processorOptions?.muted ?? true;
    this.port.onmessage = ({ data }) => {
      this.epoch = data.epoch;
      this.muted = data.muted;
    };
  }

  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (!this.muted && ch && ch.length) {
      this.port.postMessage({ epoch: this.epoch, samples: ch.slice() });
    }
    return true;
  }
}

registerProcessor('rtdt-mic-tap', RTDTMicTap);
