import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AlertCircle, Loader2, Network, RefreshCw } from 'lucide-react';
import {
  ChannelStatus,
  LightningChannel,
  getLightningChannels,
  subscribeLightningChannelEvents,
} from '../../../services/lightningApi';
import { ChannelRow } from './ChannelRow';

type Filter = 'all' | 'open' | 'pending' | 'closed';

const matchesFilter = (status: ChannelStatus, f: Filter): boolean => {
  switch (f) {
    case 'all':
      return true;
    case 'open':
      return status === 'open';
    case 'pending':
      return status.startsWith('pending-');
    case 'closed':
      return status === 'closed';
  }
};

export const ChannelList = () => {
  const navigate = useNavigate();
  const [channels, setChannels] = useState<LightningChannel[]>([]);
  const [filter, setFilter] = useState<Filter>('open');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setError(null);
    try {
      const r = await getLightningChannels();
      setChannels(r.channels || []);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load channels');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // Trailing debounce: a peer flapping emits a burst of events, and each one
    // otherwise costs a full channel list fetch.
    let timer: ReturnType<typeof setTimeout> | null = null;
    const reload = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(load, 500);
    };
    load();
    // The stream replays nothing, so anything that happened while it was down
    // is only picked up by refetching once it is back. The first connect is
    // already covered by the load above.
    let firstOpen = true;
    const unsubscribe = subscribeLightningChannelEvents(reload, {
      onOpen: () => {
        if (firstOpen) {
          firstOpen = false;
          return;
        }
        reload();
      },
    });
    return () => {
      if (timer) clearTimeout(timer);
      unsubscribe();
    };
  }, []);

  const visible = channels.filter((c) => matchesFilter(c.status, filter));

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-4">
      <div className="flex items-center gap-2 flex-wrap">
        <Network className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold flex-1">Channels</h3>
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as Filter)}
          className="px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
        >
          <option value="all">All</option>
          <option value="open">Open</option>
          <option value="pending">Pending</option>
          <option value="closed">Closed</option>
        </select>
        <button
          onClick={load}
          disabled={loading}
          className="inline-flex items-center gap-2 px-3 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="flex items-center gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}

      {loading && channels.length === 0 ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading channels…
        </div>
      ) : visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No channels {filter !== 'all' && `(${filter})`} yet.
        </p>
      ) : (
        <ul className="space-y-2">
          {visible.map((c) => (
            <li key={c.channelPoint}>
              <ChannelRow
                channel={c}
                onSelect={(cp) => navigate(`/wallet/lightning/channels/${encodeURIComponent(cp)}`)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};
