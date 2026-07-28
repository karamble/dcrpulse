// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

// Opening and closing a game panel.
//
// These go through the ordinary dashboard API, which means same-origin plus the
// session cookie - and that is the whole reason the token they hand back can be
// as narrow as it is. Only this application, at this origin, with the user
// logged in, can ask for one. The page that receives it could never have.

export interface GamePanelSession {
  /** The credential the framed page uses, and its only one. It is not the
   *  game's own token: that one authorizes spending and never reaches a
   *  browser. */
  token: string;
  expiresAt: string;
  /** Where to point the frame. */
  uiUrl: string;
  /** Where the frame sends its API calls. */
  apiBase: string;
  tableId?: string;
  /** Where this host will have the game pay the user. Derived here from the
   *  bound wallet account and pinned on the game before the panel opened. */
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
