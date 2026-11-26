// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Activity, ArrowDownLeft, HardDrive, Link2, Loader2, Repeat, Wifi, WifiOff } from 'lucide-react';
import {
  LightningPeers,
  getLightningPeers,
  reconnectLightningPeer,
} from '../../services/lightningApi';
import { OnionIcon } from './OnionIcon';
import { StatusPill } from './StatusPill';
import { formatBytes } from '../../utils/bytes';
import { toYMDTime } from '../../utils/date';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';

const shortKey = (k: string) => (k.length > 20 ? `${k.slice(0, 10)}...${k.slice(-6)}` : k);

// PeersCard lists dcrlnd's connected peers and the channel peers that are not
// connected, which can be reconnected at their channel-graph addresses.
export const PeersCard = () => {
  const [data, setData] = useState<LightningPeers | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [rowError, setRowError] = useState<{ pubkey: string; text: string } | null>(null);

  const load = async () => {
    try {
      setData(await getLightningPeers());
      setError(null);
    } catch (err) {
      setError(apiError(err, 'Failed to load Lightning peers'));
    }
  };

  useVisiblePoll(load, 15000);

  const reconnect = async (pubkey: string) => {
    setBusy(pubkey);
    setRowError(null);
    try {
      await reconnectLightningPeer(pubkey);
      await load();
    } catch (err) {
      setRowError({ pubkey, text: apiError(err, 'Reconnect failed') });
    } finally {
      setBusy(null);
    }
  };

  const peers = data?.peers ?? [];
  const offline = data?.offlineChannelPeers ?? [];

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Wifi className="h-6 w-6 text-primary" />
        </div>
        <div className="flex-1">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Peers</h3>
            <span className="px-3 py-1 rounded-md text-sm font-medium bg-primary/10 text-primary border border-primary/20">
              {peers.length} connected
            </span>
          </div>
          <p className="text-sm text-muted-foreground">Lightning network connections</p>
        </div>
      </div>

      {error && <p className="mb-3 text-sm text-destructive">{error}</p>}

      {!data && !error ? (
        <div className="text-sm text-muted-foreground flex items-center gap-2">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading peers…
        </div>
      ) : (
        <div className="space-y-6">
          <div className="max-h-[400px] overflow-y-auto space-y-3">
            {peers.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">No peers connected</div>
            ) : (
              peers.map((p) => (
                <div key={p.pubkey} className="p-4 rounded-lg bg-muted/30 border border-border/30">
                  <div className="flex items-center gap-3 min-w-0">
                    <div
                      className={`shrink-0 p-2 rounded-lg border ${
                        p.tor ? 'bg-[#7D4698]/10 border-[#7D4698]/30' : 'bg-success/10 border-success/20'
                      }`}
                    >
                      {p.tor ? (
                        <span title="Connected over Tor">
                          <OnionIcon className="h-4 w-4 text-[#7D4698]" />
                        </span>
                      ) : (
                        <Wifi className="h-4 w-4 text-success" />
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium break-all">{p.alias || shortKey(p.pubkey)}</p>
                        {p.hasChannel && (
                          <StatusPill tone="info" label="channel" icon={<Link2 className="h-3 w-3" />} />
                        )}
                        {p.inbound && (
                          <StatusPill tone="muted" label="inbound" icon={<ArrowDownLeft className="h-3 w-3" />} />
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground font-mono break-all mt-0.5">{p.address}</p>
                    </div>
                  </div>
                  <div className="grid grid-cols-3 gap-3 mt-3 text-xs">
                    <div className="flex items-center gap-1.5">
                      <Activity className="h-3 w-3 text-muted-foreground" />
                      <span className="text-muted-foreground">Ping:</span>
                      <span className="font-medium">
                        {p.pingMicros > 0 ? `${Math.round(p.pingMicros / 1000)} ms` : '-'}
                      </span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <HardDrive className="h-3 w-3 text-muted-foreground" />
                      <span className="text-muted-foreground">Traffic:</span>
                      <span className="font-medium">{formatBytes(p.bytesSent + p.bytesRecv)}</span>
                    </div>
                    {!!p.flapCount && (
                      <div
                        className="flex items-center gap-1.5"
                        title={p.lastFlapUnix ? `Last ${toYMDTime(p.lastFlapUnix * 1000)}` : undefined}
                      >
                        <Repeat className="h-3 w-3 text-muted-foreground" />
                        <span className="text-muted-foreground">Reconnects:</span>
                        <span className="font-medium">{p.flapCount}</span>
                      </div>
                    )}
                  </div>
                  {p.lastError && (
                    <p className="mt-2 text-xs text-muted-foreground break-all">
                      Last error
                      {p.lastErrorUnix ? ` (${toYMDTime(p.lastErrorUnix * 1000)})` : ''}: {p.lastError}
                    </p>
                  )}
                </div>
              ))
            )}
          </div>

          {offline.length > 0 && (
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-sm font-medium text-warning">
                <WifiOff className="h-4 w-4" />
                Channel peers not connected
              </div>
              {offline.map((p) => (
                <div key={p.pubkey} className="p-4 rounded-lg bg-muted/10 border border-warning/30">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium break-all">{p.alias || shortKey(p.pubkey)}</p>
                      <p className="text-xs text-muted-foreground font-mono break-all mt-0.5">
                        {shortKey(p.pubkey)} · {p.channels} {p.channels === 1 ? 'channel' : 'channels'}
                      </p>
                      <p className="text-xs text-muted-foreground font-mono break-all mt-0.5">
                        {p.addresses.length > 0 ? p.addresses.join(', ') : 'No address in the channel graph'}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => reconnect(p.pubkey)}
                      disabled={busy !== null || p.addresses.length === 0}
                      className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm bg-primary/10 text-primary border border-primary/20 hover:bg-primary/20 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {busy === p.pubkey && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                      {busy === p.pubkey ? 'Reconnecting…' : 'Reconnect'}
                    </button>
                  </div>
                  {rowError?.pubkey === p.pubkey && (
                    <p className="mt-2 text-xs text-destructive break-all">{rowError.text}</p>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
