// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { 
  Users, Layers, TrendingUp, Coins, Wallet, 
  Lock
} from 'lucide-react';
import { NodeStatus } from '../components/NodeStatus';
import { MetricCard } from '../components/MetricCard';
import { BlockchainInfo } from '../components/BlockchainInfo';
import { PeersList } from '../components/PeersList';
import { StakingStats } from '../components/StakingStats';
import { MempoolActivity } from '../components/MempoolActivity';
import { RecentAlertsCard } from '../components/alerts/RecentAlertsCard';
import { TicketPoolCard } from '../components/TicketPoolCard';
import { getDashboardData, DashboardData } from '../services/api';
import { useVisiblePoll } from '../hooks/useVisiblePoll';

interface NodeSync {
  status: string;
  syncProgress: number;
  syncPhase: string;
  syncMessage: string;
  startupNote?: string;
  startupLog?: string;
}

// Names for the section keys the backend reports in `degraded`.
const SECTION_LABELS: Record<string, string> = {
  nodeStatus: 'node status',
  blockchainInfo: 'blockchain',
  networkInfo: 'network',
  peers: 'peers',
  supplyInfo: 'supply',
  stakingInfo: 'staking',
  mempoolInfo: 'mempool',
};

// The backend sends DCR amounts as numbers, so grouping and precision are
// decided here. An absent amount is one dcrd could not supply.
const dcr = (amount: number | undefined, digits = 0) =>
  amount === undefined || amount === null
    ? 'N/A'
    : amount.toLocaleString(undefined, {
        minimumFractionDigits: digits,
        maximumFractionDigits: digits,
      });

export const NodeDashboard = () => {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Live dcrd sync progress, pushed over a WebSocket on each block-connected
  // notification (smoother than the 30s dashboard poll). Falls back to the
  // polled dashboard data when no snapshot has arrived.
  const [nodeSync, setNodeSync] = useState<NodeSync | null>(null);

  const fetchData = async () => {
    try {
      const dashboardData = await getDashboardData();
      setData(dashboardData);
      setError(null);
      // The 30s poll is authoritative for recovery: if it reports the node is no
      // longer syncing, drop any stale live sync frame so a missed/transient
      // WebSocket update cannot keep the progress bar pinned at 100% until a reload.
      // A degraded section comes back zeroed, not "not syncing", so it does not
      // outrank the live frame.
      if (
        !dashboardData.degraded?.includes('nodeStatus') &&
        dashboardData.nodeStatus.status !== 'syncing'
      ) {
        setNodeSync(null);
      }
    } catch (err: any) {
      console.error('Error fetching dashboard data:', err);
      if (err.response?.status === 503) {
        const serverMsg = typeof err.response.data === 'string' ? err.response.data.trim() : '';
        setError(serverMsg || 'RPC client not connected. Please configure the connection below.');
      } else {
        setError(err.message || 'Failed to fetch data');
      }
    } finally {
      setLoading(false);
    }
  };

  useVisiblePoll(fetchData, 30000);

  // Push-driven dcrd sync progress via WebSocket (reconnects with backoff).
  useEffect(() => {
    let ws: WebSocket | null = null;
    let cancelled = false;
    let retry = 1000;
    let timer: number | undefined;
    const connect = () => {
      if (cancelled) return;
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
      ws = new WebSocket(`${proto}://${window.location.host}/api/node/sync/stream`);
      ws.onopen = () => {
        retry = 1000;
        // Re-pull the authoritative status on every (re)connect so a stream that
        // reconnected after a restart reconciles immediately instead of waiting
        // for the next 30s poll.
        fetchData();
      };
      ws.onmessage = (e) => {
        try {
          const s = JSON.parse(e.data);
          if (s && typeof s.syncProgress === 'number' && s.status) setNodeSync(s);
        } catch {
          /* ignore non-JSON (ping) frames */
        }
      };
      ws.onclose = () => {
        if (cancelled) return;
        timer = window.setTimeout(connect, retry);
        retry = Math.min(retry * 2, 30000);
      };
      ws.onerror = () => {
        try {
          ws?.close();
        } catch {
          /* ignore */
        }
      };
    };
    connect();
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
      try {
        ws?.close();
      } catch {
        /* ignore */
      }
    };
  }, []);

  // A degraded section came back zeroed, so it renders as unavailable rather
  // than as a reading of zero.
  const stale = (section: string) => data?.degraded?.includes(section) ?? false;
  const metric = (section: string, value: string | number): string | number =>
    !data ? 'Loading...' : stale(section) ? 'Unavailable' : value;

  // The live frame leads, since the poll 503s while dcrd is not serving. When
  // the WebSocket cannot connect, that 503 body is the same startup explanation.
  const node = nodeSync ?? data?.nodeStatus;
  const nodeStage = node?.status || (error && !data ? 'starting' : '');
  const nodeDown = nodeStage === 'starting' || nodeStage === 'upgrading';

  return (
    <div className="space-y-6">
      {/* Error Message. Suppressed while the node card carries the same text. */}
      {error && !nodeDown && (
        <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20">
          <p className="text-red-500 font-medium">{error}</p>
        </div>
      )}

      {/* Partial failure: the rest of the page is still live */}
      {data?.degraded?.length ? (
        <div className="p-4 rounded-lg bg-amber-500/10 border border-amber-500/20">
          <p className="text-amber-500 font-medium">
            Some data could not be read from the node:{' '}
            {data.degraded.map((s) => SECTION_LABELS[s] ?? s).join(', ')}. The rest of this
            page is current.
          </p>
        </div>
      ) : null}

      {/* Loading State */}
      {loading && !data && (
        <div className="text-center py-12">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent"></div>
          <p className="mt-4 text-muted-foreground">Loading dashboard data...</p>
        </div>
      )}

      {/* Node Status. Always rendered: the sync bar matters most in the states
          that used to unmount this card. */}
      {nodeStage && (
        <NodeStatus
          status={nodeStage}
          syncProgress={node?.syncProgress ?? 0}
          version={stale('nodeStatus') ? undefined : data?.nodeStatus.version}
          syncMessage={node?.syncMessage || 'Starting up'}
          startupNote={nodeSync?.startupNote ?? (error && !data ? error : undefined)}
          startupLog={nodeSync?.startupLog}
        />
      )}

      {/* Nothing behind these cards while dcrd is not serving, so explain the
          wait instead of rendering a grid of unavailable readings. */}
      {nodeDown && (
        <p className="text-sm text-muted-foreground">
          Node data will appear once dcrd finishes starting and answers RPC.
        </p>
      )}

      {!nodeDown && (
        <>
          {/* Metrics Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
            <MetricCard
              title="Circulating Supply"
              value={metric('supplyInfo', dcr(data?.supplyInfo?.circulatingSupply))}
              subtitle="DCR of 21 million"
              icon={Coins}
              trend={{ value: "Max Supply: 21M DCR", isPositive: true }}
            />
            <MetricCard
              title="Network Peers"
              // peerCount comes from the same RPC as the peers section, so a peer
              // failure means this count is not a real zero.
              value={metric('peers', data?.networkInfo?.peerCount ?? 'N/A')}
              subtitle="Connected nodes"
              icon={Users}
            />
            <MetricCard
              title="Block Height"
              value={metric('blockchainInfo', data?.blockchainInfo?.blockHeight?.toLocaleString() || 'N/A')}
              subtitle="Latest block"
              icon={Layers}
            />
            <MetricCard
              title="Network Hashrate"
              value={metric('networkInfo', data?.networkInfo?.hashrate || 'N/A')}
              subtitle="Total network power"
              icon={TrendingUp}
            />
          </div>

          {/* Additional Metrics Grid */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
            <MetricCard
              title="Treasury Balance"
              value={metric('supplyInfo', dcr(data?.supplyInfo?.treasurySize, 2))}
              subtitle={
                <>
                  DCR in treasury
                  <br />
                  Self-funded from block reward
                </>
              }
              icon={Wallet}
            />
            <MetricCard
              title="Supply Staked"
              value={metric('supplyInfo', dcr(data?.supplyInfo?.stakedSupply))}
              subtitle="DCR - Stakeholders Rule"
              icon={Lock}
              trend={!stale('supplyInfo') && data?.supplyInfo?.stakedPercent ? {
                value: `${data.supplyInfo.stakedPercent.toFixed(1)}% of supply`,
                isPositive: true
              } : undefined}
            />
            <div className="md:col-span-2">
              <TicketPoolCard
                data={stale('stakingInfo') ? undefined : data?.stakingInfo}
                currentBlockHeight={stale('blockchainInfo') ? undefined : data?.blockchainInfo?.blockHeight}
              />
            </div>
          </div>

          {/* Details Grid */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <BlockchainInfo data={stale('blockchainInfo') ? undefined : data?.blockchainInfo} />
            <StakingStats data={stale('stakingInfo') ? undefined : data?.stakingInfo} />
          </div>

          {/* Mempool Activity & Peers */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <MempoolActivity data={stale('mempoolInfo') ? undefined : data?.mempoolInfo} />
            <PeersList peers={stale('peers') ? undefined : data?.peers} />
          </div>
        </>
      )}

      <RecentAlertsCard />
    </div>
  );
};

