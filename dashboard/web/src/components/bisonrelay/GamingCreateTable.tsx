// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Loader2, X } from 'lucide-react';
import { createGamingTable } from '../../services/gamingApi';
import { listBisonrelayGCs, type BisonrelayGC } from '../../services/bisonrelayApi';

const inputCls =
  'w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm focus:outline-none focus:border-primary/50';
const primaryBtnCls =
  'px-3 py-1.5 rounded-lg bg-gradient-primary text-primary-foreground text-xs font-semibold disabled:opacity-50 disabled:cursor-not-allowed';
const mutedBtnCls =
  'px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-xs font-semibold hover:bg-muted/50';

// A game has no host. The invitation is terms plus a session id, and a table
// exists once enough peers join under the same ones, so this composes a link
// and posts it. The seat is taken as part of sending it.

export const GamingCreateTable = ({ game, onClose }: { game: string; onClose: () => void }) => {
  const [gcs, setGcs] = useState<BisonrelayGC[]>([]);
  const [gcid, setGcid] = useState('');
  const [buyin, setBuyin] = useState(0.01);
  const [seats, setSeats] = useState(2);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [done, setDone] = useState<{ sid: string; until: number } | null>(null);

  useEffect(() => {
    listBisonrelayGCs()
      .then(setGcs)
      .catch(() => {});
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const create = () => {
    setBusy(true);
    setErr(null);
    createGamingTable(game, gcid, buyin, seats)
      .then((t) => setDone({ sid: t.sid, until: t.until }))
      .catch((e) => {
        const body = (e as { response?: { data?: string } })?.response?.data;
        setErr(typeof body === 'string' ? body : (e as Error)?.message || 'Could not create a table');
      })
      .finally(() => setBusy(false));
  };

  return (
    <div className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4" onClick={onClose}>
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold pr-4 capitalize">New {game} table</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {done ? (
          <div className="space-y-3">
            <p className="text-sm">
              Posted, and you are seated. Registration closes at block {done.until.toLocaleString()},
              about 20 minutes from now.
            </p>
            <p className="text-xs text-muted-foreground">
              Anybody in that group chat can take a seat until then. The table forms when it is full
              and gives up if it is not.
            </p>
            <div className="flex gap-2 pt-1">
              <button type="button" onClick={onClose} className={primaryBtnCls}>
                Done
              </button>
            </div>
          </div>
        ) : (
          <>
            <label className="text-xs space-y-1 block">
              <span className="text-muted-foreground block">Group chat to invite</span>
              <select value={gcid} onChange={(e) => setGcid(e.target.value)} className={inputCls}>
                <option value="">Pick a group chat</option>
                {gcs.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.alias || g.name || g.id}
                  </option>
                ))}
              </select>
              <span className="text-muted-foreground block">
                The invitation is an ordinary message, so everyone there can read it whether or not
                they have the game.
              </span>
            </label>

            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground block">Buy-in a seat (DCR)</span>
                <input
                  type="number"
                  min={0}
                  step="0.001"
                  value={buyin}
                  onChange={(e) => setBuyin(Number(e.target.value) || 0)}
                  className={inputCls}
                />
              </label>
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground block">Seats</span>
                <input
                  type="number"
                  min={2}
                  max={6}
                  step="1"
                  value={seats}
                  onChange={(e) => setSeats(Number(e.target.value) || 2)}
                  className={inputCls}
                />
              </label>
            </div>

            <p className="text-xs text-muted-foreground">
              Registration stays open about 20 minutes, stated as a block height because that is
              what every player checks. Your stake is refundable by you alone after a day if the
              table never deals.
            </p>

            {err && (
              <div className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            )}

            <div className="flex gap-2 pt-1">
              <button type="button" onClick={onClose} className={mutedBtnCls}>
                Cancel
              </button>
              <button
                type="button"
                onClick={create}
                disabled={busy || !gcid || buyin <= 0}
                className={primaryBtnCls}
              >
                {busy && <Loader2 className="h-3 w-3 animate-spin inline mr-1" />}
                {busy ? 'Posting...' : 'Post the invitation'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
};
