// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

// Opening and closing a game panel. Same-origin plus the session cookie is what
// mints the token, which is why it can be as narrow as it is.

export interface GamePanelSession {
  /** The framed page's only credential. Not the game's own token, which
   *  authorizes spending and never reaches a browser. */
  token: string;
  expiresAt: string;
  uiUrl: string;
  apiBase: string;
  tableId?: string;
  /** Derived from the bound gaming account and pinned on the game. */
  payout?: string;
}

export const openGamePanel = async (game: string, tableId?: string): Promise<GamePanelSession> => {
  const { data } = await api.post<GamePanelSession>('/br/gaming/ui/session', { game, tableId });
  return data;
};

export const refreshGamePanel = async (
  token: string,
): Promise<{ token: string; expiresAt: string }> => {
  const { data } = await api.post<{ token: string; expiresAt: string }>(
    '/br/gaming/ui/session/refresh',
    { token },
  );
  return data;
};

export const closeGamePanel = async (token: string): Promise<void> => {
  await api.post('/br/gaming/ui/session/end', { token });
};
