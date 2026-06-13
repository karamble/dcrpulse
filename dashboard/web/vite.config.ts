// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  // The dcrtime file-hashing worker is emitted with a distinctive, matchable
  // name so the Go static server can scope the wasm-unsafe-eval CSP to just
  // that one asset (the only place WebAssembly runs).
  worker: {
    format: 'es',
    rollupOptions: {
      output: {
        entryFileNames: 'assets/dcrtime-hash-worker-[hash].js',
        chunkFileNames: 'assets/dcrtime-hash-worker-[hash].js',
      },
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})

