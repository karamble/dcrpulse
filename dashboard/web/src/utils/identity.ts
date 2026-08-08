// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// identityToHex normalises a Bison Relay identity to lowercase hex. The daemon
// sends it base64 in some replies and hex in others, so an already-hex value
// passes through. Anything that is not 32 bytes is returned unchanged, which
// simply fails to match wherever the result is compared.
export const identityToHex = (s: string): string => {
  if (/^[0-9a-f]{64}$/i.test(s)) return s;
  try {
    const bin = atob(s);
    if (bin.length !== 32) return s;
    let hex = '';
    for (let i = 0; i < bin.length; i++) {
      hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  } catch {
    return s;
  }
};
