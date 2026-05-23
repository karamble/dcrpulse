// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Helpers shared by ContactAvatar in the contacts list and BigAvatar in the
// per-user sub-nav header. Kept in their own module so both surfaces stay in
// sync if the palette or sniff list ever changes.

const palette = [
  'bg-rose-600', 'bg-amber-600', 'bg-emerald-600', 'bg-teal-600',
  'bg-sky-600', 'bg-indigo-600', 'bg-fuchsia-600', 'bg-pink-600',
];

export function colorForUid(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return palette[h % palette.length];
}

// Sniffs PNG/JPEG/GIF/WebP magic bytes. Returns a data: URL or '' if the
// blob is empty or of an unrecognised type. AddressBook avatars from BR are
// raw bytes base64-encoded; we cannot trust a content-type header.
export function avatarDataUrl(b64?: string): string {
  if (!b64) return '';
  try {
    const bin = atob(b64);
    if (bin.length < 4) return '';
    const b0 = bin.charCodeAt(0);
    const b1 = bin.charCodeAt(1);
    const b2 = bin.charCodeAt(2);
    const b3 = bin.charCodeAt(3);
    let mime = '';
    if (b0 === 0x89 && b1 === 0x50 && b2 === 0x4e && b3 === 0x47) mime = 'image/png';
    else if (b0 === 0xff && b1 === 0xd8 && b2 === 0xff) mime = 'image/jpeg';
    else if (b0 === 0x47 && b1 === 0x49 && b2 === 0x46) mime = 'image/gif';
    else if (b0 === 0x52 && b1 === 0x49 && b2 === 0x46 && b3 === 0x46 && bin.length > 11 &&
      bin.substring(8, 12) === 'WEBP') mime = 'image/webp';
    if (!mime) return '';
    return `data:${mime};base64,${b64}`;
  } catch {
    return '';
  }
}
