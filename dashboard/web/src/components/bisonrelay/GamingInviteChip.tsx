// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useContext, useEffect, useState } from 'react';
import { Gamepad2, Loader2 } from 'lucide-react';
import { GamingGame, getGamingGames } from '../../services/gamingApi';
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
// Taking the seat belongs to the game, not to this page: a seat costs money and
// is signed for by a key only the game holds, so the chip reads the terms and
// says where the person can act on them.
export const GamingInviteChip = ({ invite }: { invite: GamingInvite }) => {
  const [game, setGame] = useState<GamingGame | null>(null);
  const [loading, setLoading] = useState(true);
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
          Register {name} under Bison Relay &gt; Gaming to join tables like this one.
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
        <span className="text-xs text-muted-foreground">
          Join this table from {name}, which is where a seat is taken.
        </span>
      )}
    </span>
  );
};
