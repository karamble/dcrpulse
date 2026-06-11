import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, Copy, Loader2, Trash2 } from 'lucide-react';
import {
  LightningWatchtower,
  addLnWatchtower,
  listLnWatchtowers,
  removeLnWatchtower,
} from '../../../services/lightningApi';
import { StatusPill } from '../StatusPill';

const trunc = (s: string, head = 12, tail = 8) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;
const isValidHex33 = (s: string) => /^[0-9a-fA-F]{66}$/.test(s);

export const WatchtowersSection = () => {
  const [towers, setTowers] = useState<LightningWatchtower[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);

  const [pubKey, setPubKey] = useState('');
  const [address, setAddress] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const { towers } = await listLnWatchtowers();
      setTowers(towers || []);
      setListError(null);
    } catch (err: any) {
      const body = err?.response?.data;
      setListError(typeof body === 'string' ? body : err?.message || 'List failed');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const onAdd = async () => {
    if (!isValidHex33(pubKey.trim()) || !address.trim()) return;
    setAdding(true);
    setAddError(null);
    try {
      await addLnWatchtower(pubKey.trim(), address.trim());
      setPubKey('');
      setAddress('');
      refresh();
    } catch (err: any) {
      const body = err?.response?.data;
      setAddError(typeof body === 'string' ? body : err?.message || 'Add failed');
    } finally {
      setAdding(false);
    }
  };

  const onRemove = async (hex: string) => {
    setBusyKey(hex);
    try {
      await removeLnWatchtower(hex);
      refresh();
    } catch {
      /* best-effort */
    } finally {
      setBusyKey(null);
    }
  };

  const copy = async (v: string) => {
    try {
      await navigator.clipboard.writeText(v);
      setCopiedKey(v);
      window.setTimeout(() => setCopiedKey((c) => (c === v ? null : c)), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  return (
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-4">
      <div>
        <h3 className="text-lg font-semibold">Watchtowers</h3>
        <p className="text-sm text-muted-foreground">
          Register watchtower clients. A watchtower monitors your channels for breach
          attempts while you are offline.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div>
          <label className="block text-sm font-medium mb-1" htmlFor="wt-pubkey">
            Watchtower pubkey (66 hex)
          </label>
          <input
            id="wt-pubkey"
            type="text"
            value={pubKey}
            onChange={(e) => setPubKey(e.target.value.trim())}
            placeholder="03abcd..."
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground font-mono text-xs focus:outline-none focus:border-primary"
          />
        </div>
        <div>
          <label className="block text-sm font-medium mb-1" htmlFor="wt-addr">
            Address (host:port)
          </label>
          <input
            id="wt-addr"
            type="text"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="tower.example.com:9911"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
          />
        </div>
      </div>
      {addError && (
        <div className="text-sm text-destructive inline-flex items-center gap-1">
          <AlertCircle className="h-3.5 w-3.5" />
          {addError}
        </div>
      )}
      <div className="flex justify-end">
        <button
          type="button"
          onClick={onAdd}
          disabled={adding || !isValidHex33(pubKey) || !address.trim()}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
          {adding ? 'Adding…' : 'Add watchtower'}
        </button>
      </div>

      <div className="pt-2">
        <div className="text-sm font-medium mb-2">Registered ({towers.length})</div>
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : listError ? (
          <div className="text-sm text-destructive">{listError}</div>
        ) : towers.length === 0 ? (
          <div className="text-sm text-muted-foreground">No watchtowers registered.</div>
        ) : (
          <div className="space-y-2">
            {towers.map((t) => (
              <div
                key={t.pubKeyHex}
                className="px-3 py-2 rounded-lg bg-background/40 border border-border/60 flex items-start justify-between gap-3"
              >
                <div className="min-w-0 space-y-1">
                  <div className="font-mono text-xs">
                    {trunc(t.pubKeyHex)}
                    <button
                      onClick={() => copy(t.pubKeyHex)}
                      className="ml-2 inline-flex items-center text-muted-foreground hover:text-foreground"
                      title="Copy"
                      type="button"
                    >
                      {copiedKey === t.pubKeyHex ? (
                        <CheckCircle2 className="h-3.5 w-3.5 text-success" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  </div>
                  <div className="text-xs text-muted-foreground space-y-0.5">
                    {t.addresses.map((a) => (
                      <div key={a}>{a}</div>
                    ))}
                    <div>{t.numSessions} session{t.numSessions === 1 ? '' : 's'}</div>
                  </div>
                </div>
                <div className="flex flex-col items-end gap-2 shrink-0">
                  <StatusPill
                    label={t.activeSessionCandidate ? 'Active' : 'Inactive'}
                    tone={t.activeSessionCandidate ? 'success' : 'muted'}
                  />
                  <button
                    type="button"
                    onClick={() => onRemove(t.pubKeyHex)}
                    disabled={busyKey === t.pubKeyHex}
                    className="inline-flex items-center gap-1 text-xs text-destructive hover:text-destructive/80 disabled:opacity-50"
                  >
                    {busyKey === t.pubKeyHex ? <Loader2 className="h-3 w-3 animate-spin" /> : <Trash2 className="h-3 w-3" />}
                    Remove
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
