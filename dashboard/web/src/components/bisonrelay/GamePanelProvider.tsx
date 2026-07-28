// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { closeGamePanel, openGamePanel, type GamePanelSession } from '../../services/gameUiApi';
import GamePanel from './GamePanel';

// One panel, mounted above the whole application.
//
// Above rather than inside the Bison Relay page, and that is a decision about
// what a game is rather than about React: a hand of poker is money in escrow
// with obligations attached, and it must survive the user going to look at
// their wallet balance. Mounting it inside a page would end a hand every time
// somebody navigated. The two places a panel is opened from - the gaming tab
// and an accepted invitation in a chat - are also in different parts of the
// tree and need the same one.

interface GamePanelContextValue {
  open: (game: string, tableId?: string) => Promise<void>;
  close: () => void;
  openGame?: string;
  opening: boolean;
  error?: string;
}

const GamePanelContext = createContext<GamePanelContextValue>({
  open: async () => {},
  close: () => {},
  opening: false,
});

export function useGamePanel() {
  return useContext(GamePanelContext);
}

export function GamePanelProvider({ children }: { children: ReactNode }) {
  const [game, setGame] = useState<string>();
  const [session, setSession] = useState<GamePanelSession>();
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState<string>();

  const close = useCallback(() => {
    const token = session?.token;
    setGame(undefined);
    setSession(undefined);
    setError(undefined);
    if (token) {
      // The token dies with the panel. It is short-lived anyway, but a
      // closed panel leaving a working credential behind would make
      // "closed" mean less than it says.
      closeGamePanel(token).catch(() => {});
    }
  }, [session]);

  const open = useCallback(
    async (next: string, tableId?: string) => {
      setOpening(true);
      setError(undefined);
      try {
        const opened = await openGamePanel(next, tableId);
        setGame(next);
        setSession(opened);
      } catch (err) {
        const message =
          (err as { response?: { data?: string } })?.response?.data ||
          (err as Error)?.message ||
          'could not open that game';
        setError(String(message));
      } finally {
        setOpening(false);
      }
    },
    [],
  );

  const value = useMemo(
    () => ({ open, close, openGame: game, opening, error }),
    [open, close, game, opening, error],
  );

  return (
    <GamePanelContext.Provider value={value}>
      {children}
      {game && session && <GamePanel game={game} session={session} onClose={close} />}
    </GamePanelContext.Provider>
  );
}
