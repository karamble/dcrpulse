// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// hashFile computes the SHA-256 of a file entirely in the browser via a Web
// Worker (so the UI stays responsive and WebAssembly is confined to the worker).
// The file is never uploaded or written to disk; only the hex digest is returned.
export function hashFile(file: File, onProgress?: (fraction: number) => void): Promise<string> {
  return new Promise((resolve, reject) => {
    let worker: Worker;
    try {
      worker = new Worker(new URL('../workers/dcrtime-hash-worker.ts', import.meta.url), {
        type: 'module',
      });
    } catch (err: any) {
      reject(new Error('could not start the hashing worker: ' + (err?.message || err)));
      return;
    }
    worker.onmessage = (e: MessageEvent) => {
      const msg = e.data;
      if (msg?.type === 'progress') {
        onProgress?.(msg.progress);
      } else if (msg?.type === 'done') {
        worker.terminate();
        resolve(msg.digest as string);
      } else if (msg?.type === 'error') {
        worker.terminate();
        reject(new Error(msg.error || 'hashing failed'));
      }
    };
    worker.onerror = (e) => {
      worker.terminate();
      reject(new Error(e.message || 'hashing worker error'));
    };
    worker.postMessage({ file });
  });
}
