// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Gamepad2, Loader2 } from 'lucide-react';
import { GamingGame, getGamingGames } from '../../services/gamingApi';
import { GamingInvite } from './gamingInviteParse';

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
// It states what it cannot do. Accepting an invite has to start the game, and
// nothing starts games yet, so the chip says so rather than offering a button
// that quietly does nothing.
export const GamingInviteChip = ({ invite }: { invite: GamingInvite }) => {
  const [game, setGame] = useState<GamingGame | null>(null);
  const [loading, setLoading] = useState(true);

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
      ) : (
        // The honest state today. Accepting means starting the game and
        // running its onboarding, and nothing starts games yet - so this
        // says so instead of offering a button that would do nothing.
        <span className="text-xs text-muted-foreground">
          {name} is added, but games cannot be launched from here yet.
        </span>
      )}
    </span>
  );
};
