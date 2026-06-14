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
import { TicketPoolCard } from '../components/TicketPoolCard';
import { getDashboardData, DashboardData } from '../services/api';

interface NodeSync {
  status: string;
  syncProgress: number;
  syncPhase: string;
  syncMessage: string;
}

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
      if (dashboardData.nodeStatus.status !== 'syncing') setNodeSync(null);
    } catch (err: any) {
      console.error('Error fetching dashboard data:', err);
      if (err.response?.status === 503) {
        setError('RPC client not connected. Please configure the connection below.');
      } else {
        setError(err.message || 'Failed to fetch data');
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    // Auto-refresh every 30 seconds
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, []);

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

  return (
    <div className="space-y-6">
      {/* Error Message */}
      {error && (
        <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20 animate-fade-in">
          <p className="text-red-500 font-medium">{error}</p>
        </div>
      )}

      {/* Loading State */}
      {loading && !data && (
        <div className="text-center py-12">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent"></div>
          <p className="mt-4 text-muted-foreground">Loading dashboard data...</p>
        </div>
      )}

      {/* Node Status */}
      {data && (
        <NodeStatus
          status={(nodeSync?.status ?? data.nodeStatus.status) as any}
          syncProgress={nodeSync?.syncProgress ?? data.nodeStatus.syncProgress}
          version={data.nodeStatus.version}
          syncMessage={nodeSync?.syncMessage ?? data.nodeStatus.syncMessage}
        />
      )}

      {/* Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <MetricCard
          title="Circulating Supply"
          value={data ? (data.supplyInfo?.circulatingSupply || 'N/A') : 'Loading...'}
          subtitle="DCR of 21 million"
          icon={Coins}
          trend={{ value: "Max Supply: 21M DCR", isPositive: true }}
        />
        <MetricCard
          title="Network Peers"
          value={data ? (data.networkInfo?.peerCount ?? 'N/A') : 'Loading...'}
          subtitle="Connected nodes"
          icon={Users}
        />
        <MetricCard
          title="Block Height"
          value={data ? (data.blockchainInfo?.blockHeight?.toLocaleString() || 'N/A') : 'Loading...'}
          subtitle="Latest block"
          icon={Layers}
        />
        <MetricCard
          title="Network Hashrate"
          value={data ? (data.networkInfo?.hashrate || 'N/A') : 'Loading...'}
          subtitle="Total network power"
          icon={TrendingUp}
        />
      </div>

      {/* Additional Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <MetricCard
          title="Treasury Balance"
          value={data ? (data.supplyInfo?.treasurySize || 'N/A') : 'Loading...'}
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
          value={data ? (data.supplyInfo?.stakedSupply || 'N/A') : 'Loading...'}
          subtitle="DCR - Stakeholders Rule"
          icon={Lock}
          trend={data?.supplyInfo?.stakedPercent ? { 
            value: `${data.supplyInfo.stakedPercent.toFixed(1)}% of supply`, 
            isPositive: true 
          } : undefined}
        />
        <div className="md:col-span-2">
          <TicketPoolCard 
            data={data?.stakingInfo} 
            currentBlockHeight={data?.blockchainInfo?.blockHeight}
          />
        </div>
      </div>

      {/* Details Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <BlockchainInfo data={data?.blockchainInfo} />
        <StakingStats data={data?.stakingInfo} />
      </div>

      {/* Mempool Activity & Peers */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <MempoolActivity data={data?.mempoolInfo} />
        <PeersList peers={data?.peers} />
      </div>
    </div>
  );
};

