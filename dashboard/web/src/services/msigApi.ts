// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export interface MsigOwnKey {
  pubKey: string;
  address: string;
  account: number;
  branch: number;
  index: number;
}

export interface MsigPeer {
  uid: string;
  nick: string;
  pubkey?: string;
  state: string;
  lastSeenTs?: number;
  reason?: string;
}

export interface MsigWallet {
  tempId: string;
  address?: string;
  label: string;
  m: number;
  n: number;
  network: string;
  scriptHex?: string;
  rosterPubKeys?: string[];
  role: 'initiator' | 'cosigner';
  status: string;
  failReason?: string;
  createdHeight?: number;
  own?: MsigOwnKey;
  initiatorUid?: string;
  peers: MsigPeer[];
  createdAt: number;
  updatedAt: number;
}

export interface MsigUTXO {
  txid: string;
  vout: number;
  tree: number;
  atoms: number;
  confirmations: number;
}

export interface MsigDetail {
  record: MsigWallet;
  walletName: string;
  isActiveWallet: boolean;
  utxos?: MsigUTXO[];
  balanceAtoms: number;
}

export interface MsigPendingItem {
  walletName: string;
  tempId: string;
  label: string;
  m: number;
  n: number;
  status: string;
  initiatorNick?: string;
  needsSwitch: boolean;
  kind: 'invite' | 'resume';
}

export interface MsigInvitee {
  uid: string;
  nick: string;
}

export const listMsigWallets = async (): Promise<{ walletName: string; wallets: MsigWallet[] }> => {
  const { data } = await api.get('/msig/wallets');
  return { walletName: data.walletName, wallets: data.wallets ?? [] };
};

export const createMsigWallet = async (
  label: string,
  m: number,
  account: number,
  invitees: MsigInvitee[],
): Promise<MsigWallet> => {
  const { data } = await api.post<MsigWallet>('/msig/wallets/invite', { label, m, account, invitees });
  return data;
};

export const acceptMsigInvite = async (id: string, account: number): Promise<void> => {
  await api.post('/msig/wallets/accept', { id, account });
};

export const declineMsigInvite = async (id: string, reason?: string): Promise<void> => {
  await api.post('/msig/wallets/decline', { id, reason });
};

export const cancelMsigRound = async (id: string): Promise<void> => {
  await api.post('/msig/wallets/cancel', { id });
};

export const getMsigDetail = async (id: string): Promise<MsigDetail> => {
  const { data } = await api.get<MsigDetail>('/msig/wallets/detail', { params: { id } });
  return data;
};

export const getMsigBackup = async (id: string): Promise<unknown> => {
  const { data } = await api.get('/msig/wallets/backup', { params: { id } });
  return data;
};

export const getMsigPending = async (): Promise<MsigPendingItem[]> => {
  const { data } = await api.get('/msig/pending');
  return data.items ?? [];
};

export const refreshMsig = async (): Promise<void> => {
  await api.post('/msig/refresh');
};

// Status labels shared by the list, banner and detail screens.
export const msigStatusLabel = (status: string): string => {
  switch (status) {
    case 'inviting':
      return 'Waiting for cosigners';
    case 'activating':
      return 'Activating';
    case 'active':
      return 'Active';
    case 'invited':
      return 'Invitation received';
    case 'accepted':
      return 'Waiting for the roster';
    case 'pending_import':
      return 'Switch wallet to finish';
    case 'declined':
      return 'Declined';
    case 'failed':
      return 'Failed';
    default:
      return status;
  }
};

export const msigPeerLabel = (state: string): string => {
  switch (state) {
    case 'invited':
      return 'Invited';
    case 'accepted':
      return 'Key received';
    case 'roster_sent':
      return 'Roster sent';
    case 'ready':
      return 'Ready';
    case 'declined':
      return 'Declined';
    default:
      return state;
  }
};
