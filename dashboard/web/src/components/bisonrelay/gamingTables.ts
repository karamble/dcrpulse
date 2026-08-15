// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { GamingLock, GamingReportedTable } from '../../services/gamingApi';

export type StageTone = 'muted' | 'info' | 'warning' | 'success';

export interface Stage {
  label: string;
  tone: StageTone;
  line: string;
}

// KNOWN_STATES softens a formation state into a readable phrase, and is
// consulted in one place only: the fallback, where nothing better is known.
//
// "settled" is deliberately absent and must stay absent. A table that has
// finished playing still reports settled, so a map that translated it would
// put a confident, wrong word on a dead table - which is the bug the game's own
// lobby shipped and had to have taken back out.
const KNOWN_STATES: Record<string, string> = {
  joining: 'taking seats',
  formed: 'agreeing its roster',
  committed: 'agreeing its roster',
  aborted: 'given up',
};

// tableStage says what a table is doing, from the only three facts the bridge
// carries about it.
//
// The order of these tests is the design. `state` never decides on its own,
// because it cannot: `settled` covers a table about to deal and a table that
// finished hours ago, and only `over` and `settling` tell those apart. So both
// are checked before anything else, and no later branch may override them.
export const tableStage = (t: GamingReportedTable, tip: number): Stage => {
  if (t.settling) {
    return {
      label: 'Settling',
      tone: 'warning',
      line:
        'The game can still complete a cooperative settlement. Nothing of this table' +
        ' is offered back until that is over - a refund now would spend an input the' +
        ' settlement needs, and defeat the payout every seat signed.',
    };
  }
  if (t.over) {
    return {
      label: 'Finished',
      tone: 'muted',
      line: 'Play is over. Anything still locked is a matter for the coin below.',
    };
  }
  if (tip > 0 && t.until > 0) {
    if (tip < t.until) {
      const left = t.until - tip;
      return {
        label: 'Registration open',
        tone: 'info',
        line: `Closes at block ${t.until.toLocaleString()} - ${left.toLocaleString()} ${
          left === 1 ? 'block' : 'blocks'
        } away. Anyone in the group chat can take a seat until then.`,
      };
    }
    if (tip === t.until) {
      return {
        label: 'Registration closing',
        tone: 'info',
        line: `Block ${t.until.toLocaleString()} is the last one that can carry a seat.`,
      };
    }
    if (tip === t.until + 1) {
      return {
        label: 'Drawing seats',
        tone: 'info',
        line: `Registration closed at block ${t.until.toLocaleString()}. Seats are drawn from this block's hash, which nobody could know while anybody was still joining.`,
      };
    }
  }
  // The honest floor. Past the draw, with neither over nor settling, nothing
  // on the wire separates a table mid-hand from one that never filled - so the
  // game's own word is quoted rather than interpreted.
  const known = KNOWN_STATES[t.state];
  return {
    label: known ? known.charAt(0).toUpperCase() + known.slice(1) : `Reported "${t.state}"`,
    tone: 'muted',
    line: known
      ? `The game reports this table as ${known}.`
      : `The game reports this table as "${t.state}". Nothing here can tell a table still dealing from one that never filled.`,
  };
};

// locksFor is the coin a table is holding, joined on the session id the game
// reports against both.
export const locksFor = (locks: GamingLock[], sid: string): GamingLock[] =>
  locks.filter((l) => l.sid === sid && !l.spent);

// sortTables puts what needs attention first: money that is time-boxed, then
// deadlines by how soon they fall, then everything that has finished.
export const sortTables = (tables: GamingReportedTable[], tip: number): GamingReportedTable[] =>
  [...tables].sort((a, b) => {
    const rank = (t: GamingReportedTable) =>
      t.settling ? 0 : t.over ? 3 : tip > 0 && t.until > tip ? 1 : 2;
    const ra = rank(a);
    const rb = rank(b);
    if (ra !== rb) return ra - rb;
    if (ra === 1) return a.until - b.until;
    return a.sid.localeCompare(b.sid);
  });
