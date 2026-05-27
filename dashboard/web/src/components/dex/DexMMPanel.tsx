// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Bot, KeyRound, Pencil, Play, Plus, Square, Trash2 } from 'lucide-react';
import {
  getDexConfig,
  removeMMBotConfig,
  startMMBot,
  stopMMBot,
  type DexMarket,
  type MMAllocation,
  type MMBotStatus,
} from '../../services/dcrdexApi';
import { useMMRefresh, useMMStatus } from './DexLiveProvider';
import { CoinIcon } from './CoinIcon';
import { CexIcon, CEX_DISPLAY, SUPPORTED_CEXES } from './CexIcon';
import { DexMMActivity } from './DexMMActivity';
import { DexMMWizard } from './DexMMWizard';
import { DexMMFundingDialog } from './DexMMFundingDialog';
import { DexMMCexConfigForm } from './DexMMCexConfigForm';
import { botTypeOf, needsCex } from './dexMMConfig';

const HOST = 'dex.decred.org:7232';

const botKind = (b: MMBotStatus): string =>
  b.config.simpleArbConfig ? 'Simple arb' : b.config.arbMarketMakingConfig ? 'Arb market maker' : 'Basic market maker';

type View = 'list' | 'new' | 'edit' | 'cex';

// DexMMPanel is the Market Maker tab: a CEX-credentials section, the list of
// configured bots with start/stop/edit/delete, and the bot config form. Bot and
// CEX state are read from the shared, live market-making status; bot start is a
// fund-spending action gated behind an explicit confirmation.
export const DexMMPanel = () => {
  const status = useMMStatus();
  const refresh = useMMRefresh();
  const [markets, setMarkets] = useState<DexMarket[]>([]);
  const [marketsLoaded, setMarketsLoaded] = useState(false);
  const [view, setView] = useState<View>('list');
  const [editBot, setEditBot] = useState<MMBotStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [startBot, setStartBot] = useState<MMBotStatus | null>(null);
  const [startBusy, setStartBusy] = useState(false);

  useEffect(() => {
    getDexConfig(HOST)
      .then((c) => setMarkets(c.markets))
      .catch(() => {})
      .finally(() => setMarketsLoaded(true));
  }, []);

  const bots = status?.bots ?? [];
  const cexes = status?.cexes ?? {};
  const key = (b: MMBotStatus) => `${b.config.baseID}-${b.config.quoteID}`;

  const act = async (k: string, fn: () => Promise<void>) => {
    setBusyKey(k);
    setErr(null);
    try {
      await fn();
      refresh();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Action failed');
    } finally {
      setBusyKey(null);
    }
  };

  const confirmStart = async (alloc: MMAllocation) => {
    if (!startBot) return;
    setStartBusy(true);
    setErr(null);
    try {
      await startMMBot({ host: startBot.config.host, baseID: startBot.config.baseID, quoteID: startBot.config.quoteID, alloc });
      refresh();
      setStartBot(null);
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Action failed');
    } finally {
      setStartBusy(false);
    }
  };

  const marketLabel = (b: MMBotStatus) => {
    const m = markets.find((mk) => mk.baseID === b.config.baseID && mk.quoteID === b.config.quoteID);
    return m ? `${m.base}/${m.quote}` : `${b.config.baseID}/${b.config.quoteID}`;
  };

  if (!marketsLoaded || !status) {
    return (
      <div className="min-h-[30vh] flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    );
  }

  if (view === 'new' || view === 'edit') {
    return (
      <div className="px-3 lg:px-4">
        <h2 className="font-semibold mb-3 flex items-center gap-2">
          <Bot className="h-5 w-5 text-primary" />
          {view === 'edit' ? 'Edit bot' : 'New market-maker bot'}
        </h2>
        <DexMMWizard
          host={HOST}
          markets={markets}
          cexes={cexes}
          editBot={view === 'edit' ? editBot ?? undefined : undefined}
          refresh={refresh}
          onCexSaved={refresh}
          onDone={() => {
            setView('list');
            setEditBot(null);
          }}
        />
      </div>
    );
  }

  return (
    <div className="px-3 lg:px-4 space-y-4">
      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      <section className="rounded-xl border border-border/60 bg-card p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-semibold flex items-center gap-2">
            <KeyRound className="h-4 w-4 text-primary" />
            Exchanges
          </h3>
          <button
            type="button"
            onClick={() => setView(view === 'cex' ? 'list' : 'cex')}
            className="text-xs px-2.5 py-1 rounded-lg border border-border hover:bg-muted/10"
          >
            {view === 'cex' ? 'Close' : 'Configure keys'}
          </button>
        </div>
        <div className="flex flex-wrap gap-3">
          {SUPPORTED_CEXES.map((c) => {
            const st = cexes[c];
            const configured = !!st?.config;
            return (
              <div key={c} className="flex items-center gap-2 text-sm">
                <CexIcon name={c} className="h-5 w-5" />
                <span>{CEX_DISPLAY[c] || c}</span>
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    st?.connected ? 'bg-success' : configured ? 'bg-warning' : 'bg-muted-foreground/40'
                  }`}
                />
                <span className="text-xs text-muted-foreground">
                  {st?.connected ? 'connected' : configured ? 'configured' : 'not set'}
                </span>
              </div>
            );
          })}
        </div>
        {view === 'cex' && (
          <div className="mt-4 pt-4 border-t border-border/40">
            <DexMMCexConfigForm onSaved={refresh} />
          </div>
        )}
      </section>

      <section className="rounded-xl border border-border/60 bg-card">
        <div className="flex items-center justify-between p-4 border-b border-border/50">
          <h3 className="font-semibold flex items-center gap-2">
            <Bot className="h-4 w-4 text-primary" />
            Bots
          </h3>
          <button
            type="button"
            onClick={() => setView('new')}
            disabled={!markets.length}
            className="flex items-center gap-1.5 text-sm px-3 py-1.5 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            <Plus className="h-4 w-4" />
            New bot
          </button>
        </div>

        {bots.length === 0 ? (
          <div className="px-4 py-8 text-sm text-muted-foreground text-center">No bots configured yet.</div>
        ) : (
          <div className="divide-y divide-border/40">
            {bots.map((b) => {
              const k = key(b);
              const busy = busyKey === k;
              return (
                <div key={k} className="p-4 space-y-2">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2 min-w-0">
                      {(() => {
                        const m = markets.find((mk) => mk.baseID === b.config.baseID && mk.quoteID === b.config.quoteID);
                        return m ? (
                          <span className="flex -space-x-1.5 shrink-0">
                            <CoinIcon symbol={m.base} className="h-5 w-5 ring-1 ring-card" />
                            <CoinIcon symbol={m.quote} className="h-5 w-5 ring-1 ring-card" />
                          </span>
                        ) : null;
                      })()}
                      <div className="min-w-0">
                        <div className="font-medium">{marketLabel(b)}</div>
                        <div className="text-[11px] text-muted-foreground flex items-center gap-1.5">
                          {botKind(b)}
                          {b.config.cexName && (
                            <>
                              <span>via</span>
                              <CexIcon name={b.config.cexName} className="h-3.5 w-3.5" />
                            </>
                          )}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      {b.running ? (
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => act(k, () => stopMMBot(b.config.host, b.config.baseID, b.config.quoteID))}
                          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-border hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                        >
                          <Square className="h-3.5 w-3.5" /> Stop
                        </button>
                      ) : (
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() => setStartBot(b)}
                          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg border border-border hover:bg-success/10 hover:text-success disabled:opacity-50"
                        >
                          <Play className="h-3.5 w-3.5" /> Start
                        </button>
                      )}
                      {!b.running && (
                        <>
                          <button
                            type="button"
                            title="Edit"
                            onClick={() => {
                              setEditBot(b);
                              setView('edit');
                            }}
                            className="p-1.5 rounded-lg hover:bg-muted/20 text-muted-foreground"
                          >
                            <Pencil className="h-4 w-4" />
                          </button>
                          <button
                            type="button"
                            title="Delete"
                            disabled={busy}
                            onClick={() =>
                              window.confirm('Delete this bot config?') &&
                              act(k, () => removeMMBotConfig(b.config.host, b.config.baseID, b.config.quoteID))
                            }
                            className="p-1.5 rounded-lg hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </>
                      )}
                    </div>
                  </div>

                  {b.running && b.runStats && (
                    <div className="rounded-lg bg-background/40 border border-border/40">
                      <DexMMActivity bot={b} />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </section>

      {startBot &&
        (() => {
          const m = markets.find((mk) => mk.baseID === startBot.config.baseID && mk.quoteID === startBot.config.quoteID);
          if (!m) return null;
          return (
            <DexMMFundingDialog
              market={m}
              needsCex={needsCex(botTypeOf(startBot.config))}
              busy={startBusy}
              onConfirm={confirmStart}
              onCancel={() => setStartBot(null)}
            />
          );
        })()}
    </div>
  );
};
