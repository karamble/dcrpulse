// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useContext, useEffect, useState } from 'react';
import { Check, Gamepad2, Loader2 } from 'lucide-react';
import { GamingGame, acceptGamingInvite, getGamingGames } from '../../services/gamingApi';
import { GamingInvite } from './gamingInviteParse';
import { GamingChatCtx } from './gamingChatContext';

const fmtDcr = (atoms: number): string => (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

// GamingInviteChip renders a gaming:// invite found in a chat message, the same
// way an lnpay:// invoice renders as a pay chip.
//
// The invite is an ordinary message rather than hidden protocol traffic, so it
// stays legible to anyone: a client that knows nothing about games shows a link
// a person can still act on. What this adds is the part a host can answer
// honestly - which game it is, what it costs, and whether this installation can
// actually join.
//
// Accepting hands the invitation to the game, which joins the table in the
// conversation the invitation arrived in. The seat itself is signed for by a key
// only the game holds. The chip offers that only when the game is connected: a
// game that is registered but not running would take the click and do nothing.
export const GamingInviteChip = ({ invite }: { invite: GamingInvite }) => {
  const [game, setGame] = useState<GamingGame | null>(null);
  const [loading, setLoading] = useState(true);
  const [accepting, setAccepting] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const gcid = useContext(GamingChatCtx);

  useEffect(() => {
    let cancelled = false;
    getGamingGames()
      .then((games) => {
        if (cancelled) return;
        setGame(games.find((g) => g.id === invite.game) ?? null);
      })
      .catch(() => {
        /* An unreachable catalogue leaves the invite readable, which is the
           point of it being a message rather than a frame. */
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [invite.game]);

  // A game is in the list only if the operator registered it, so its absence
  // is the answer rather than a separate case.
  const name = game?.name ?? invite.game;
  const ready = game?.ready ?? false;

  const accept = async () => {
    if (!gcid) return;
    setAccepting(true);
    setError(null);
    try {
      await acceptGamingInvite(invite.game, invite.raw, gcid);
      setAccepted(true);
    } catch (e) {
      // The game's own refusal is the useful part - it knows why the
      // invitation was not one it could act on.
      const detail =
        (e as { response?: { data?: string } })?.response?.data ?? (e as Error)?.message ?? '';
      setError(String(detail).trim() || 'could not accept');
    } finally {
      setAccepting(false);
    }
  };

  return (
    <span className="inline-flex flex-col gap-1 my-1 px-3 py-2 rounded-lg bg-muted/10 border border-border/50 text-sm align-middle">
      <span className="flex items-center gap-2 font-medium">
        <Gamepad2 className="h-4 w-4 shrink-0" />
        {name}
        {invite.kind === 'table' ? ' table' : ` ${invite.kind}`}
      </span>

      <span className="text-xs text-muted-foreground">
        {invite.buyinAtoms !== null && <>Buy-in {fmtDcr(invite.buyinAtoms)} DCR. </>}
        {invite.seats !== null && <>{invite.seats} seats. </>}
        {invite.buyinAtoms === null && invite.seats === null && <>No terms stated. </>}
      </span>

      {loading ? (
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          Checking whether {name} is registered...
        </span>
      ) : !game ? (
        <span className="text-xs text-muted-foreground">
          Register <span className="font-mono">{invite.game}</span> under Bison Relay &gt; Gaming to
          join tables like this one.
        </span>
      ) : accepted ? (
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <Check className="h-3 w-3" />
          Joined. {name} is forming the table.
        </span>
      ) : !gcid ? (
        // A table plays in the conversation its invitation arrived in, so
        // there is nothing to join without one.
        <span className="text-xs text-muted-foreground">
          Open this invitation in its group chat to join.
        </span>
      ) : !ready ? (
        // Registered and connected are different answers, and a button here
        // would send a click nothing is listening for.
        <span className="text-xs text-muted-foreground">
          {name} is registered, but not connected to this bridge.
        </span>
      ) : (
        <span className="flex flex-col gap-1">
          <button
            type="button"
            onClick={accept}
            disabled={accepting}
            className="self-start inline-flex items-center gap-1 px-2 py-1 rounded-md bg-primary/20 hover:bg-primary/30 disabled:opacity-50 text-xs font-medium"
          >
            {accepting && <Loader2 className="h-3 w-3 animate-spin" />}
            {accepting ? 'Joining...' : 'Accept'}
          </button>
          {error && <span className="text-xs text-destructive break-words">{error}</span>}
        </span>
      )}
    </span>
  );
};
