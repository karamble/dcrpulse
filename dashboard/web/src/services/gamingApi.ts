// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

// One registered game's confinement: what it is called, the account it may
// spend from, and how much of it. Per game because the credential a game
// presents is an identity, and a cap on a principal nobody can tell apart is
// not a cap.
export interface GamePolicy {
  // name is the operator's label; blank means the game is called by its id.
  name: string;
  // account is blank until the operator binds one, and a game with no account
  // bound can stake nothing.
  account: string;
  perTableCapDcr: number;
  perDayCapDcr: number;
  approvalTimeoutSecs: number;
}

// The bridge's own state: whether it runs, which games are registered, and what
// each of them is trusted with. Everything that costs money is per game.
export interface GamingSettings {
  enabled: boolean;
  registeredGames: string[];
  policies: Record<string, GamePolicy>;
  // gameCredentials says which games have been issued a credential and when.
  // Only the fingerprint travels: it is enough to tell two credentials apart
  // and useless for connecting with. It travels outward only - the server
  // issues credentials and never reads one back.
  gameCredentials?: Record<string, GameCredential>;
}

export interface GameCredential {
  fingerprint: string;
  issuedAt: number;
}

// What an operator carries to a game, shown once and never again. The private
// key is in this answer and in no file on the appliance.
export interface GamingCredentialMaterial {
  game: string;
  certPem: string;
  keyPem: string;
  bridgeCertPem: string;
  fingerprint: string;
  issuedAt: number;
}

// What a game's connection wizard needs besides its own credential.
export interface GamingBridgeInfo {
  bridgeCertPem: string;
  port: string;
}

export interface GamingGame {
  // id is the routing key, and the only name the wire carries.
  id: string;
  // name is the operator's label, or the id when they gave none.
  name: string;
  // ready reports whether the game is connected to the bridge right now. A
  // game runs on a machine of the person's choosing, so it can be registered
  // and simply not running.
  ready: boolean;
  // minRefundBlocks and bondLockBlocks are the lock terms the game advertised
  // on connecting. A table's refund lock is minted at max(288, minRefundBlocks);
  // bondLockBlocks is 0 for a game that stakes no bond.
  minRefundBlocks: number;
  bondLockBlocks: number;
}

export const getGamingSettings = async (): Promise<GamingSettings> => {
  const { data } = await api.get<GamingSettings>('/br/gaming/settings');
  return data;
};

export const setGamingSettings = async (s: GamingSettings): Promise<GamingSettings> => {
  const { data } = await api.post<GamingSettings>('/br/gaming/settings', s);
  return data;
};

export const getGamingGames = async (): Promise<GamingGame[]> => {
  const { data } = await api.get<{ games: GamingGame[] }>('/br/gaming/games');
  return data.games ?? [];
};

export const getGamingBridgeInfo = async (): Promise<GamingBridgeInfo> => {
  const { data } = await api.get<GamingBridgeInfo>('/br/gaming/bridge');
  return data;
};

// Issuing replaces any credential the game already had, which is what
// regenerating means: the old one stops working at once.
export const issueGamingCredential = async (game: string): Promise<GamingCredentialMaterial> => {
  const { data } = await api.post<GamingCredentialMaterial>('/br/gaming/credential', { game });
  return data;
};

export const revokeGamingCredential = async (game: string): Promise<void> => {
  await api.post('/br/gaming/credential/revoke', { game });
};

// A table proposed into a group chat. The seat is taken as part of posting it.
export interface GamingTable {
  sid: string;
  invite: string;
  // until is the block height registration closes at.
  until: number;
  height: number;
  gcid: string;
}

export const createGamingTable = async (
  game: string,
  gcid: string,
  buyinDcr: number,
  seats: number,
  openBlocks: number,
): Promise<GamingTable> => {
  const { data } = await api.post<GamingTable>('/br/gaming/table', {
    game,
    gcid,
    buyinDcr,
    seats,
    openBlocks,
  });
  return data;
};

// One piece of coin a game has locked on chain. maturesAt is absolute because a
// count of blocks is true for one block and then quietly wrong; blocksLeft is
// derived against the bridge's own tip when it answers.
export interface GamingLock {
  kind: 'bond' | 'stake' | 'tablebond';
  sid?: string;
  seat: number;
  outpoint: string;
  address?: string;
  atoms: number;
  maturesAt: number;
  blocksLeft: number;
  spendable: boolean;
  // spending is a spend sitting in the mempool: the coin is still the game's,
  // because a broadcast can be dropped, but nothing may ask for it again.
  spending: boolean;
  spent: boolean;
}

export interface GamingReportedTable {
  sid: string;
  gcid: string;
  state: string;
  seats: number;
  buyinAtoms: number;
  until: number;
  over: boolean;
  // settling is true while the game could still complete a cooperative
  // settlement. A refund taken then spends an input that settlement needs.
  settling: boolean;
}

export interface GamingReportedState {
  reported: boolean;
  reportedAt?: number;
  tipHeight?: number;
  tables?: GamingReportedTable[];
  locks?: GamingLock[];
  chainErr?: string;
}

export const getGamingState = async (
  game: string,
  refresh = false,
): Promise<GamingReportedState> => {
  const { data } = await api.get<GamingReportedState>(
    `/br/gaming/state?game=${encodeURIComponent(game)}${refresh ? '&refresh=1' : ''}`,
  );
  return data;
};

// Reclaimed coin lands in the account the game is bound to. Where it goes is
// not a parameter here on purpose: the server derives it.
export const reclaimGaming = async (
  game: string,
  kind: 'bond' | 'stake' | 'tablebond',
  sid?: string,
  outpoint?: string,
): Promise<string> => {
  const { data } = await api.post<{ txid: string }>('/br/gaming/reclaim', {
    game,
    kind,
    sid,
    outpoint,
  });
  return data.txid;
};

export const acceptGamingInvite = async (
  game: string,
  invite: string,
  gcid: string,
): Promise<string> => {
  const { data } = await api.post<{ accepted: boolean; sid: string }>('/br/gaming/invite', {
    game,
    invite,
    gcid,
  });
  return data.sid;
};

// 'publishing' is the moment between approval and the network's answer. It
// exists only in the console's view; a game is never told about it.
export type GamingSpendState =
  | 'pending'
  | 'publishing'
  | 'approved'
  | 'denied'
  | 'expired'
  | 'failed';

export interface GamingSpend {
  id: string;
  game: string;
  address: string;
  amountAtoms: number;
  reason?: string;
  state: GamingSpendState;
  txid?: string;
  error?: string;
  requestedAt: number;
  decidedAt?: number;
  expiresAt: number;
}

// A request lapses against the server's clock, not the browser's, and the two
// disagree by however far the machine's time has drifted. Go always sends a
// Date header, so the offset comes back with the answer and a countdown can be
// the server's rather than this computer's opinion of it.
export interface GamingSpendsAnswer {
  // pending is everything still in flight - awaiting an answer or being
  // paid - always in full. decided is one page of history, newest first.
  pending: GamingSpend[];
  decided: GamingSpend[];
  decidedTotal: number;
  page: number;
  pageSize: number;
  // usedToday is the server's own day total per game - the number the cap
  // is actually enforced against, not a client-side re-derivation.
  usedToday: Record<string, number>;
  // serverNow is unix seconds as the server saw them, or 0 when the header was
  // unreadable - in which case a caller should fall back to its own clock.
  serverNow: number;
}

interface spendsWire {
  pending?: GamingSpend[];
  decided?: GamingSpend[];
  decidedTotal?: number;
  page?: number;
  pageSize?: number;
  usedToday?: Record<string, number>;
}

export const getGamingSpends = async (): Promise<GamingSpendsAnswer> => {
  const res = await api.get<spendsWire>('/br/gaming/spends');
  const date = res.headers?.date;
  const parsed = typeof date === 'string' ? Date.parse(date) : NaN;
  return {
    pending: res.data.pending ?? [],
    decided: res.data.decided ?? [],
    decidedTotal: res.data.decidedTotal ?? 0,
    page: res.data.page ?? 1,
    pageSize: res.data.pageSize ?? 10,
    usedToday: res.data.usedToday ?? {},
    serverNow: Number.isFinite(parsed) ? Math.floor(parsed / 1000) : 0,
  };
};

// getGamingSpendHistory reads one page of decided history. Deliberately not
// the shared poll: a page flip is one panel's business, and routing it
// through the store would wake the badge and the strip for it.
export const getGamingSpendHistory = async (
  page: number,
  pageSize: number,
): Promise<{ decided: GamingSpend[]; decidedTotal: number }> => {
  const { data } = await api.get<spendsWire>(
    `/br/gaming/spends?page=${page}&pageSize=${pageSize}`,
  );
  return { decided: data.decided ?? [], decidedTotal: data.decidedTotal ?? 0 };
};

// decideGamingSpend answers a game's request. Approving needs the wallet
// passphrase, because that is the only thing that can move money and it belongs
// to the person, not to the dashboard and never to the game.
export const decideGamingSpend = async (
  id: string,
  approve: boolean,
  passphrase?: string,
): Promise<GamingSpend> => {
  const { data } = await api.post<GamingSpend>('/br/gaming/spends/decide', {
    id,
    approve,
    passphrase: passphrase ?? '',
  });
  return data;
};
