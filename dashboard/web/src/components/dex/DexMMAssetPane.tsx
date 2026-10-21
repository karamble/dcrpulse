// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexOrderOption } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { NumberSlider, Slider } from './DexMMInputs';
import { DexMMWalletOptions } from './DexMMWalletOptions';
import { ASSET_LIMITS, type AssetFactors, type BotType } from './dexMMConfig';

// TransferRange is the coin range bisonw's Minimum Transfer slider spans: the
// exchange's minimum withdrawal up to the larger of twice that and the commit.
export interface TransferRange {
  min: number;
  max: number;
  decimals: number;
}

// DexMMAssetPane is one asset's column of bisonw's settings page: the reserves
// it keeps, the minimum CEX transfer, and the wallet's funding options.
export const DexMMAssetPane = ({
  symbol,
  isQuote,
  botType,
  isToken,
  factors,
  onFactor,
  showTransfer,
  transfer,
  walletOpts,
  walletValue,
  onWalletChange,
}: {
  symbol: string;
  isQuote: boolean;
  botType: BotType;
  isToken: boolean;
  factors: AssetFactors;
  onFactor: (key: keyof AssetFactors, val: string) => void;
  showTransfer: boolean;
  transfer: TransferRange | null;
  walletOpts: DexOrderOption[];
  walletValue: Record<string, string>;
  onWalletChange: (v: Record<string, string>) => void;
}) => {
  const hasWalletOpts = walletOpts.some((o) => isQuote || !o.quoteAssetOnly);
  const factor = Number(factors.transferFactor) || 0;
  return (
    <section
      aria-label={`${symbol} settings`}
      className="rounded-lg border border-border/50 bg-background/40 p-3 space-y-4"
    >
      <div className="flex items-center gap-2 text-sm font-medium">
        <CoinIcon symbol={symbol} className="h-4 w-4" />
        {symbol.toUpperCase()}
      </div>
      {botType !== 'simplearb' && (
        <NumberSlider
          label="Order reserves"
          value={factors.orderReservesFactor}
          min={ASSET_LIMITS.orderReservesFactor.min}
          max={ASSET_LIMITS.orderReservesFactor.max}
          step={0.01}
          suffix="x"
          onChange={(v) => onFactor('orderReservesFactor', v)}
        />
      )}
      {isQuote && (
        <NumberSlider
          label="Slippage buffer"
          value={factors.slippageBufferFactor}
          min={ASSET_LIMITS.slippageBufferFactor.min}
          max={ASSET_LIMITS.slippageBufferFactor.max}
          step={0.01}
          suffix="x"
          onChange={(v) => onFactor('slippageBufferFactor', v)}
        />
      )}
      {isToken && (
        <NumberSlider
          label="Swap fee reserves"
          value={factors.swapFeeN}
          min={ASSET_LIMITS.swapFeeN.min}
          max={ASSET_LIMITS.swapFeeN.max}
          step={10}
          suffix="swaps"
          onChange={(v) => onFactor('swapFeeN', v)}
        />
      )}
      {showTransfer &&
        (transfer ? (
          <NumberSlider
            label="Minimum transfer"
            value={(transfer.min + factor * (transfer.max - transfer.min)).toFixed(transfer.decimals)}
            min={transfer.min}
            max={transfer.max}
            step={Number((10 ** -transfer.decimals).toFixed(transfer.decimals))}
            suffix={symbol.toUpperCase()}
            hint="The minimum amount that can be transferred."
            onChange={(v) => {
              const span = transfer.max - transfer.min;
              const f = span > 0 ? Math.min(1, Math.max(0, ((Number(v) || 0) - transfer.min) / span)) : 0;
              onFactor('transferFactor', String(Number(f.toFixed(4))));
            }}
          />
        ) : (
          <Slider
            label="Minimum transfer"
            value={factors.transferFactor}
            min={0}
            max={1}
            step={0.01}
            hint="The amount shows once the exchange's minimum withdrawal and the market report are known."
            onChange={(v) => onFactor('transferFactor', v)}
          />
        ))}
      <div className="space-y-2">
        <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">Wallet options</span>
        {hasWalletOpts ? (
          <DexMMWalletOptions
            assetSymbol={symbol}
            opts={walletOpts}
            isQuote={isQuote}
            value={walletValue}
            onChange={onWalletChange}
            embedded
          />
        ) : (
          <div className="text-xs text-muted-foreground">No settings available.</div>
        )}
      </div>
    </section>
  );
};
