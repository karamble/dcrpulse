// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useContext, useEffect, useState } from 'react';
import { Check, Gamepad2, Loader2 } from 'lucide-react';
import { GamingGame, acceptGamingInvite, getGamingGames } from '../../services/gamingApi';
import { GamingInvite } from './gamingInviteParse';
import { useGamePanel } from './GamePanelProvider';
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
// conversation the invitation arrived in. The chip only ever offers that when
// the game is actually running: a game that is added but not up would take the
// click and do nothing, which is worse than saying so.
export const GamingInviteChip = ({ invite }: { invite: GamingInvite }) => {
  const panel = useGamePanel();
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

  const name = game?.name ?? invite.game;
  const installed = game?.installed ?? false;
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
          Checking whether {name} is installed...
        </span>
      ) : !game ? (
        <span className="text-xs text-muted-foreground">
          This installation does not have {invite.game}, so it cannot join.
        </span>
      ) : !installed ? (
        <span className="text-xs text-muted-foreground">
          Add {name} under Bison Relay &gt; Gaming to join tables like this one.
        </span>
      ) : !ready ? (
        // Added and not running. Installed and ready are different answers,
        // and offering a button here would send a click nothing is listening
        // for.
        <span className="text-xs text-muted-foreground">
          {name} is added, but its service is not running yet.
        </span>
      ) : accepted ? (
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <Check className="h-3 w-3" />
          Joined. {name} is forming the table.
          <button
            type="button"
            onClick={() => { void panel.open(invite.game, invite.sid); }}
            disabled={panel.opening}
            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-primary/20 hover:bg-primary/30 disabled:opacity-50 text-xs font-medium text-primary"
          >
            {panel.opening ? 'Opening...' : 'Open table'}
          </button>
        </span>
      ) : !gcid ? (
        // A table plays in the conversation its invitation arrived in, so
        // there is nothing to join without one.
        <span className="text-xs text-muted-foreground">
          Open this invitation in its group chat to join.
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
          {error && <span className="text-xs text-destructive">{error}</span>}
        </span>
      )}
    </span>
  );
};
