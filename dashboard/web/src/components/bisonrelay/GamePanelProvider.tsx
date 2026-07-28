// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { closeGamePanel, openGamePanel, type GamePanelSession } from '../../services/gameUiApi';
import GamePanel from './GamePanel';

// One panel, above the routes: a hand in progress has money in escrow and has
// to survive the user navigating away.

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
