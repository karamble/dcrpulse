// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Mic tap for Bison Relay realtime calls: forwards each render quantum to the
// main thread, which encodes it to Opus. Shipped as a file rather than built
// as a blob: URL at runtime, because the document CSP allows scripts from
// 'self' only.
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
