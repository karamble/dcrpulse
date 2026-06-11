// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexOrderOption } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';

const isTrue = (v: string) => v === 'true';

// DexMMWalletOptions renders one asset's market-maker funding options (bisonw's
// multifundingopts: the multisplit toggle and its split-buffer slider), mirroring
// the per-asset Wallet Options pane on the bisonw bot-config page. multisplit lets
// the bot pre-size a split funding transaction, so it can fund orders even when
// the balance sits in UTXOs larger than the bot's per-market allocation.
export const DexMMWalletOptions = ({
  assetSymbol,
  opts,
  isQuote,
  value,
  onChange,
}: {
  assetSymbol: string;
  opts: DexOrderOption[];
  isQuote: boolean;
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
}) => {
  const visible = opts.filter((o) => isQuote || !o.quoteAssetOnly);
  if (visible.length === 0) return null;
  const set = (key: string, v: string) => onChange({ ...value, [key]: v });

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3 space-y-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <CoinIcon symbol={assetSymbol} className="h-4 w-4" />
        {assetSymbol.toUpperCase()}
      </div>
      {visible.map((o) => {
        if (o.dependsOn && !isTrue(value[o.dependsOn] ?? '')) return null;
        const cur = value[o.key] ?? o.default;
        if (o.isBoolean) {
          return (
            <label key={o.key} className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={isTrue(cur)}
                onChange={(e) => set(o.key, e.target.checked ? 'true' : 'false')}
                className="mt-0.5"
              />
              <span>
                {o.displayName}
                {o.description && <span className="text-muted-foreground"> - {o.description}</span>}
              </span>
            </label>
          );
        }
        if (o.xyRange) {
          const { start, end, xUnit } = o.xyRange;
          return (
            <div key={o.key} className="space-y-1">
              <div className="flex items-center justify-between">
                <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{o.displayName}</span>
                <span className="font-mono text-sm">
                  {cur}
                  {xUnit}
                </span>
              </div>
              <input
                type="range"
                min={start.x}
                max={end.x}
                step={1}
                value={cur}
                onChange={(e) => set(o.key, e.target.value)}
                className="w-full accent-primary"
              />
              {o.description && <div className="text-[10px] text-muted-foreground">{o.description}</div>}
            </div>
          );
        }
        return null;
      })}
    </div>
  );
};
