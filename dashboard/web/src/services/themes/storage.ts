// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Browser cache of the active theme, used only for instant first paint. The
// server (global config) is the source of truth; this just avoids a flash of
// the default theme before the /api/themes response lands. The same key is
// read by the inline <script> in index.html (keep the key in sync).

import { Theme } from './types';

export const ACTIVE_THEME_CACHE_KEY = 'dcrpulse.theme.active';

export function cacheActiveTheme(theme: Theme): void {
  try {
    localStorage.setItem(ACTIVE_THEME_CACHE_KEY, JSON.stringify(theme));
  } catch {
    /* ignore quota / private-mode errors */
  }
}

export function readCachedActiveTheme(): Theme | null {
  try {
    const raw = localStorage.getItem(ACTIVE_THEME_CACHE_KEY);
    if (!raw) return null;
    const t = JSON.parse(raw) as Theme;
    return t && t.colors ? t : null;
  } catch {
    return null;
  }
}
