// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// dexConfigCache persists the last successfully fetched market list per DEX
// host in localStorage so the trading view renders even while the DEX server
// can not be connected (the config fetch needs a live server connection).

import type { DexMarket } from '../../services/dcrdexApi';

export interface DexConfigCache {
  markets: DexMarket[];
  candleDurs: string[];
}

const keyFor = (host: string) => `dexConfigCache:${host}`;

export const loadDexConfigCache = (host: string): DexConfigCache | null => {
  try {
    const raw = localStorage.getItem(keyFor(host));
    if (!raw) return null;
    const c = JSON.parse(raw) as Partial<DexConfigCache>;
    if (!Array.isArray(c.markets) || !c.markets.length) return null;
    return {
      markets: c.markets,
      candleDurs: Array.isArray(c.candleDurs) ? c.candleDurs : [],
    };
  } catch {
    return null;
  }
};

export const saveDexConfigCache = (host: string, c: DexConfigCache) => {
  try {
    localStorage.setItem(keyFor(host), JSON.stringify(c));
  } catch {
    /* ignore */
  }
};
