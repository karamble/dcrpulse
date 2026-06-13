// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// dcrtime file-hashing worker. This is the ONLY place WebAssembly runs in the
// dashboard, so its served response carries a worker-scoped wasm-unsafe-eval
// CSP while the document CSP stays strict. The file is streamed in chunks and
// never leaves the browser - only the resulting digest is returned.

import { createSHA256 } from 'hash-wasm';

// self is the worker global; cast to avoid the DOM Window.postMessage signature.
const ctx: any = self;

const CHUNK = 8 * 1024 * 1024; // 8 MiB

ctx.onmessage = async (e: MessageEvent<{ file: Blob }>) => {
  const { file } = e.data;
  try {
    const hasher = await createSHA256();
    hasher.init();
    const total = file.size;
    let offset = 0;
    while (offset < total) {
      const slice = file.slice(offset, offset + CHUNK);
      const buf = new Uint8Array(await slice.arrayBuffer());
      hasher.update(buf);
      offset += CHUNK;
      ctx.postMessage({ type: 'progress', progress: total ? Math.min(1, offset / total) : 1 });
    }
    if (total === 0) {
      ctx.postMessage({ type: 'progress', progress: 1 });
    }
    ctx.postMessage({ type: 'done', digest: hasher.digest('hex') });
  } catch (err: any) {
    ctx.postMessage({ type: 'error', error: String(err?.message || err) });
  }
};
