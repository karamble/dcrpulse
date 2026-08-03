// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export interface MsigPeer {
  uid: string;
  nick: string;
  pubkey?: string;
  xpub?: string;
  state: string;
  lastSeenTs?: number;
  reason?: string;
}

export interface MsigProposalInput {
  txid: string;
  vout: number;
  tree: number;
  atoms: number;
}

export interface MsigProposalOutput {
  address: string;
  atoms: number;
  isChange: boolean;
}

export interface MsigQueueHop {
  uid: string;
  nick: string;
  state: string;
  sentAt?: number;
  deadline?: number;
  reason?: string;
}

export interface MsigProposal {
  txid: string;
  role: 'initiator' | 'cosigner';
  status: string;
  rawTx: string;
  sigCount: number;
  signedBy?: Record<string, boolean>;
  inputs: MsigProposalInput[];
  outputs: MsigProposalOutput[];
  feeAtoms: number;
  note?: string;
  reason?: string;
  queue?: MsigQueueHop[];
  hopTtlSecs?: number;
  fromUid?: string;
  fromNick?: string;
  createdAt: number;
  updatedAt: number;
}

export interface MsigOwnHDKey {
  xpub: string;
  account: number;
}

export interface MsigCursor {
  next: number;
  importedThrough: number;
  lastUsed: number;
}

export interface MsigWallet {
  tempId: string;
  address?: string;
  label: string;
  m: number;
  n: number;
  network: string;
  transport?: 'manual' | '';
  hd?: boolean;
  xpubs?: string[];
  ownHd?: MsigOwnHDKey;
  ext?: MsigCursor;
  int?: MsigCursor;
  role: 'initiator' | 'cosigner';
  status: string;
  failReason?: string;
  createdHeight?: number;
  initiatorUid?: string;
  peers: MsigPeer[];
  createdAt: number;
  updatedAt: number;
  proposals?: Record<string, MsigProposal>;
}

export interface MsigUTXO {
  txid: string;
  vout: number;
  tree: number;
  atoms: number;
  confirmations: number;
  address?: string;
}

export interface MsigReceive {
  address: string;
  index: number;
  gapExhausted: boolean;
}

export interface MsigDetail {
  record: MsigWallet;
  walletName: string;
  isActiveWallet: boolean;
  utxos?: MsigUTXO[];
  balanceAtoms: number;
  receive?: MsigReceive;
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
  invitees: MsigInvitee[],
  transport: '' | 'manual',
  passphrase: string,
): Promise<MsigWallet> => {
  const { data } = await api.post<MsigWallet>('/msig/wallets/invite', { label, m, invitees, transport, passphrase });
  return data;
};

export interface MsigManualFrame {
  mid: string;
  toUid: string;
  toNick: string;
  type: string;
  body: string;
  ts: number;
}

export const getMsigOutbox = async (id: string): Promise<MsigManualFrame[]> => {
  const { data } = await api.get('/msig/outbox', { params: { id } });
  return data.frames ?? [];
};

export const markMsigDelivered = async (id: string, mid: string, toUid: string): Promise<void> => {
  await api.post('/msig/outbox/done', { id, mid, toUid });
};

export interface MsigImportResult {
  outcome: 'processed' | 'duplicate' | 'ignored';
  type: string;
  walletId?: string;
  label?: string;
}

export const importMsigFrame = async (req: {
  body: string;
  id?: string;
  fromUid?: string;
  fromLabel?: string;
}): Promise<MsigImportResult> => {
  const { data } = await api.post<MsigImportResult>('/msig/import', req);
  return data;
};

// msigFrameTypeLabel names a coordination message for humans.
export const msigFrameTypeLabel = (type: string): string => {
  switch (type) {
    case 'invite':
      return 'Invitation';
    case 'accept':
      return 'Acceptance';
    case 'decline':
      return 'Decline';
    case 'roster':
      return 'Key roster';
    case 'ready':
      return 'Ready confirmation';
    case 'invite_cancel':
      return 'Cancellation';
    case 'sign_req':
      return 'Signing request';
    case 'sig':
      return 'Signature';
    case 'sig_decline':
      return 'Signing refusal';
    case 'broadcast':
      return 'Broadcast notice';
    default:
      return type;
  }
};

export const acceptMsigInvite = async (id: string, passphrase: string): Promise<void> => {
  await api.post('/msig/wallets/accept', { id, passphrase });
};

export const freshMsigAddress = async (id: string): Promise<MsigReceive> => {
  const { data } = await api.post<MsigReceive>('/msig/receive', { id });
  return { ...data, gapExhausted: false };
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

export const restoreMsigWallet = async (card: unknown, passphrase?: string): Promise<MsigWallet> => {
  const { data } = await api.post<MsigWallet>('/msig/wallets/restore', { card, passphrase });
  return data;
};

export interface MsigRecipient {
  address: string;
  amountAtoms: number;
}

export const proposeMsigSpend = async (
  walletId: string,
  recipients: MsigRecipient[],
  sendAll: boolean,
  queueUids: string[],
  note: string,
  hopTtlSecs: number,
  passphrase: string,
): Promise<MsigProposal> => {
  const { data } = await api.post<MsigProposal>('/msig/proposals/propose', {
    walletId, recipients, sendAll, queueUids, note, hopTtlSecs, passphrase,
  });
  return data;
};

export const signMsigProposal = async (
  walletId: string,
  txid: string,
  passphrase: string,
): Promise<void> => {
  await api.post('/msig/proposals/sign', { walletId, txid, passphrase });
};

export const rejectMsigProposal = async (walletId: string, txid: string, reason?: string): Promise<void> => {
  await api.post('/msig/proposals/reject', { walletId, txid, reason });
};

export const abortMsigProposal = async (walletId: string, txid: string): Promise<void> => {
  await api.post('/msig/proposals/abort', { walletId, txid });
};

export const rebroadcastMsigProposal = async (walletId: string, txid: string): Promise<void> => {
  await api.post('/msig/proposals/rebroadcast', { walletId, txid });
};

export const msigProposalLabel = (status: string): string => {
  switch (status) {
    case 'collecting':
      return 'Collecting signatures';
    case 'ready':
      return 'Ready to broadcast';
    case 'broadcast':
      return 'Broadcast';
    case 'confirmed':
      return 'Confirmed';
    case 'incoming':
      return 'Needs your approval';
    case 'signed':
      return 'You approved';
    case 'declined':
      return 'Declined';
    case 'failed':
      return 'Failed';
    case 'aborted':
      return 'Cancelled';
    case 'superseded':
      return 'Superseded';
    default:
      return status;
  }
};

export const msigHopLabel = (state: string): string => {
  switch (state) {
    case 'pending':
      return 'Waiting in queue';
    case 'sent':
      return 'Asked to approve';
    case 'signed':
      return 'Approved';
    case 'declined':
      return 'Declined';
    case 'timeout':
      return 'No answer';
    case 'skipped':
      return 'Skipped';
    default:
      return state;
  }
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
