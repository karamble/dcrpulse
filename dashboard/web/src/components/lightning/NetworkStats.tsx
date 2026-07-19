import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  AlertCircle,
  ArrowDownToLine,
  ArrowUpToLine,
  GitBranch,
  Layers,
  Loader2,
  Network,
  Plug,
  Sigma,
  Users,
  Wallet,
} from 'lucide-react';
import {
  LightningChannel,
  LightningNetworkPanel,
  TopLightningNode,
  getLightningChannels,
  getLightningNetwork,
} from '../../services/lightningApi';

const atomsToDcr = (atoms: number) => (atoms / 1e8).toFixed(8);
const truncate = (s: string, n = 10) =>
  s.length > 2 * n + 3 ? `${s.slice(0, n)}...${s.slice(-n)}` : s;

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  sub?: string;
}

const StatCard = ({ icon, label, value, sub }: StatCardProps) => (
  <div className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-1">
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      {icon}
      <span>{label}</span>
    </div>
    <div className="text-lg font-semibold text-foreground break-all">{value}</div>
    {sub && <div className="text-xs text-muted-foreground">{sub}</div>}
  </div>
);

interface PeerChannelAgg {
  local: number;
  remote: number;
  capacity: number;
  anyOpen: boolean;
}

export const NetworkStats = () => {
  const navigate = useNavigate();
  const [panel, setPanel] = useState<LightningNetworkPanel | null>(null);
  const [channels, setChannels] = useState<LightningChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const openChannelTo = (n: TopLightningNode) => {
    const uri = n.address ? `${n.pubkey}@${n.address}` : n.pubkey;
    navigate(`/wallet/lightning/channels?peer=${encodeURIComponent(uri)}`);
  };

  const load = async () => {
    getLightningChannels()
      .then((r) => setChannels(r.channels || []))
      .catch(() => setChannels([]));
    try {
      const p = await getLightningNetwork();
      setPanel(p);
      setError(null);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load network stats');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  if (loading && !panel) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading network stats...
      </div>
    );
  }

  if (error && !panel) {
    return (
      <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
        <AlertCircle className="h-4 w-4" />
        {error}
      </div>
    );
  }

  if (!panel) return null;

  const info = panel.info;

  const byPeer = new Map<string, PeerChannelAgg>();
  for (const c of channels) {
    if (c.status === 'closed') continue;
    const agg = byPeer.get(c.remotePubkey) || {
      local: 0,
      remote: 0,
      capacity: 0,
      anyOpen: false,
    };
    agg.local += c.localBalance;
    agg.remote += c.remoteBalance;
    agg.capacity += c.capacity;
    if (c.status === 'open') agg.anyOpen = true;
    byPeer.set(c.remotePubkey, agg);
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4">
        <StatCard
          icon={<Users className="h-3.5 w-3.5" />}
          label="Nodes"
          value={info.numNodes.toLocaleString()}
        />
        <StatCard
          icon={<Network className="h-3.5 w-3.5" />}
          label="Channels"
          value={info.numChannels.toLocaleString()}
        />
        <StatCard
          icon={<Wallet className="h-3.5 w-3.5" />}
          label="Total capacity"
          value={`${atomsToDcr(info.totalNetworkCapacity)} DCR`}
        />
        <StatCard
          icon={<Sigma className="h-3.5 w-3.5" />}
          label="Avg channel size"
          value={`${atomsToDcr(info.avgChannelSize)} DCR`}
        />
        <StatCard
          icon={<Layers className="h-3.5 w-3.5" />}
          label="Median channel size"
          value={`${atomsToDcr(info.medianChannelSize)} DCR`}
        />
        <StatCard
          icon={<GitBranch className="h-3.5 w-3.5" />}
          label="Graph diameter"
          value={`${info.graphDiameter} hops`}
        />
        <StatCard
          icon={<ArrowDownToLine className="h-3.5 w-3.5" />}
          label="Smallest channel"
          value={`${atomsToDcr(info.minChannelSize)} DCR`}
        />
        <StatCard
          icon={<ArrowUpToLine className="h-3.5 w-3.5" />}
          label="Largest channel"
          value={`${atomsToDcr(info.maxChannelSize)} DCR`}
        />
        <StatCard
          icon={<GitBranch className="h-3.5 w-3.5" />}
          label="Avg out-degree"
          value={info.avgOutDegree.toFixed(2)}
        />
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
        <h3 className="text-base font-semibold">Top 10 nodes by capacity</h3>
        {panel.topNodes.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Graph not yet populated. Once peers gossip channel data, top nodes will appear here.
          </p>
        ) : (
          <div className="overflow-x-auto -mx-2">
            <table className="w-full text-sm min-w-[480px]">
              <thead className="text-xs text-muted-foreground border-b border-border/40">
                <tr>
                  <th className="px-2 py-2 text-left font-medium">#</th>
                  <th className="px-2 py-2 text-left font-medium">Alias</th>
                  <th className="px-2 py-2 text-left font-medium hidden sm:table-cell">Pubkey</th>
                  <th className="px-2 py-2 text-right font-medium">Channels</th>
                  <th className="px-2 py-2 text-right font-medium">Capacity</th>
                  <th className="px-2 py-2" />
                </tr>
              </thead>
              <tbody>
                {panel.topNodes.map((n, idx) => {
                  const agg = byPeer.get(n.pubkey);
                  return (
                  <tr key={n.pubkey} className="border-b border-border/20 last:border-none">
                    <td className="px-2 py-2 text-muted-foreground">{idx + 1}</td>
                    <td className="px-2 py-2 font-medium">
                      {n.alias || <span className="text-muted-foreground">(no alias)</span>}
                      {agg && (
                        <div
                          className="mt-1 max-w-[160px]"
                          title={`Your channel — Local ${atomsToDcr(agg.local)} DCR / Remote ${atomsToDcr(agg.remote)} DCR`}
                        >
                          <div className="h-1 rounded-full bg-muted/30 overflow-hidden">
                            <div
                              className="h-full bg-primary/60"
                              style={{
                                width: `${agg.capacity > 0 ? Math.max(0, Math.min(100, (agg.local / agg.capacity) * 100)) : 0}%`,
                              }}
                            />
                          </div>
                          <div className="mt-0.5 flex justify-between text-[10px] font-normal text-muted-foreground">
                            <span>{(agg.local / 1e8).toFixed(3)}</span>
                            {!agg.anyOpen && <span className="text-warning">pending</span>}
                            <span>{(agg.remote / 1e8).toFixed(3)}</span>
                          </div>
                        </div>
                      )}
                    </td>
                    <td className="px-2 py-2 font-mono text-xs text-muted-foreground hidden sm:table-cell">
                      {truncate(n.pubkey, 8)}
                    </td>
                    <td className="px-2 py-2 text-right">{n.numChannels.toLocaleString()}</td>
                    <td className="px-2 py-2 text-right font-mono text-xs">
                      {atomsToDcr(n.capacityAtoms)}
                    </td>
                    <td className="px-2 py-2 text-right">
                      <button
                        type="button"
                        onClick={() => openChannelTo(n)}
                        title="Open a channel with this node"
                        className="inline-flex items-center px-2 py-1 rounded hover:bg-muted/20 transition-colors text-muted-foreground hover:text-foreground"
                      >
                        <Plug className="h-4 w-4" />
                      </button>
                    </td>
                  </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};
