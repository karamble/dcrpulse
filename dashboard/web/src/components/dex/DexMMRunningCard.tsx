// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Bot, ScrollText, Square } from 'lucide-react';
import { stopMMBot, type DexMarket, type MMBotStatus } from '../../services/dcrdexApi';
import { useMMRefresh } from './DexLiveProvider';
import { DexMMActivity, type AssetInfo } from './DexMMActivity';
import { DexMMRunLogs } from './DexMMRunLogs';

// DexMMRunningCard is the trade view's right-sidebar card shown in place of the
// order form when a market-maker bot is running on the selected market. Manual
// trading is blocked while a bot runs (mirroring bisonw's markets page), so this
// surfaces the bot's live stats plus a one-click stop. Stopping flips the shared
// MM status (this refresh and bisonw's runstats notification), which brings the
// order form back on its own.
export const DexMMRunningCard = ({
  bot,
  market,
  assetOf,
}: {
  bot: MMBotStatus;
  market?: DexMarket;
  assetOf?: AssetInfo;
}) => {
  const refreshMM = useMMRefresh();
  const [stopping, setStopping] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [showLogs, setShowLogs] = useState(false);
  const resolveAsset: AssetInfo = assetOf ?? ((id) => ({ symbol: `#${id}`, convFactor: 1e8 }));

  const stop = async () => {
    setStopping(true);
    setErr(null);
    try {
      await stopMMBot(bot.config.host, bot.config.baseID, bot.config.quoteID);
      refreshMM();
    } catch (e: any) {
      setErr((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'Failed to stop bot');
    } finally {
      setStopping(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between gap-2 px-3 py-2 border-b border-border/50">
        <div className="flex items-center gap-1.5 text-sm font-medium">
          <Bot className="h-4 w-4 text-primary" />
          Market maker running
        </div>
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => setShowLogs(true)}
            className="flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg border border-border hover:bg-muted/10"
          >
            <ScrollText className="h-3 w-3" /> Logs
          </button>
          <button
            type="button"
            disabled={stopping}
            onClick={stop}
            className="flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg border border-border hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
          >
            <Square className="h-3 w-3" /> {stopping ? 'Stopping...' : 'Stop'}
          </button>
        </div>
      </div>

      <div className="px-3 py-1.5 text-[11px] text-muted-foreground bg-muted/10 border-b border-border/40">
        Manual trading is disabled while a bot is running on this market.
      </div>

      {err && <div className="px-3 py-1.5 text-[11px] text-destructive">{err}</div>}

      <DexMMActivity bot={bot} compact market={market} assetOf={assetOf} />

      {showLogs && (
        <DexMMRunLogs bot={bot} market={market} assetOf={resolveAsset} onClose={() => setShowLogs(false)} />
      )}
    </div>
  );
};
