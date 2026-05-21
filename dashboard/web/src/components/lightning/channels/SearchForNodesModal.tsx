import { useEffect, useState } from 'react';
import { Loader2, X } from 'lucide-react';
import { NodeMatch, searchLightningNodes } from '../../../services/lightningApi';

interface Props {
  onClose: () => void;
  onPick: (pubkey: string) => void;
}

export const SearchForNodesModal = ({ onClose, onPick }: Props) => {
  const [query, setQuery] = useState('');
  const [matches, setMatches] = useState<NodeMatch[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const id = window.setTimeout(async () => {
      setLoading(true);
      setError(null);
      try {
        const r = await searchLightningNodes(query);
        setMatches(r.matches);
      } catch (err: any) {
        const body = err?.response?.data;
        setError(typeof body === 'string' ? body : err?.message || 'Search failed');
      } finally {
        setLoading(false);
      }
    }, 250);
    return () => window.clearTimeout(id);
  }, [query]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-lg mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[80vh] flex flex-col">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold">Search nodes</h3>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="p-6 space-y-3 overflow-hidden flex flex-col flex-1">
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            autoFocus
            placeholder="alias or pubkey…"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
          />
          {error && <div className="text-sm text-destructive">{error}</div>}
          {loading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Querying graph…
            </div>
          ) : matches.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No matches. Graph may not be synced yet — until the wallet has peers gossiping
              channel updates, DescribeGraph is empty.
            </p>
          ) : (
            <ul className="overflow-y-auto flex-1 divide-y divide-border/40">
              {matches.map((m) => (
                <li key={m.pubkey}>
                  <button
                    type="button"
                    onClick={() => onPick(m.pubkey)}
                    className="w-full text-left py-2 hover:bg-muted/10 px-2 rounded"
                  >
                    <div className="text-sm font-medium">{m.alias || '(no alias)'}</div>
                    <div className="text-xs text-muted-foreground font-mono break-all">
                      {m.pubkey}
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
};
