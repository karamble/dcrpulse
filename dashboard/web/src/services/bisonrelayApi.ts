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
  posts_subscribed?: boolean;
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
  // payment or a posts subscribe/unsubscribe request) is in flight.
  // pending=true renders a spinner next to the text. tipKey / subKey
  // correlate the placeholder with the eventual live event so we can
  // replace it in place.
  pending?: boolean;
  tipKey?: string;
  subKey?: string;
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

export const subscribeBisonrelayPosts = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/subscribe-posts', { uid });
};

export const unsubscribeBisonrelayPosts = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/unsubscribe-posts', { uid });
};

export interface BisonrelayPostListItem {
  id: string;
  title: string;
  timestamp: number;
}

export const listBisonrelayUserPosts = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/list-posts', { uid });
};

export const fetchBisonrelayUserPost = async (uid: string, pid: string): Promise<void> => {
  await api.post('/br/contacts/fetch-post', { uid, pid });
};

export interface BisonrelayPostSummary {
  id: string;
  from: string;
  author_id: string;
  author_nick: string;
  date: number;
  last_status_ts: number;
  title: string;
}

export interface BisonrelayPostBodySegment {
  kind: 'text' | 'embed';
  html?: string;
  name?: string;
  mime?: string;
  data_b64?: string;
  size?: number;
  alt?: string;
}

export interface BisonrelayPostBody {
  title: string;
  markdown: string;
  segments: BisonrelayPostBodySegment[] | null;
  attributes: Record<string, string>;
}

export const getBisonrelayPosts = async (): Promise<BisonrelayPostSummary[]> => {
  const { data } = await api.get<{ posts: BisonrelayPostSummary[] | null }>('/br/posts');
  return data.posts ?? [];
};

export interface BisonrelayIdentity {
  nick?: string;
  name?: string;
  identity?: string;
  avatar?: string;
}

export const getBisonrelayIdentity = async (): Promise<BisonrelayIdentity> => {
  const { data } = await api.get<BisonrelayIdentity>('/br/identity');
  return data;
};

export const createBisonrelayPost = async (
  post: string,
  descr?: string,
): Promise<BisonrelayPostSummary> => {
  const { data } = await api.post<BisonrelayPostSummary>('/br/posts/new', {
    post,
    descr: descr ?? '',
  });
  return data;
};

export interface BisonrelaySharedFile {
  fid: string;
  filename: string;
  cost: number; // milliatoms
  size: number;
  global: boolean;
}

export const getBisonrelaySharedFiles = async (): Promise<BisonrelaySharedFile[]> => {
  const { data } = await api.get<{ files: BisonrelaySharedFile[] | null }>(
    '/br/shared-files',
  );
  return data.files ?? [];
};

// renderBisonrelayPostBody runs the editor's draft markdown through the
// same server-side renderer the Feed detail view uses, so the Preview
// tab matches exactly. Returns the segmented body shape.
export const renderBisonrelayPostBody = async (
  post: string,
  title?: string,
): Promise<BisonrelayPostBody> => {
  const { data } = await api.post<BisonrelayPostBody>('/br/posts/render', {
    post,
    title: title ?? '',
  });
  return data;
};

export const getBisonrelayPostBody = async (
  uid: string,
  pid: string,
): Promise<BisonrelayPostBody> => {
  const { data } = await api.get<BisonrelayPostBody>('/br/posts/body', {
    params: { uid, pid },
  });
  return data;
};

export interface BisonrelayPostComment {
  status_from: string;
  from_nick: string;
  comment: string;
  parent?: string;
  timestamp: number;
  identifier?: string;
  // Client-side fields used while a comment is in flight.
  pending?: boolean;
  commentKey?: string;
}

export const getBisonrelayPostComments = async (
  uid: string,
  pid: string,
): Promise<BisonrelayPostComment[]> => {
  const { data } = await api.get<{ comments: BisonrelayPostComment[] | null }>(
    '/br/posts/comments',
    { params: { uid, pid } },
  );
  return data.comments ?? [];
};

export const postBisonrelayComment = async (
  uid: string,
  pid: string,
  comment: string,
  parent?: string,
): Promise<{ identifier: string }> => {
  const { data } = await api.post<{ identifier: string }>('/br/posts/comment', {
    uid,
    pid,
    comment,
    parent: parent ?? '',
  });
  return data;
};

export interface BisonrelayContentItem {
  file_id: string;
  filename: string;
  size: number;
  directory: string;
  description: string;
  cost: number;
  downloaded: boolean;
}

export const listBisonrelayUserContent = async (uid: string): Promise<void> => {
  await api.post('/br/contacts/list-content', { uid });
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
  | 'tip-failed'
  | 'posts-subscribed'
  | 'posts-unsubscribed'
  | 'posts-subscriber-updated'
  | 'posts-list-received'
  | 'content-list-received'
  | 'post-received'
  | 'post-status-received'
  | 'file-download-progress'
  | 'file-download-completed';

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

// Files-tab bindings: share-add, unshare, list-downloads, cancel-download.
// Each is a thin proxy over the matching brclientd /shared-files or
// /downloads route — see internal/handlers/bisonrelay.go.

export interface BisonrelayShareFileResult {
  fid: string;
  filename: string;
  cost: number;
  size: number;
  global: boolean;
}

export const shareBisonrelayFile = async (
  file: File,
  costDcr: number,
  targetUid: string,
  descr: string,
): Promise<BisonrelayShareFileResult> => {
  const form = new FormData();
  form.append('file', file, file.name);
  if (costDcr > 0) form.append('cost_dcr', String(costDcr));
  if (targetUid) form.append('target_uid', targetUid);
  if (descr) form.append('descr', descr);
  const { data } = await api.post<BisonrelayShareFileResult>(
    '/br/files/add',
    form,
    { headers: { 'Content-Type': 'multipart/form-data' } },
  );
  return data;
};

export const unshareBisonrelayFile = async (
  fid: string,
  targetUid?: string,
): Promise<void> => {
  await api.post('/br/files/shared/remove', { fid, target_uid: targetUid ?? '' });
};

export interface BisonrelayDownloadItem {
  uid: string;
  nick: string;
  fid: string;
  filename: string;
  size: number;
  total_chunks: number;
  missing_chunks: number;
  disk_path: string;
  is_sent: boolean;
}

export const getBisonrelayManageDownloads = async (): Promise<BisonrelayDownloadItem[]> => {
  const { data } = await api.get<{ downloads: BisonrelayDownloadItem[] | null }>(
    '/br/files/downloads',
  );
  return data?.downloads ?? [];
};

export const cancelBisonrelayDownload = async (fid: string): Promise<void> => {
  await api.post('/br/files/downloads/cancel', { fid });
};

// Stats bindings: each endpoint is a thin pass-through over the matching
// brclientd /stats/* route. Values denominated in milliatoms (1 DCR = 1e11
// matoms) on the wire; the UI converts at render time.

export interface BisonrelayStatsTopContact {
  uid: string;
  nick: string;
  sent_atoms: number;
  received_atoms: number;
}

export interface BisonrelayStatsOverview {
  nick: string;
  identity: string;
  stage: string;
  connected_at?: string;
  server_node?: string;
  contacts_count: number;
  posts_authored: number;
  subscriptions_count: number;
  subscribers_count: number;
  total_sent_atoms: number;
  total_received_atoms: number;
  total_fees_atoms: number;
  rmq_p50_ns: number;
  top_contacts: BisonrelayStatsTopContact[];
}

export const getBisonrelayStatsOverview = async (): Promise<BisonrelayStatsOverview> => {
  const { data } = await api.get<BisonrelayStatsOverview>('/br/stats/overview');
  return data;
};

export interface BisonrelayPayStatsBreakdown {
  prefix: string;
  total: number;
}

export interface BisonrelayPayStatsUser {
  uid: string;
  nick: string;
  sent_atoms: number;
  received_atoms: number;
  fees_atoms: number;
  breakdowns?: BisonrelayPayStatsBreakdown[];
}

export interface BisonrelayQuantile {
  rel: string;
  n: number;
  max_ns: number;
}

export interface BisonrelayStatsPayments {
  users: BisonrelayPayStatsUser[];
  rmq_rtt_quantiles: BisonrelayQuantile[];
}

export const getBisonrelayStatsPayments = async (): Promise<BisonrelayStatsPayments> => {
  const { data } = await api.get<BisonrelayStatsPayments>('/br/stats/payments');
  return data;
};

export interface BisonrelayServerPolicy {
  push_pay_rate_matoms: number;
  push_pay_rate_bytes: number;
  push_pay_rate_min_matoms: number;
  max_push_invoices: number;
  max_msg_size: number;
  expiration_days: number;
}

export interface BisonrelayStatsNetwork {
  server_node?: string;
  recommended_peer?: string;
  connected_at?: string;
  stage: string;
  policy: BisonrelayServerPolicy;
  rmq_quantiles: BisonrelayQuantile[];
}

export const getBisonrelayStatsNetwork = async (): Promise<BisonrelayStatsNetwork> => {
  const { data } = await api.get<BisonrelayStatsNetwork>('/br/stats/network');
  return data;
};

export interface BisonrelayRatchetInfo {
  nb_saved_keys: number;
  will_ratchet: boolean;
  last_enc_time?: string;
  last_dec_time?: string;
  send_rv_plain?: string;
  recv_rv_plain?: string;
  drain_rv_plain?: string;
}

export interface BisonrelayStatsContact {
  uid: string;
  nick: string;
  nick_alias?: string;
  first_created: string;
  last_completed_kx: string;
  last_handshake_attempt?: string;
  ignored: boolean;
  ratchet?: BisonrelayRatchetInfo;
}

export const getBisonrelayStatsContacts = async (): Promise<BisonrelayStatsContact[]> => {
  const { data } = await api.get<{ contacts: BisonrelayStatsContact[] | null }>(
    '/br/stats/contacts',
  );
  return data?.contacts ?? [];
};

export interface BisonrelayAuthoredPostStats {
  pid: string;
  title: string;
  date: string;
  last_status_ts?: string;
  hearts: number;
  comments: number;
}

export interface BisonrelayStatsPosts {
  authored: BisonrelayAuthoredPostStats[];
  subscribers_count: number;
  subscriptions_count: number;
}

export const getBisonrelayStatsPosts = async (): Promise<BisonrelayStatsPosts> => {
  const { data } = await api.get<BisonrelayStatsPosts>('/br/stats/posts');
  return data;
};
