// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Writer half of BR's --embed[...]-- syntax. The reader half lives in
// embedParser.ts; canonical Go reference is
// github.com/companyzero/bisonrelay/internal/mdembeds.

// EditorEmbed holds the side-map data for one staged attachment while the
// user is composing. Two practical shapes: inline data (name+type+data) or
// download reference (download fid + filename + size + optional cost).
// The editor keeps a `Record<id, EditorEmbed>` and inlines the wire form
// at submit/preview time.
export interface EditorEmbed {
  // Display name for the toolbar chip + alt fallback.
  displayName: string;

  // Inline-data path:
  name?: string;
  mime?: string;
  dataB64?: string;
  alt?: string;

  // Download-reference path (paywall when cost > 0):
  download?: string;
  filename?: string;
  size?: number;
  cost?: number; // milliatoms
}

export type EditorEmbedMap = Record<string, EditorEmbed>;

// newEmbedId returns a short hex string used as the placeholder id in the
// editor's text buffer. Collisions are negligible for typical post sizes
// but the editor still guards against a hash already present in the map.
export function newEmbedId(existing: EditorEmbedMap): string {
  let id = randomHex(8);
  let tries = 0;
  while (existing[id] && tries < 8) {
    id = randomHex(8);
    tries++;
  }
  return id;
}

const HEX = '0123456789abcdef';
function randomHex(len: number): string {
  let out = '';
  for (let i = 0; i < len; i++) {
    out += HEX[Math.floor(Math.random() * 16)];
  }
  return out;
}

// placeholderFor returns the literal placeholder string the editor splices
// into the textarea. Kept short so the user can move/delete it cleanly.
export function placeholderFor(id: string): string {
  return `--embed[id=${id}]--`;
}

// composeBRBody substitutes every --embed[id=X]-- placeholder in the
// display body with the full BR wire-format tag derived from the side
// map. Placeholders without a backing map entry are dropped (treated as
// leftover from a removed attachment).
export function composeBRBody(displayBody: string, embeds: EditorEmbedMap): string {
  return displayBody.replace(/--embed\[id=([0-9a-f]+)\]--/g, (_, id) => {
    const e = embeds[id];
    if (!e) return '';
    return embedToWireString(e);
  });
}

function embedToWireString(e: EditorEmbed): string {
  const parts: string[] = [];
  // Wire order matches mdembeds.go's EmbeddedArgs.String for readability,
  // but BR doesn't depend on the order — it's just a comma-split.
  if (e.name) parts.push(`name=${e.name}`);
  if (e.alt) parts.push(`alt=${encodeURIComponent(e.alt)}`);
  if (e.mime) parts.push(`type=${e.mime}`);
  if (e.download) parts.push(`download=${e.download}`);
  if (e.filename) parts.push(`filename=${e.filename}`);
  if (e.size && e.size > 0) parts.push(`size=${e.size}`);
  if (e.cost && e.cost > 0) parts.push(`cost=${e.cost}`);
  if (e.dataB64) parts.push(`data=${e.dataB64}`);
  return `--embed[${parts.join(',')}]--`;
}

// estimatedWireBytes returns the approximate wire size of the composed
// post body, including all embed payloads. Used by the editor footer.
export function estimatedWireBytes(displayBody: string, embeds: EditorEmbedMap): number {
  let total = displayBody.length;
  for (const id in embeds) {
    const e = embeds[id];
    // Subtract the placeholder length (we'll re-add the wire form).
    total -= placeholderFor(id).length;
    total += embedToWireString(e).length;
  }
  return Math.max(0, total);
}
