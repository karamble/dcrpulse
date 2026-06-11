// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Color math for the theme editor. Tokens are stored as HSL channels
// ("H S% L%"); the native <input type="color"> speaks hex. These helpers
// bridge the two and compute WCAG contrast. No third-party dependency.

import { HslChannels } from './types';

// Matches "H S% L%" with optional decimals, e.g. "217 91% 60%".
const CHANNELS_RE = /^\s*\d{1,3}(\.\d+)?\s+\d{1,3}(\.\d+)?%\s+\d{1,3}(\.\d+)?%\s*$/;

export function isHslChannels(v: unknown): v is HslChannels {
  return typeof v === 'string' && CHANNELS_RE.test(v);
}

interface Hsl {
  h: number;
  s: number;
  l: number;
}

function parseChannels(channels: HslChannels): Hsl {
  const [h, s, l] = channels
    .trim()
    .replace(/%/g, '')
    .split(/\s+/)
    .map(Number);
  return { h, s, l };
}

function hslToRgb({ h, s, l }: Hsl): [number, number, number] {
  const sN = s / 100;
  const lN = l / 100;
  const c = (1 - Math.abs(2 * lN - 1)) * sN;
  const hp = ((h % 360) + 360) % 360 / 60;
  const x = c * (1 - Math.abs((hp % 2) - 1));
  let r = 0;
  let g = 0;
  let b = 0;
  if (hp >= 0 && hp < 1) [r, g, b] = [c, x, 0];
  else if (hp < 2) [r, g, b] = [x, c, 0];
  else if (hp < 3) [r, g, b] = [0, c, x];
  else if (hp < 4) [r, g, b] = [0, x, c];
  else if (hp < 5) [r, g, b] = [x, 0, c];
  else [r, g, b] = [c, 0, x];
  const m = lN - c / 2;
  return [
    Math.round((r + m) * 255),
    Math.round((g + m) * 255),
    Math.round((b + m) * 255),
  ];
}

function rgbToHsl(r: number, g: number, b: number): Hsl {
  const rN = r / 255;
  const gN = g / 255;
  const bN = b / 255;
  const max = Math.max(rN, gN, bN);
  const min = Math.min(rN, gN, bN);
  const d = max - min;
  let h = 0;
  if (d !== 0) {
    if (max === rN) h = ((gN - bN) / d) % 6;
    else if (max === gN) h = (bN - rN) / d + 2;
    else h = (rN - gN) / d + 4;
    h *= 60;
    if (h < 0) h += 360;
  }
  const l = (max + min) / 2;
  const s = d === 0 ? 0 : d / (1 - Math.abs(2 * l - 1));
  return { h: Math.round(h), s: Math.round(s * 100), l: Math.round(l * 100) };
}

const toHexByte = (n: number) => n.toString(16).padStart(2, '0');

export function hslChannelsToHex(channels: HslChannels): string {
  if (!isHslChannels(channels)) return '#000000';
  const [r, g, b] = hslToRgb(parseChannels(channels));
  return `#${toHexByte(r)}${toHexByte(g)}${toHexByte(b)}`;
}

export function hexToHslChannels(hex: string): HslChannels {
  const m = /^#?([0-9a-fA-F]{6})$/.exec(hex.trim());
  if (!m) return '0 0% 0%';
  const int = parseInt(m[1], 16);
  const { h, s, l } = rgbToHsl((int >> 16) & 255, (int >> 8) & 255, int & 255);
  return `${h} ${s}% ${l}%`;
}

// Relative luminance per WCAG 2.x.
function luminance(channels: HslChannels): number {
  const [r, g, b] = hslToRgb(parseChannels(channels)).map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// Contrast ratio (1..21) between two HSL-channel colors.
export function contrastRatio(a: HslChannels, b: HslChannels): number {
  if (!isHslChannels(a) || !isHslChannels(b)) return 1;
  const la = luminance(a);
  const lb = luminance(b);
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}
