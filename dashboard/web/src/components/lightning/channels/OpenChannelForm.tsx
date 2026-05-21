import { useEffect, useRef, useState } from 'react';
import { AlertCircle, ChevronDown, Loader2, Plug } from 'lucide-react';
import {
  PeerPreset,
  getLightningPeerPresets,
  openLightningChannel,
} from '../../../services/lightningApi';
import { SearchForNodesModal } from './SearchForNodesModal';

interface Props {
  onChannelOpened: () => void;
}

const dcrToAtoms = (dcr: string): number => {
  const n = Number(dcr);
  if (!isFinite(n) || n <= 0) return 0;
  return Math.round(n * 1e8);
};

const isValidPubkeyHex = (s: string) => /^[0-9a-fA-F]{66}$/.test(s);
const isValidPeerURI = (s: string) => {
  const at = s.indexOf('@');
  if (at === -1) return isValidPubkeyHex(s);
  return isValidPubkeyHex(s.slice(0, at)) && s.slice(at + 1).length > 0;
};

export const OpenChannelForm = ({ onChannelOpened }: Props) => {
  const [presets, setPresets] = useState<PeerPreset[]>([]);
  const [peerUri, setPeerUri] = useState('');
  const [localDcr, setLocalDcr] = useState('');
  const [pushDcr, setPushDcr] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [presetsOpen, setPresetsOpen] = useState(false);
  const presetsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    getLightningPeerPresets()
      .then((r) => setPresets(r.presets))
      .catch(() => setPresets([]));
  }, []);

  useEffect(() => {
    if (!presetsOpen) return;
    const close = (e: MouseEvent) => {
      if (presetsRef.current && !presetsRef.current.contains(e.target as Node)) {
        setPresetsOpen(false);
      }
    };
    window.addEventListener('mousedown', close);
    return () => window.removeEventListener('mousedown', close);
  }, [presetsOpen]);

  const localAtoms = dcrToAtoms(localDcr);
  const pushAtoms = pushDcr ? dcrToAtoms(pushDcr) : 0;
  const canSubmit =
    !submitting &&
    isValidPeerURI(peerUri.trim()) &&
    localAtoms > 0 &&
    pushAtoms < localAtoms;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    setFeedback(null);
    try {
      const resp = await openLightningChannel({
        peerUri: peerUri.trim(),
        localAtoms,
        pushAtoms: pushAtoms || undefined,
      });
      setFeedback(`Channel pending: ${resp.fundingTxid}:${resp.outputIndex}`);
      setPeerUri('');
      setLocalDcr('');
      setPushDcr('');
      onChannelOpened();
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to open channel');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <Plug className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Create a channel</h3>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label htmlFor="ln-peer" className="block text-sm text-muted-foreground mb-1">
            Counterparty node (pubkey or pubkey@host:port)
          </label>
          <div className="flex gap-2 items-stretch">
            <input
              id="ln-peer"
              list="ln-peer-presets"
              type="text"
              value={peerUri}
              onChange={(e) => setPeerUri(e.target.value)}
              placeholder="03…@host:9735"
              disabled={submitting}
              className="flex-1 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50 font-mono text-xs"
            />
            <div ref={presetsRef} className="relative">
              <button
                type="button"
                onClick={() => setPresetsOpen((o) => !o)}
                disabled={submitting || presets.length === 0}
                title={presets.length === 0 ? 'No presets (enable in Settings → Privacy)' : 'Pick a peer preset'}
                className="h-full px-3 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm disabled:opacity-50 inline-flex items-center gap-1"
              >
                Presets
                <ChevronDown className="h-3.5 w-3.5" />
              </button>
              {presetsOpen && presets.length > 0 && (
                <div className="absolute right-0 mt-1 w-80 max-w-[calc(100vw-2rem)] max-h-72 overflow-y-auto rounded-lg border border-border/50 bg-card shadow-xl z-10">
                  <ul className="divide-y divide-border/40">
                    {presets.map((p) => (
                      <li key={p.uri}>
                        <button
                          type="button"
                          onClick={() => {
                            setPeerUri(p.uri);
                            setPresetsOpen(false);
                          }}
                          className="w-full text-left px-3 py-2 hover:bg-muted/20"
                        >
                          <div className="text-sm font-medium flex items-center gap-2">
                            {p.label}
                            {p.isFallback && (
                              <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/30 text-muted-foreground">
                                fallback
                              </span>
                            )}
                          </div>
                          <div className="text-xs text-muted-foreground font-mono break-all">
                            {p.uri}
                          </div>
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
            <button
              type="button"
              onClick={() => setSearchOpen(true)}
              disabled={submitting}
              className="px-3 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm disabled:opacity-50"
            >
              Search graph
            </button>
          </div>
          <p className="text-xs text-muted-foreground mt-1">
            Presets are fetched from Bison Relay's seeder (bisonrelay.org). Disable under Settings → Privacy if you prefer manual entry only.
          </p>
          <datalist id="ln-peer-presets">
            {presets.map((p) => (
              <option key={p.uri} value={p.uri} label={p.label} />
            ))}
          </datalist>
          {peerUri && !isValidPeerURI(peerUri.trim()) && (
            <p className="text-xs text-destructive mt-1">
              Must be 66 hex chars (pubkey) optionally followed by @host:port.
            </p>
          )}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm text-muted-foreground mb-1">Local funding (DCR)</label>
            <input
              type="number"
              step="0.00000001"
              min="0"
              value={localDcr}
              onChange={(e) => setLocalDcr(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>
          <div>
            <label className="block text-sm text-muted-foreground mb-1">Push amount (DCR, optional)</label>
            <input
              type="number"
              step="0.00000001"
              min="0"
              value={pushDcr}
              onChange={(e) => setPushDcr(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>
        </div>

        {error && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}
        {feedback && (
          <div className="text-sm text-success break-all">{feedback}</div>
        )}

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={!canSubmit}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            {submitting ? 'Opening…' : 'Open channel'}
          </button>
        </div>
      </form>

      {searchOpen && (
        <SearchForNodesModal
          onClose={() => setSearchOpen(false)}
          onPick={(pubkey) => {
            setPeerUri(pubkey);
            setSearchOpen(false);
          }}
        />
      )}
    </div>
  );
};
