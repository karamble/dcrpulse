// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';

// Text scale for the Bison Relay section only, mirroring bruig's message
// font size setting. A per-browser preference, so it lives in localStorage
// rather than on the server.

export type BrTextScale = 'xsmall' | 'small' | 'default' | 'medium' | 'large' | 'xlarge';

export const BR_TEXT_SCALES: { id: BrTextScale; label: string; factor: number }[] = [
  { id: 'xsmall', label: 'Extra small', factor: 0.65 },
  { id: 'small', label: 'Small', factor: 0.85 },
  { id: 'default', label: 'Default', factor: 1 },
  { id: 'medium', label: 'Medium', factor: 1.15 },
  { id: 'large', label: 'Large', factor: 1.25 },
  { id: 'xlarge', label: 'Extra large', factor: 1.5 },
];

const STORAGE_KEY = 'dcrpulse.br.text-scale';
const CHANGE_EVENT = 'br-text-scale-changed';

export const getBrTextScale = (): BrTextScale => {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v && BR_TEXT_SCALES.some((s) => s.id === v)) return v as BrTextScale;
  } catch {
    // Private mode or quota errors fall back to the default.
  }
  return 'default';
};

export const setBrTextScale = (v: BrTextScale): void => {
  try {
    localStorage.setItem(STORAGE_KEY, v);
  } catch {
    // Still dispatch so the current session applies the change.
  }
  // The page root and the settings select live in different subtrees; a
  // window event keeps them in sync without threading state through props.
  window.dispatchEvent(new CustomEvent(CHANGE_EVENT));
};

export const brTextScaleFactor = (id: BrTextScale): number =>
  BR_TEXT_SCALES.find((s) => s.id === id)?.factor ?? 1;

export const useBrTextScale = (): { scale: BrTextScale; factor: number } => {
  const [scale, setScale] = useState<BrTextScale>(getBrTextScale);

  useEffect(() => {
    const sync = () => setScale(getBrTextScale());
    window.addEventListener(CHANGE_EVENT, sync);
    // The storage event fires in OTHER tabs of the same origin.
    window.addEventListener('storage', sync);
    return () => {
      window.removeEventListener(CHANGE_EVENT, sync);
      window.removeEventListener('storage', sync);
    };
  }, []);

  return { scale, factor: brTextScaleFactor(scale) };
};
