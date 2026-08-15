// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { getGamingState } from '../../services/gamingApi';
import { apiError } from '../../utils/apiError';
import type { GameState } from './GamingTablesCard';

// What every registered game last reported about its tables and its coin.
//
// Held once at the tab rather than per panel. Each panel doing its own read
// meant one round trip per game per panel, to programs on machines somebody
// else is running, every time the tab was opened.
//
// The default read is the bridge's cached copy. A live look needs the game to
// be running and the route refuses the whole request when it is not, so asking
// live by default turned "that game is switched off" into "nothing is known
// about what it holds" - about coin the bridge had a perfectly good record of.
export const useGamingStates = (games: { id: string; name: string }[]) => {
  const [states, setStates] = useState<Record<string, GameState>>({});
  const [refreshing, setRefreshing] = useState(false);

  const key = games.map((g) => g.id).join(',');
  const namesRef = useRef<Record<string, string>>({});
  for (const g of games) namesRef.current[g.id] = g.name;

  const loadOne = useCallback(async (id: string, live: boolean) => {
    const name = namesRef.current[id] ?? id;
    try {
      const state = await getGamingState(id, live);
      setStates((s) => ({ ...s, [id]: { game: id, name, state, error: null } }));
    } catch (e) {
      const text = apiError(e, `Could not read what ${name} holds`);
      if (!live) {
        setStates((s) => ({
          ...s,
          [id]: { game: id, name, state: s[id]?.state ?? null, error: text },
        }));
        return;
      }
      // A live look failed, so fall back to what the bridge kept.
      try {
        const state = await getGamingState(id, false);
        setStates((s) => ({ ...s, [id]: { game: id, name, state, error: null } }));
      } catch (e2) {
        setStates((s) => ({
          ...s,
          [id]: {
            game: id,
            name,
            state: s[id]?.state ?? null,
            error: apiError(e2, `Could not read what ${name} holds`),
          },
        }));
      }
    }
  }, []);

  useEffect(() => {
    const ids = key ? key.split(',') : [];
    for (const id of ids) void loadOne(id, false);
  }, [key, loadOne]);

  // refreshAll is the explicit ask: one live look per game, in sequence rather
  // than all at once, because each is a round trip to a separate program.
  const refreshAll = useCallback(async () => {
    setRefreshing(true);
    try {
      const ids = key ? key.split(',') : [];
      for (const id of ids) await loadOne(id, true);
    } finally {
      setRefreshing(false);
    }
  }, [key, loadOne]);

  return { states, refreshing, refreshAll, refreshOne: loadOne };
};
