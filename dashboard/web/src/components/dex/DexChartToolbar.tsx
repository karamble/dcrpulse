// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Activity, CandlestickChart, LineChart, PenLine, Trash2 } from 'lucide-react';
import type { DexChartType } from './dexChartPrefs';

export interface IndicatorMeta {
  name: string;
  label: string;
  pane: 'main' | 'sub';
}

export interface DrawTool {
  name: string;
  label: string;
}

interface Props {
  chartType: DexChartType;
  onChartType: (t: DexChartType) => void;
  indicators: readonly IndicatorMeta[];
  active: Set<string>;
  onToggle: (name: string) => void;
  drawTools: readonly DrawTool[];
  onDraw: (name: string) => void;
  onClearDraw: () => void;
}

// DexChartToolbar is the presentational chart toolbar: a chart-type toggle, an
// indicators menu (toggle MA/EMA/BOLL on the price pane and VOL/MACD/RSI/KDJ as
// sub-panes), and a drawing-tools menu. All chart calls are done by the parent
// through the passed callbacks.
export const DexChartToolbar = ({
  chartType,
  onChartType,
  indicators,
  active,
  onToggle,
  drawTools,
  onDraw,
  onClearDraw,
}: Props) => {
  const [menu, setMenu] = useState<'indicators' | 'draw' | null>(null);

  const typeBtn = (t: DexChartType, Icon: typeof CandlestickChart, title: string) => (
    <button
      type="button"
      title={title}
      onClick={() => onChartType(t)}
      className={`p-1 rounded transition-colors ${
        chartType === t ? 'bg-muted/50 text-foreground' : 'text-muted-foreground hover:text-foreground'
      }`}
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );

  const menuBtn = (id: 'indicators' | 'draw', Icon: typeof Activity, label: string) => (
    <button
      type="button"
      onClick={() => setMenu((m) => (m === id ? null : id))}
      className={`flex items-center gap-1 px-1.5 py-1 rounded transition-colors ${
        menu === id ? 'bg-muted/50 text-foreground' : 'text-muted-foreground hover:text-foreground'
      }`}
    >
      <Icon className="h-3.5 w-3.5" />
      {label}
    </button>
  );

  return (
    <div className="relative flex items-center gap-1">
      <div className="flex items-center gap-0.5 mr-1">
        {typeBtn('candle_solid', CandlestickChart, 'Candles')}
        {typeBtn('area', LineChart, 'Line')}
      </div>

      {menuBtn('indicators', Activity, 'Indicators')}
      {menuBtn('draw', PenLine, 'Draw')}

      {menu && (
        <>
          <button type="button" aria-hidden className="fixed inset-0 z-10 cursor-default" onClick={() => setMenu(null)} />
          <div className="absolute right-0 top-7 z-20 w-44 rounded-xl border border-border/60 bg-card shadow-lg py-1">
            {menu === 'indicators' &&
              indicators.map((ind) => (
                <button
                  key={ind.name}
                  type="button"
                  onClick={() => onToggle(ind.name)}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-xs text-left hover:bg-muted/30"
                >
                  <span
                    className={`h-3 w-3 rounded-sm border flex items-center justify-center ${
                      active.has(ind.name) ? 'bg-primary border-primary' : 'border-border'
                    }`}
                  >
                    {active.has(ind.name) && <span className="h-1.5 w-1.5 rounded-[1px] bg-white" />}
                  </span>
                  {ind.label}
                </button>
              ))}
            {menu === 'draw' && (
              <>
                {drawTools.map((tool) => (
                  <button
                    key={tool.name}
                    type="button"
                    onClick={() => {
                      onDraw(tool.name);
                      setMenu(null);
                    }}
                    className="flex w-full items-center px-3 py-1.5 text-xs text-left hover:bg-muted/30"
                  >
                    {tool.label}
                  </button>
                ))}
                <div className="my-1 border-t border-border/40" />
                <button
                  type="button"
                  onClick={() => {
                    onClearDraw();
                    setMenu(null);
                  }}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-xs text-left text-destructive hover:bg-destructive/10"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  Clear drawings
                </button>
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
};
