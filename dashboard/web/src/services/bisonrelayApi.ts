// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export type BisonrelayStage =
  | 'waiting-for-dcrlnd'
  | 'waiting-for-channel'
  | 'needs-identity'
  | 'starting'
  | 'wallet-checking'
  | 'connecting'
  | 'ready'
  | 'disconnected';

export interface BisonrelayStatus {
  stage: BisonrelayStage;
  nick?: string;
  serverNode?: string;
  recommendedPeer?: string;
  walletCheckErr?: string;
  lastUpdated: string;
}

export interface BisonrelayVersion {
  appName: string;
  appVersion: string;
  goRuntime: string;
}

export const getBisonrelayStatus = async (): Promise<BisonrelayStatus> => {
  const { data } = await api.get<BisonrelayStatus>('/br/status');
  return data;
};

export const getBisonrelayVersion = async (): Promise<BisonrelayVersion> => {
  const { data } = await api.get<BisonrelayVersion>('/br/version');
  return data;
};

export const setupBisonrelay = async (nick: string, name: string): Promise<void> => {
  await api.post('/br/setup', { nick, name });
};

export interface BisonrelayContact {
  id?: {
    nick?: string;
    name?: string;
    identity?: string;
    avatar?: string;
  };
  nick_alias?: string;
  first_created?: string;
  last_completed_kx?: string;
  last_read_msg_time?: string;
  ignored?: boolean;
}

export interface BisonrelayContactsResponse {
  entries: BisonrelayContact[] | null;
}

export const getBisonrelayContacts = async (): Promise<BisonrelayContact[]> => {
  const { data } = await api.get<BisonrelayContactsResponse>('/br/contacts');
  return data.entries ?? [];
};

export interface BisonrelayMessage {
  message: string;
  from: string;
  timestamp: number;
  internal: boolean;
  // Client-side only fields used while an async operation (e.g. a tip
  // payment) is in flight. pending=true renders a spinner next to the
  // text; tipKey correlates the placeholder with the eventual live
  // tip-sent / tip-failed event so we can replace it in place.
  pending?: boolean;
  tipKey?: string;
}

export interface BisonrelayMessagesResponse {
  uid: string;
  page: number;
  page_size: number;
  entries: BisonrelayMessage[] | null;
}

export const getBisonrelayMessages = async (
  contact: string,
  page = 0,
  pageSize = 50,
): Promise<BisonrelayMessagesResponse> => {
  const { data } = await api.get<BisonrelayMessagesResponse>('/br/messages', {
    params: { contact, page, page_size: pageSize },
  });
  return data;
};

export interface BisonrelayPMAttachment {
  name: string;
  mime: string;
  data_b64: string;
}

export interface BisonrelayPMSendResult {
  body: string;
}

export const sendBisonrelayPM = async (
  user: string,
  msg: string,
  embed?: BisonrelayPMAttachment,
): Promise<BisonrelayPMSendResult> => {
  const payload: { user: string; msg: string; embed?: BisonrelayPMAttachment } = { user, msg };
  if (embed) payload.embed = embed;
  const { data } = await api.post<BisonrelayPMSendResult>('/br/pm', payload);
  return data ?? { body: msg };
};

export interface BisonrelayFileSendResult {
  filename: string;
  size: number;
}

export const sendBisonrelayFile = async (
  user: string,
  file: File,
): Promise<BisonrelayFileSendResult> => {
  const form = new FormData();
  form.append('user', user);
  form.append('file', file, file.name);
  const { data } = await api.post<BisonrelayFileSendResult>('/br/files/send', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return data ?? { filename: file.name, size: file.size };
};

export interface BisonrelayInvite {
  invite_bytes: string;
  invite_key: string;
}

export const writeBisonrelayInvite = async (): Promise<BisonrelayInvite> => {
  const { data } = await api.post<BisonrelayInvite>('/br/invites/write');
  return data;
};

export const acceptBisonrelayInvite = async (invite: string): Promise<void> => {
  await api.post('/br/invites/accept', { invite });
};

export const renameBisonrelayContact = async (
  uid: string,
  newNick: string,
): Promise<void> => {
  await api.post('/br/contacts/rename', { uid, new_nick: newNick });
};

export const kxResetBisonrelayContact = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/kx-reset', { uid });
};

export const handshakeBisonrelayContact = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/handshake', { uid });
};

export const suggestKxBisonrelayContact = async (
  invitee: string,
  target: string,
): Promise<void> => {
  await api.post('/br/contacts/suggest-kx', { invitee, target });
};

export const transResetBisonrelayContact = async (
  mediator: string,
  target: string,
): Promise<void> => {
  await api.post('/br/contacts/trans-reset', { mediator, target });
};

export const acceptBisonrelayKxSuggestion = async (
  mediator: string,
  target: string,
): Promise<void> => {
  await api.post('/br/contacts/accept-suggestion', { mediator, target });
};

export const tipBisonrelayContact = async (
  uid: string,
  dcrAmount: number,
  maxAttempts: number = 1,
): Promise<void> => {
  await api.post('/br/contacts/tip', { uid, dcrAmount, maxAttempts });
};

export type BisonrelayEventType =
  | 'pm'
  | 'kx'
  | 'gcm'
  | 'download'
  | 'kx-suggested'
  | 'tip-sent'
  | 'tip-received'
  | 'tip-failed';

export interface BisonrelayLiveEvent {
  type: BisonrelayEventType;
  payload: any;
}

export interface BisonrelayDownloadEntry {
  name: string;
  size: number;
  mtime: number;
}

export interface BisonrelayDownloadsResponse {
  files: BisonrelayDownloadEntry[] | null;
}

export const getBisonrelayDownloads = async (
  contactNick: string,
): Promise<BisonrelayDownloadEntry[]> => {
  if (!contactNick) return [];
  try {
    const { data } = await api.get<BisonrelayDownloadsResponse>(
      `/br/downloads/${encodeURIComponent(contactNick)}`,
    );
    return data?.files ?? [];
  } catch {
    return [];
  }
};
