// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, Settings } from 'lucide-react';
import type { DexMarket, MMCexStatus } from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { CexIcon, CEX_DISPLAY, SUPPORTED_CEXES } from './CexIcon';
import { DexMMCexConfigForm } from './DexMMCexConfigForm';
import { BOT_TYPES, cexSupportsMarket, cexesSupportingMarket, needsCex, type BotType } from './dexMMConfig';

// A CEX tile's state for the chosen market: it can need keys, be disconnected,
// be configured-but-unable-to-arb the pair, or be ready to select.
type CexState = 'needsconfig' | 'disconnected' | 'unavailable' | 'ok';

// DexMMBotTypeSelector is step 2 of the wizard: pick the bot strategy and, for
// arbitrage strategies, the centralized exchange to use. Arb modes and CEX tiles
// are gated by whether a configured CEX can arbitrage the chosen market, mirroring
// v1.0.6 showBotTypeForm + setCEXAvailability.
export const DexMMBotTypeSelector = ({
  market,
  cexes,
  initialBotType,
  initialCex,
  onContinue,
  onChangeMarket,
  onCexSaved,
}: {
  market: DexMarket;
  cexes: Record<string, MMCexStatus>;
  initialBotType?: BotType;
  initialCex?: string;
  onContinue: (botType: BotType, cexName?: string) => void;
  onChangeMarket: () => void;
  onCexSaved: () => void;
}) => {
  const [botType, setBotType] = useState<BotType>(initialBotType ?? 'basicmm');
  const [cexName, setCexName] = useState<string>(initialCex ?? '');
  const [configuring, setConfiguring] = useState<string | null>(null);

  const supporting = useMemo(
    () => cexesSupportingMarket(cexes, market.baseID, market.quoteID),
    [cexes, market.baseID, market.quoteID],
  );
  const arbEnabled = supporting.length > 0;
  const arb = needsCex(botType);

  const tileState = (c: string): CexState => {
    const st = cexes[c];
    if (!st?.config) return 'needsconfig';
    if (st.connectErr) return 'disconnected';
    if (!cexSupportsMarket(st, market.baseID, market.quoteID)) return 'unavailable';
    return 'ok';
  };

  // If an arb mode is selected but no CEX can arb this market, fall back to basic.
  useEffect(() => {
    if (needsCex(botType) && !arbEnabled) setBotType('basicmm');
  }, [botType, arbEnabled]);

  // Auto-select a supporting CEX when an arb mode is active.
  useEffect(() => {
    if (!needsCex(botType) || !arbEnabled) return;
    if (!cexName || tileState(cexName) !== 'ok') setCexName(supporting[0]);
  }, [botType, arbEnabled, supporting]);

  const selectedOk = !!cexName && tileState(cexName) === 'ok';
  const canContinue = !arb || selectedOk;

  const handleTileClick = (c: string) => {
    const st = tileState(c);
    if (st === 'needsconfig' || st === 'disconnected') {
      setConfiguring(c);
      return;
    }
    if (st === 'ok') setCexName(c);
  };

  return (
    <div className="space-y-4 max-w-2xl">
      <button
        type="button"
        onClick={onChangeMarket}
        className="flex items-center gap-2 text-sm hover:bg-muted/10 rounded-lg px-2 py-1 -ml-2"
      >
        <span className="flex -space-x-1.5">
          <CoinIcon symbol={market.base} className="h-5 w-5 ring-1 ring-card" />
          <CoinIcon symbol={market.quote} className="h-5 w-5 ring-1 ring-card" />
        </span>
        <span className="font-medium">
          {market.base}/{market.quote}
        </span>
        <span className="text-xs text-muted-foreground">change market</span>
      </button>

      <div className="grid sm:grid-cols-3 gap-2">
        {BOT_TYPES.map((b) => {
          const disabled = b.cex && !arbEnabled;
          return (
            <button
              key={b.id}
              type="button"
              disabled={disabled}
              onClick={() => setBotType(b.id)}
              className={`text-left p-3 rounded-lg border transition-colors disabled:opacity-40 ${
                botType === b.id ? 'border-primary bg-muted/20' : 'border-border hover:bg-muted/10'
              }`}
            >
              <div className="text-sm font-medium">{b.label}</div>
              <div className="text-[11px] text-muted-foreground mt-0.5">{b.desc}</div>
            </button>
          );
        })}
      </div>

      {arb && (
        <div className="space-y-2">
          <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">Exchange</div>
          <div className="flex flex-wrap gap-2">
            {SUPPORTED_CEXES.map((c) => {
              const st = tileState(c);
              const selected = cexName === c;
              const label =
                st === 'needsconfig'
                  ? 'set keys'
                  : st === 'disconnected'
                    ? 'fix keys'
                    : st === 'unavailable'
                      ? 'market not available'
                      : cexes[c]?.connected
                        ? 'connected'
                        : 'configured';
              return (
                <div
                  key={c}
                  onClick={() => handleTileClick(c)}
                  className={`relative flex flex-col items-center gap-1 px-4 py-3 rounded-lg border transition-colors ${
                    st === 'unavailable' ? 'opacity-40 cursor-default' : 'cursor-pointer'
                  } ${selected ? 'border-primary bg-muted/20' : 'border-border hover:bg-muted/10'}`}
                >
                  {cexes[c]?.config && (
                    <button
                      type="button"
                      title="Reconfigure keys"
                      onClick={(e) => {
                        e.stopPropagation();
                        setConfiguring(c);
                      }}
                      className="absolute top-1 right-1 p-1 rounded hover:bg-muted/30 text-muted-foreground"
                    >
                      <Settings className="h-3 w-3" />
                    </button>
                  )}
                  <CexIcon name={c} className={`h-7 w-7 ${st === 'unavailable' ? 'grayscale' : ''}`} />
                  <span className="text-sm">{CEX_DISPLAY[c] || c}</span>
                  <span
                    className={`text-[10px] ${
                      st === 'disconnected'
                        ? 'text-destructive'
                        : st === 'ok'
                          ? 'text-success'
                          : 'text-muted-foreground'
                    }`}
                  >
                    {label}
                  </span>
                </div>
              );
            })}
          </div>
          {configuring && (
            <div className="mt-2 p-3 rounded-lg border border-border/50 bg-background/40">
              <DexMMCexConfigForm
                onSaved={() => {
                  setConfiguring(null);
                  onCexSaved();
                }}
              />
            </div>
          )}
        </div>
      )}

      {!arbEnabled && (
        <div className="text-xs text-muted-foreground">
          No configured exchange can arbitrage {market.base}/{market.quote}. Configure a supporting exchange, or use the
          basic market maker.
        </div>
      )}

      <div>
        <button
          type="button"
          disabled={!canContinue}
          onClick={() => onContinue(botType, arb ? cexName : undefined)}
          className="flex items-center gap-1.5 px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
        >
          Continue <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
};
