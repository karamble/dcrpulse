// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// dexChartPrefs persists the trade-view chart toolbar selection (chart type and
// active indicators) in localStorage so it survives reloads.

export type DexChartType = 'candle_solid' | 'area';

export interface DexChartPrefs {
  chartType: DexChartType;
  indicators: string[];
}

const KEY = 'dexChartPrefs';
const DEFAULTS: DexChartPrefs = { chartType: 'candle_solid', indicators: ['VOL'] };

export const loadChartPrefs = (): DexChartPrefs => {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return { ...DEFAULTS };
    const p = JSON.parse(raw) as Partial<DexChartPrefs>;
    return {
      chartType: p.chartType === 'area' ? 'area' : 'candle_solid',
      indicators: Array.isArray(p.indicators) ? p.indicators : DEFAULTS.indicators,
    };
  } catch {
    return { ...DEFAULTS };
  }
};

export const saveChartPrefs = (p: DexChartPrefs) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(p));
  } catch {
    /* ignore */
  }
};
