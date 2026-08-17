// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, Loader2, X } from 'lucide-react';
import { createGamingTable } from '../../services/gamingApi';
import { listBisonrelayGCs, type BisonrelayGC } from '../../services/bisonrelayApi';
import { apiError } from '../../utils/apiError';
import { blocksToDuration } from '../../utils/blocks';

// The dashboard mints a table's refund lock at max(this, the game's advertised
// minimum). It matches the daemon's own gamingRefundBlocks default.
const GAMING_REFUND_BLOCKS = 288;

const inputCls =
  'w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm focus:outline-none focus:border-primary/50';
const primaryBtnCls =
  'px-3 py-1.5 rounded-lg bg-gradient-primary text-primary-foreground text-xs font-semibold disabled:opacity-50 disabled:cursor-not-allowed';
const mutedBtnCls =
  'px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-xs font-semibold hover:bg-muted/50';

// A game has no host. The invitation is terms plus a session id, and a table
// exists once enough peers join under the same ones, so this composes a link
// and posts it. The seat is taken as part of sending it.
export const GamingCreateTable = ({
  game,
  label,
  minRefundBlocks,
  bondLockBlocks,
  onClose,
}: {
  game: string;
  label: string;
  minRefundBlocks: number;
  bondLockBlocks: number;
  onClose: () => void;
}) => {
  const refundBlocks = Math.max(GAMING_REFUND_BLOCKS, minRefundBlocks);
  const [gcs, setGcs] = useState<BisonrelayGC[]>([]);
  const [gcid, setGcid] = useState('');
  const [buyin, setBuyin] = useState(0.001);
  const [seats, setSeats] = useState(2);
  const [openBlocks, setOpenBlocks] = useState(1);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [gcErr, setGcErr] = useState<string | null>(null);
  const [done, setDone] = useState<{ sid: string; until: number; invite: string } | null>(null);
  // Posting takes a seat and puts a real invitation in a group chat, and there
  // is nothing to cancel once it is under way. So dismissal is refused while
  // it runs rather than leaving somebody seated at a table they never saw.
  const requestClose = useCallback(() => {
    if (busy) {
      setErr('Posting the invitation. This takes a seat, so it cannot be stopped from here.');
      return;
    }
    onClose();
  }, [busy, onClose]);

  useEffect(() => {
    listBisonrelayGCs()
      .then((v) => {
        setGcs(v);
        setGcErr(null);
      })
      .catch((e) => setGcErr(apiError(e, 'Could not read your group chats')));
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') requestClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [requestClose]);

  const create = () => {
    setBusy(true);
    setErr(null);
    createGamingTable(game, gcid, buyin, seats, openBlocks)
      .then((t) => setDone({ sid: t.sid, until: t.until, invite: t.invite }))
      .catch((e) => setErr(apiError(e, 'Could not create a table')))
      .finally(() => setBusy(false));
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={requestClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold pr-4">New {label} table</h3>
          <button
            type="button"
            onClick={requestClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {done ? (
          <div className="space-y-3">
            <p className="text-sm">
              Posted, and you are seated. Registration closes at block {done.until.toLocaleString()}{' '}
              - roughly {blocksToDuration(openBlocks)} away, though a block takes as long as it takes.
            </p>
            <label className="text-xs space-y-1 block">
              <span className="text-muted-foreground block">
                The invitation, if you need to send it again
              </span>
              <textarea
                readOnly
                rows={2}
                value={done.invite}
                onFocus={(e) => e.currentTarget.select()}
                className="w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 font-mono text-[11px]"
              />
              <span className="text-muted-foreground block font-mono break-all">{done.sid}</span>
            </label>
            <p className="text-xs text-muted-foreground">
              Anybody in that group chat can take a seat until then, and seats are drawn a block
              later. The table forms when it is full and gives up if it is not.
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
              {gcErr ? (
                <span className="text-destructive block break-words">
                  Your group chats could not be read: {gcErr}. This list being empty is that
                  failure, not an answer about which chats you are in.
                </span>
              ) : (
                <span className="text-muted-foreground block">
                  The invitation is an ordinary message, so everyone there can read it whether or
                  not they have the game.
                </span>
              )}
            </label>

            <div className="grid gap-3 sm:grid-cols-3">
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
              <label className="text-xs space-y-1">
                <span className="text-muted-foreground block">Open for (blocks)</span>
                <input
                  type="number"
                  min={1}
                  max={288}
                  step="1"
                  value={openBlocks}
                  onChange={(e) => setOpenBlocks(Number(e.target.value) || 1)}
                  className={inputCls}
                />
              </label>
            </div>

            <p className="text-xs text-muted-foreground">
              Registration closes {openBlocks === 1 ? 'one block' : `${openBlocks} blocks`} from
              now, roughly {blocksToDuration(openBlocks)} at the target rate, stated as a height
              because that is what every player checks and a block takes as long as it takes. Seats
              are drawn a block after that, from a hash nobody could know while anybody was still
              joining. Anyone who has not accepted by then misses the table. Your stake is
              refundable by you alone after {blocksToDuration(refundBlocks)} ({refundBlocks} blocks)
              if the table never deals.
            </p>

            {bondLockBlocks > 0 && (
              <p className="text-xs text-muted-foreground">
                {label} locks a seat bond for {blocksToDuration(bondLockBlocks)} ({bondLockBlocks}{' '}
                blocks) once the table forms.
              </p>
            )}

            {err && (
              <div className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            )}

            <div className="flex gap-2 pt-1">
              <button type="button" onClick={requestClose} className={mutedBtnCls}>
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
