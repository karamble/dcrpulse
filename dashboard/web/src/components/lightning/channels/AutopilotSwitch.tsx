import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Bot, Loader2, Plug } from 'lucide-react';
import {
  TopLightningNode,
  getLightningAutopilot,
  getLightningAutopilotScores,
  getLightningNetwork,
  setLightningAutopilot,
} from '../../../services/lightningApi';
import { ScoreMeter } from '../ScoreMeter';

const truncate = (s: string, n = 10) =>
  s.length > 2 * n + 3 ? `${s.slice(0, n)}...${s.slice(-n)}` : s;

interface Suggestion {
  node: TopLightningNode;
  score: number;
}

export const AutopilotSwitch = () => {
  const navigate = useNavigate();
  const [active, setActive] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [heuristic, setHeuristic] = useState('');

  useEffect(() => {
    getLightningAutopilot()
      .then((r) => setActive(r.active))
      .catch((err) => setError(err?.message || 'Failed to load autopilot status'));
  }, []);

  // Suggestions are a secondary decoration: score the top capacity nodes
  // with the agent's own heuristic and surface the best unconnected ones.
  // Any failure leaves the card as the plain toggle.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const panel = await getLightningNetwork(25);
        const candidates = panel.topNodes || [];
        const r = await getLightningAutopilotScores(candidates.map((n) => n.pubkey));
        if (cancelled) return;
        setHeuristic(r.heuristic);
        setSuggestions(
          candidates
            .map((node) => ({ node, score: r.scores[node.pubkey] ?? 0 }))
            .filter((s) => s.score > 0)
            .sort((a, b) => b.score - a.score)
            .slice(0, 3),
        );
      } catch {
        if (!cancelled) setSuggestions([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const toggle = async () => {
    if (active === null) return;
    setBusy(true);
    setError(null);
    try {
      await setLightningAutopilot(!active);
      setActive(!active);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Toggle failed');
    } finally {
      setBusy(false);
    }
  };

  const openChannelTo = (n: TopLightningNode) => {
    const uri = n.address ? `${n.pubkey}@${n.address}` : n.pubkey;
    navigate(`/wallet/lightning/channels?peer=${encodeURIComponent(uri)}`);
  };

  return (
    <div className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3">
      <div className="flex items-center gap-3">
        <Bot className="h-5 w-5 text-primary shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold">Autopilot</div>
          <div className="text-xs text-muted-foreground">
            Automatically opens channels using up to 60% of the lightning account's spendable funds.
          </div>
          {error && <div className="text-xs text-destructive mt-1">{error}</div>}
        </div>
        <button
          type="button"
          onClick={toggle}
          disabled={busy || active === null}
          className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors disabled:opacity-50 ${
            active ? 'bg-primary/20 text-primary' : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
          }`}
        >
          {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : active ? 'Enabled' : 'Disabled'}
        </button>
      </div>
      {suggestions.length > 0 && (
        <div className="border-t border-border/30 pt-3">
          <div className="text-xs text-muted-foreground mb-1">
            Recommended next peers (by autopilot score)
          </div>
          {suggestions.map((s) => (
            <div key={s.node.pubkey} className="flex items-center gap-3 py-1.5">
              {s.node.alias ? (
                <span className="text-sm font-medium truncate">{s.node.alias}</span>
              ) : (
                <span className="font-mono text-xs text-muted-foreground truncate">
                  {truncate(s.node.pubkey, 8)}
                </span>
              )}
              <span className="ml-auto shrink-0">
                <ScoreMeter score={s.score} heuristic={heuristic} />
              </span>
              <button
                type="button"
                onClick={() => openChannelTo(s.node)}
                title="Open a channel with this node"
                className="inline-flex items-center px-2 py-1 rounded hover:bg-muted/20 transition-colors text-muted-foreground hover:text-foreground shrink-0"
              >
                <Plug className="h-4 w-4" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
