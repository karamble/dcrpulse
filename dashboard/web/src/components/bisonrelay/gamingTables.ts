// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { GamingReportedTable } from '../../services/gamingApi';

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
// The game supplies presentation status only. Financial readiness and recovery
// are rendered from the bridge authority ledger elsewhere.
export const tableStage = (t: GamingReportedTable, tip: number): Stage => {
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

// sortTables puts active registration deadlines first, then other active
// tables, then finished presentation state.
export const sortTables = (tables: GamingReportedTable[], tip: number): GamingReportedTable[] =>
  [...tables].sort((a, b) => {
    const rank = (t: GamingReportedTable) =>
	  t.over ? 2 : tip > 0 && t.until > tip ? 0 : 1;
    const ra = rank(a);
    const rb = rank(b);
    if (ra !== rb) return ra - rb;
    if (ra === 0) return a.until - b.until;
    return a.sid.localeCompare(b.sid);
  });
