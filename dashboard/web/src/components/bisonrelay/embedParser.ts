// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

export interface EmbedSegment {
  kind: 'embed';
  raw: string;
  name: string;
  mime: string;
  alt: string;
  dataB64: string;
  size: number;
  filename: string;
  download: string;
  cost: number;
  localFilename: string;
  // Quote-by-reference embed (type=quote): both are validated 64-hex or
  // empty. See docs/features/bison-relay-quote-embed.md.
  quoteFrom: string;
  quotePost: string;
}

const QUOTE_ID_RE = /^[0-9a-f]{64}$/;

export interface TextSegment {
  kind: 'text';
  text: string;
}

export interface DownloadSegment {
  kind: 'download';
  raw: string;
  nick: string;
  filename: string;
  size: number;
  mime: string;
}

export type MessageSegment = TextSegment | EmbedSegment | DownloadSegment;

const TAG_RE = /--(embed|download)\[(.*?)\]--/g;

export function parseEmbeds(body: string): MessageSegment[] {
  if (!body) return [];
  const segments: MessageSegment[] = [];
  let lastIndex = 0;
  TAG_RE.lastIndex = 0;
  for (let m = TAG_RE.exec(body); m !== null; m = TAG_RE.exec(body)) {
    if (m.index > lastIndex) {
      segments.push({ kind: 'text', text: body.substring(lastIndex, m.index) });
    }
    if (m[1] === 'embed') {
      segments.push(parseEmbedArgs(m[0], m[2]));
    } else {
      segments.push(parseDownloadArgs(m[0], m[2]));
    }
    lastIndex = m.index + m[0].length;
  }
  if (lastIndex < body.length) {
    segments.push({ kind: 'text', text: body.substring(lastIndex) });
  }
  if (segments.length === 0) {
    segments.push({ kind: 'text', text: body });
  }
  return segments;
}

function parseEmbedArgs(raw: string, inner: string): EmbedSegment {
  const out: EmbedSegment = {
    kind: 'embed',
    raw,
    name: '',
    mime: '',
    alt: '',
    dataB64: '',
    size: 0,
    filename: '',
    download: '',
    cost: 0,
    localFilename: '',
    quoteFrom: '',
    quotePost: '',
  };
  for (const part of inner.split(',')) {
    const eq = part.indexOf('=');
    if (eq < 0) continue;
    const k = part.substring(0, eq);
    const v = part.substring(eq + 1);
    switch (k) {
      case 'name':
        out.name = v;
        break;
      case 'type':
        out.mime = v;
        break;
      case 'data':
        out.dataB64 = v;
        break;
      case 'alt':
        try {
          out.alt = decodeURIComponent(v);
        } catch {
          out.alt = v;
        }
        break;
      case 'filename':
        out.filename = v;
        break;
      case 'size':
        out.size = Number(v) || 0;
        break;
      case 'cost':
        out.cost = Number(v) || 0;
        break;
      case 'download':
        out.download = v;
        break;
      case 'localfilename':
        out.localFilename = v;
        break;
      case 'from': {
        const lv = v.toLowerCase();
        if (QUOTE_ID_RE.test(lv)) out.quoteFrom = lv;
        break;
      }
      case 'post': {
        const lv = v.toLowerCase();
        if (QUOTE_ID_RE.test(lv)) out.quotePost = lv;
        break;
      }
    }
  }
  return out;
}

function parseDownloadArgs(raw: string, inner: string): DownloadSegment {
  const out: DownloadSegment = {
    kind: 'download',
    raw,
    nick: '',
    filename: '',
    size: 0,
    mime: '',
  };
  for (const part of inner.split(',')) {
    const eq = part.indexOf('=');
    if (eq < 0) continue;
    const k = part.substring(0, eq);
    const v = part.substring(eq + 1);
    switch (k) {
      case 'nick':
        out.nick = v;
        break;
      case 'name':
        out.filename = v;
        break;
      case 'size':
        out.size = Number(v) || 0;
        break;
      case 'type':
        out.mime = v;
        break;
    }
  }
  return out;
}

export function buildDownloadTag(nick: string, filename: string, size: number, mime: string): string {
  const parts: string[] = [];
  if (nick) parts.push('nick=' + nick.replace(/[,=]/g, ''));
  if (filename) parts.push('name=' + filename.replace(/[,=]/g, ''));
  if (size) parts.push('size=' + size);
  if (mime) parts.push('type=' + mime.replace(/[,=]/g, ''));
  return '--download[' + parts.join(',') + ']--';
}

export function downloadFileUrl(nick: string, filename: string): string {
  if (!nick || !filename) return '';
  return `/api/br/downloads/${encodeURIComponent(nick)}/${encodeURIComponent(filename)}`;
}

// embedFileUrl converts a localfilename of the form
// "embeds/<contact_short>/<filename>" into the dashboard URL that serves
// the persisted bytes. Returns '' if the input doesn't match.
export function embedFileUrl(localFilename: string): string {
  if (!localFilename) return '';
  const parts = localFilename.split('/');
  if (parts.length !== 3 || parts[0] !== 'embeds') return '';
  const contact = parts[1];
  const filename = parts[2];
  if (!/^[0-9a-f]{16}$/.test(contact)) return '';
  if (!/^[A-Za-z0-9._-]+$/.test(filename)) return '';
  return `/api/br/embeds/${contact}/${encodeURIComponent(filename)}`;
}

// ALLOWED_IMAGE_MIMES are the raster image types we render inline. SVG and any
// other type are intentionally excluded: peer-supplied embeds are rendered via
// data: URIs in <img>, so limiting to known-safe raster formats avoids
// SVG-script and MIME-sniffing edge cases.
const ALLOWED_IMAGE_MIMES = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
]);

export function isImageMime(mime?: string): boolean {
  return ALLOWED_IMAGE_MIMES.has((mime || '').toLowerCase().trim());
}

export function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}
