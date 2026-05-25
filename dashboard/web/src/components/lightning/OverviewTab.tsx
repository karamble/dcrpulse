import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertCircle,
  ArrowDownLeft,
  ArrowUpRight,
  Info,
  Loader2,
  Network,
  Wallet,
  Zap,
} from 'lucide-react';
import {
  LightningActivity,
  LightningBalance,
  LightningInfo,
  getLightningActivity,
  getLightningBalance,
  getLightningInfo,
} from '../../services/lightningApi';
import { NetworkStats } from './NetworkStats';
import { StatCard, fmtDcr } from './StatCard';

export const OverviewTab = () => {
  const [info, setInfo] = useState<LightningInfo | null>(null);
  const [balance, setBalance] = useState<LightningBalance | null>(null);
  const [activity, setActivity] = useState<LightningActivity | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setError(null);
    try {
      const [i, b, a] = await Promise.all([
        getLightningInfo(),
        getLightningBalance(),
        getLightningActivity(),
      ]);
      setInfo(i);
      setBalance(b);
      setActivity(a);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load Lightning data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(load, 15000);
    return () => window.clearInterval(id);
  }, []);

  if (loading && !info) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading Lightning data…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {error && (
        <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      {info && (
        <div className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex flex-wrap items-center gap-3">
          <Zap className="h-5 w-5 text-warning shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="text-sm font-semibold">
              {info.alias || 'Lightning Node'}{' '}
              <span className="text-xs text-muted-foreground font-normal">{info.version}</span>
            </div>
            <div className="text-xs text-muted-foreground font-mono break-all">{info.identityPubkey}</div>
          </div>
          <div className="text-xs text-muted-foreground text-right">
            <div>
              Chain: {info.syncedToChain ? <span className="text-success">synced</span> : <span className="text-warning">syncing</span>}
            </div>
            <div>
              Graph: {info.syncedToGraph ? <span className="text-success">synced</span> : <span className="text-warning">syncing</span>}
            </div>
            <div>
              Peers:{' '}
              <span className={info.numPeers > 0 ? 'text-success' : 'text-warning'}>
                {info.numPeers}
              </span>
            </div>
          </div>
        </div>
      )}

      {info &&
        (!info.syncedToChain || !info.syncedToGraph) &&
        (() => {
          // Surface the underlying reason the "syncing" label is sticky.
          // With no peers, dcrlnd cannot receive the gossiped channel
          // graph, so syncedToGraph never flips. With peers but no
          // channels, the node is effectively idle; open one to start
          // routing.
          const noPeers = info.numPeers === 0;
          const noChannels =
            info.numActiveChannels === 0 && info.numPendingChannels === 0;
          if (!noPeers && !noChannels) return null;
          const msg = noPeers
            ? 'No Lightning peers connected. dcrlnd cannot sync the channel graph until at least one peer is reachable.'
            : 'Connected to peers, but no channels are open or pending. Open one to start sending and receiving payments.';
          return (
            <div className="p-4 rounded-xl bg-warning/10 border border-warning/30 text-sm flex items-start gap-3">
              <Info className="h-4 w-4 text-warning shrink-0 mt-0.5" />
              <div className="flex-1 min-w-0">
                <div>{msg}</div>
                <Link
                  to="/wallet/lightning/channels"
                  className="inline-block mt-1 text-xs text-primary hover:underline"
                >
                  Open a channel →
                </Link>
              </div>
            </div>
          );
        })()}

      {balance && (
        <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
          <StatCard
            icon={<Wallet className="h-3.5 w-3.5" />}
            label="On-chain confirmed"
            value={fmtDcr(balance.onChainConfirmed)}
          />
          <StatCard
            icon={<Wallet className="h-3.5 w-3.5" />}
            label="On-chain unconfirmed"
            value={fmtDcr(balance.onChainUnconfirmed)}
          />
          <StatCard
            icon={<Wallet className="h-3.5 w-3.5" />}
            label="On-chain total"
            value={fmtDcr(balance.onChainTotal)}
          />
          <StatCard
            icon={<Network className="h-3.5 w-3.5" />}
            label="Channel local"
            value={fmtDcr(balance.channelLocal)}
            sub={info ? `${info.numActiveChannels} active` : undefined}
          />
          <StatCard
            icon={<Network className="h-3.5 w-3.5" />}
            label="Channel remote"
            value={fmtDcr(balance.channelRemote)}
          />
          <StatCard
            icon={<Network className="h-3.5 w-3.5" />}
            label="Pending channels"
            value={fmtDcr(balance.channelPending)}
            sub={info ? `${info.numPendingChannels} pending` : undefined}
          />
        </div>
      )}

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
        <h3 className="text-lg font-semibold">Recent activity</h3>
        {!activity || activity.entries.length === 0 ? (
          <p className="text-sm text-muted-foreground">No Lightning activity yet.</p>
        ) : (
          <ul className="divide-y divide-border/40">
            {activity.entries.map((e, idx) => (
              <li key={idx} className="py-2 flex items-center gap-3 text-sm">
                {e.kind === 'invoice' ? (
                  <ArrowDownLeft className="h-4 w-4 text-success shrink-0" />
                ) : (
                  <ArrowUpRight className="h-4 w-4 text-primary shrink-0" />
                )}
                <div className="flex-1 min-w-0">
                  <div className="font-medium">
                    {e.kind === 'invoice' ? 'Invoice' : 'Payment'} · {e.state}
                  </div>
                  {e.memo && <div className="text-xs text-muted-foreground truncate">{e.memo}</div>}
                </div>
                <div className="text-xs text-muted-foreground shrink-0">
                  {new Date(e.timestamp * 1000).toLocaleString()}
                </div>
                <div className="text-sm font-mono shrink-0">{fmtDcr(e.amount)}</div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="border-t border-border/30 pt-6">
        <h2 className="text-lg font-semibold mb-4">Network statistics</h2>
        <NetworkStats />
      </div>
    </div>
  );
};
