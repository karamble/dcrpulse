import { ChevronRight } from 'lucide-react';
import { LightningChannel } from '../../../services/lightningApi';

const fmtDcr = (atoms?: number) => ((atoms || 0) / 1e8).toFixed(8);
const truncate = (s: string, n = 10) =>
  s.length > 2 * n + 3 ? `${s.slice(0, n)}…${s.slice(-n)}` : s;

const statusBadge = (
  status: LightningChannel['status'],
  active?: boolean,
  currentConfs?: number,
  requiredConfs?: number,
) => {
  switch (status) {
    case 'open':
      return active ? (
        <span className="px-2 py-0.5 rounded text-xs bg-success/20 text-success">Active</span>
      ) : (
        <span className="px-2 py-0.5 rounded text-xs bg-warning/20 text-warning">Inactive</span>
      );
    case 'pending-open': {
      const haveConfs =
        typeof requiredConfs === 'number' && requiredConfs > 0;
      const label = haveConfs
        ? `Pending open · ${currentConfs ?? 0}/${requiredConfs} confs`
        : 'Pending open';
      return <span className="px-2 py-0.5 rounded text-xs bg-warning/20 text-warning">{label}</span>;
    }
    case 'pending-close-coop':
      return <span className="px-2 py-0.5 rounded text-xs bg-warning/20 text-warning">Closing</span>;
    case 'pending-close-force':
      return <span className="px-2 py-0.5 rounded text-xs bg-destructive/20 text-destructive">Force-closing</span>;
    case 'pending-wait-close':
      return <span className="px-2 py-0.5 rounded text-xs bg-warning/20 text-warning">Waiting close</span>;
    case 'closed':
      return <span className="px-2 py-0.5 rounded text-xs bg-muted/30 text-muted-foreground">Closed</span>;
  }
};

interface Props {
  channel: LightningChannel;
  onSelect: (channelPoint: string) => void;
}

export const ChannelRow = ({ channel, onSelect }: Props) => {
  const localPct =
    channel.capacity > 0
      ? Math.max(0, Math.min(100, (channel.localBalance / channel.capacity) * 100))
      : 0;

  return (
    <button
      type="button"
      onClick={() => onSelect(channel.channelPoint)}
      className="w-full text-left p-3 rounded-lg bg-muted/10 border border-border/50 hover:bg-muted/20 transition-colors space-y-2"
    >
      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold">
            {channel.remoteAlias || truncate(channel.remotePubkey)}
          </div>
          <div className="text-xs text-muted-foreground font-mono">{truncate(channel.channelPoint)}</div>
        </div>
        {statusBadge(channel.status, channel.active, channel.currentConfs, channel.requiredConfs)}
        <ChevronRight className="h-4 w-4 text-muted-foreground" />
      </div>
      <div>
        <div className="text-xs text-muted-foreground flex justify-between">
          <span>Local {fmtDcr(channel.localBalance)} DCR</span>
          <span>Capacity {fmtDcr(channel.capacity)} DCR</span>
          <span>Remote {fmtDcr(channel.remoteBalance)} DCR</span>
        </div>
        <div className="mt-1 h-2 rounded-full bg-muted/30 overflow-hidden">
          <div
            className="h-full bg-primary/60"
            style={{ width: `${localPct}%` }}
          />
        </div>
      </div>
    </button>
  );
};
