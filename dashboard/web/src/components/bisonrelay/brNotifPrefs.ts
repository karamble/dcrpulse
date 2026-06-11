// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';

// Per-type switches for the Bison Relay in-app notification indicators
// (unread bubbles, nav badge, feed activity dots), mirroring bruig's
// notification toggles. A per-browser preference in localStorage. The
// switches gate RENDERING only - unread accounting keeps running in the
// live provider, so re-enabling a switch shows the true unread state.

export interface BrNotifPrefs {
  dms: boolean;
  gcMessages: boolean;
  feedPosts: boolean;
}

const DEFAULT_PREFS: BrNotifPrefs = {
  dms: true,
  gcMessages: true,
  feedPosts: true,
};

const STORAGE_KEY = 'dcrpulse.br.notif-prefs';
const CHANGE_EVENT = 'br-notif-prefs-changed';

export const getBrNotifPrefs = (): BrNotifPrefs => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      // Merge over the defaults so prefs added later default to on.
      return { ...DEFAULT_PREFS, ...JSON.parse(raw) };
    }
  } catch {
    // Private mode, quota or parse errors fall back to the defaults.
  }
  return { ...DEFAULT_PREFS };
};

export const setBrNotifPrefs = (p: BrNotifPrefs): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(p));
  } catch {
    // Still dispatch so the current session applies the change.
  }
  window.dispatchEvent(new CustomEvent(CHANGE_EVENT));
};

export const useBrNotifPrefs = (): BrNotifPrefs => {
  const [prefs, setPrefs] = useState<BrNotifPrefs>(getBrNotifPrefs);

  useEffect(() => {
    const sync = () => setPrefs(getBrNotifPrefs());
    window.addEventListener(CHANGE_EVENT, sync);
    // The storage event fires in OTHER tabs of the same origin.
    window.addEventListener('storage', sync);
    return () => {
      window.removeEventListener(CHANGE_EVENT, sync);
      window.removeEventListener('storage', sync);
    };
  }, []);

  return prefs;
};
