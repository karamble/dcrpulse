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
  cost: number; // atoms (1 DCR = 1e8)
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

// renderBisonrelayPageBody runs draft page markdown through the same
// SplitAndRenderBRPage the Pages viewer uses, so the editor's page Preview
// matches a hosted page (forms, sections, br:// links). Dashboard-only.
export const renderBisonrelayPageBody = async (
  markdown: string,
): Promise<{ markdown: string; segments: BisonrelayPageSegment[] | null }> => {
  const { data } = await api.post<{ markdown: string; segments: BisonrelayPageSegment[] | null }>(
    '/br/pages/render',
    { markdown },
  );
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

export interface BisonrelayPostHearts {
  count: number;
  hearted_by_me: boolean;
}

export const getBisonrelayPostHearts = async (
  uid: string,
  pid: string,
): Promise<BisonrelayPostHearts> => {
  const { data } = await api.get<BisonrelayPostHearts>('/br/posts/hearts', {
    params: { uid, pid },
  });
  return data;
};

export const heartBisonrelayPost = async (
  uid: string,
  pid: string,
  heart: boolean,
): Promise<void> => {
  await api.post('/br/posts/heart', { uid, pid, heart });
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
  | 'post-heart-received'
  | 'file-download-progress'
  | 'file-download-completed'
  | 'rtdt-invited'
  | 'rtdt-invite-accepted'
  | 'rtdt-invite-canceled'
  | 'rtdt-session-updated'
  | 'rtdt-live-joined'
  | 'rtdt-allowance-refreshed'
  | 'rtdt-peer-joined'
  | 'rtdt-peer-stalled'
  | 'rtdt-send-error'
  | 'rtdt-hot-audio'
  | 'rtdt-peer-sound-changed'
  | 'rtdt-peer-exited'
  | 'rtdt-kicked'
  | 'rtdt-dissolved'
  | 'rtdt-removed'
  | 'rtdt-cookies-rotated'
  | 'rtdt-chat'
  | 'rtdt-admin-cookies'
  | 'rtdt-rtt'
  | 'rtdt-joined-instant-call'
  | 'gc-message'
  | 'gc-invited'
  | 'gc-joined'
  | 'gc-invite-accepted'
  | 'gc-members-added'
  | 'gc-members-removed'
  | 'gc-parted'
  | 'gc-killed'
  | 'gc-upgraded'
  | 'gc-admins-changed'
  | 'gc-version-warning'
  | 'gc-unkxd-member';

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

// startBisonrelayContentGet initiates a download of a shared file (FID) that a
// page advertised via --embed[download=<fid>,cost=,...]--. BR auto-pays any
// per-chunk cost; poll getBisonrelayManageDownloads to track completion, then
// load the bytes from bisonrelayContentFileUrl.
export const startBisonrelayContentGet = async (uid: string, fid: string): Promise<void> => {
  await api.post('/br/content/get', { uid, fid });
};

// bisonrelayContentFileUrl is the same-origin URL the browser loads (as an
// <img> src or a download link) for a fully-downloaded shared file.
export const bisonrelayContentFileUrl = (fid: string, uid?: string): string => {
  const q = new URLSearchParams({ fid });
  if (uid) q.set('uid', uid);
  return `/api/br/content/file?${q.toString()}`;
};

export interface BisonrelayRates {
  dcr_usd: number;
  btc_usd: number;
  source: string; // "bisonrelay" | "kraken" | "" (none yet)
  updated_at: string; // RFC3339; empty until a rate is first obtained
}

let ratesCache: { at: number; data: BisonrelayRates } | null = null;
const RATES_CACHE_MS = 2 * 60 * 1000;

// getBisonrelayRates returns the current DCR/USD (+ BTC/USD) rate, memoized for
// a couple of minutes so many download embeds on one page share a single
// request. brclientd already throttles the upstream sources behind this.
export const getBisonrelayRates = async (): Promise<BisonrelayRates> => {
  if (ratesCache && Date.now() - ratesCache.at < RATES_CACHE_MS) {
    return ratesCache.data;
  }
  const { data } = await api.get<BisonrelayRates>('/br/rates');
  ratesCache = { at: Date.now(), data };
  return data;
};

// Stats bindings: each endpoint is a thin pass-through over the matching
// brclientd /stats/* route. Values denominated in milliatoms (1 DCR = 1e11
// matoms) on the wire; the UI converts at render time.

export interface BisonrelayStatsTopContact {
  uid: string;
  nick: string;
  sent_matoms: number;
  received_matoms: number;
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
  total_sent_matoms: number;
  total_received_matoms: number;
  total_fees_matoms: number;
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
  sent_matoms: number;
  received_matoms: number;
  fees_matoms: number;
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

// ---- RTDT realtime-voice control plane ----------------------------------

export interface RTDTSessionMember {
  uid: string;
  peer_id: number;
  publisher: boolean;
  accepted: boolean;
}

export interface RTDTSessionPublisher {
  uid: string;
  peer_id: number;
  alias: string;
}

export interface RTDTLivePeer {
  peer_id: number;
  has_sound_stream: boolean;
  has_sound: boolean;
}

export interface RTDTSession {
  rv: string;
  description: string;
  size: number;
  owner: string;
  is_instant: boolean;
  local_peer_id: number;
  is_admin: boolean;
  live: boolean;
  hot_audio: boolean;
  members: RTDTSessionMember[];
  publishers: RTDTSessionPublisher[];
  live_peers?: RTDTLivePeer[];
}

export const listRTDTSessions = async (): Promise<RTDTSession[]> => {
  const { data } = await api.get<{ sessions: RTDTSession[] | null }>('/br/rtdt/sessions');
  return data?.sessions ?? [];
};

export const createRTDTSession = async (size: number, description: string): Promise<RTDTSession> => {
  const { data } = await api.post<RTDTSession>('/br/rtdt/sessions/create', { size, description });
  return data;
};

export const createInstantRTDTSession = async (uids: string[]): Promise<RTDTSession> => {
  const { data } = await api.post<RTDTSession>('/br/rtdt/sessions/create-instant', { uids });
  return data;
};

export const inviteToRTDTSession = async (rv: string, uids: string[], asPublisher: boolean): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/invite`, { uids, as_publisher: asPublisher });
};

export const acceptRTDTSession = async (rv: string, inviter: string, asPublisher: boolean): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/accept`, { inviter, as_publisher: asPublisher });
};

export const joinRTDTSession = async (rv: string): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/join`, {});
};

export const leaveRTDTSession = async (rv: string): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/leave`, {});
};

export const dissolveRTDTSession = async (rv: string): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/dissolve`, {});
};

export const kickRTDTPeer = async (rv: string, peerID: number, banSeconds: number): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/kick`, { peer_id: peerID, ban_seconds: banSeconds });
};

export const removeRTDTMember = async (rv: string, uid: string, reason: string): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/remove`, { uid, reason });
};

export const rotateRTDTCookies = async (rv: string): Promise<void> => {
  await api.post(`/br/rtdt/sessions/${rv}/rotate-cookies`, {});
};

// ---- GC (group-chat) control plane --------------------------------------

export interface BisonrelayGC {
  id: string;
  name: string;
  alias?: string;
  generation: number;
  version: number;
  owner: string;
  members: string[];
  extra_admins?: string[];
  blocked?: string[];
  local_is_owner: boolean;
  local_is_admin: boolean;
}

export interface BisonrelayGCInvite {
  id: number;
  gcid: string;
  name: string;
  description?: string;
  from: string;
  expires: number;
  version: number;
  accepted: boolean;
}

export const listBisonrelayGCs = async (): Promise<BisonrelayGC[]> => {
  const { data } = await api.get<{ gcs: BisonrelayGC[] | null }>('/br/gc');
  return data?.gcs ?? [];
};

export const createBisonrelayGC = async (name: string): Promise<BisonrelayGC> => {
  const { data } = await api.post<BisonrelayGC>('/br/gc/create', { name });
  return data;
};

export const listBisonrelayGCInvites = async (): Promise<BisonrelayGCInvite[]> => {
  const { data } = await api.get<{ invites: BisonrelayGCInvite[] | null }>('/br/gc/invites');
  return data?.invites ?? [];
};

export const acceptBisonrelayGCInvite = async (iid: number): Promise<void> => {
  await api.post('/br/gc/invites/accept', { iid });
};

export const getBisonrelayGCDetail = async (gcid: string): Promise<BisonrelayGC> => {
  const { data } = await api.get<BisonrelayGC>(`/br/gc/${gcid}`);
  return data;
};

export const inviteToBisonrelayGC = async (gcid: string, uid: string): Promise<void> => {
  await api.post(`/br/gc/${gcid}/invite`, { uid });
};

// GC message send mirrors the PM shape (msg + optional embed). Returns
// the synthesised wire body so the caller can optimistically echo.
export const sendBisonrelayGCMessage = async (
  gcid: string,
  msg: string,
  embed?: BisonrelayPMAttachment,
): Promise<{ body: string }> => {
  const { data } = await api.post<{ body: string }>(`/br/gc/${gcid}/message`, {
    msg,
    mode: 0,
    embed,
  });
  return data;
};

export interface BisonrelayGCHistory {
  gcid: string;
  page: number;
  page_size: number;
  entries: BisonrelayMessage[];
}

export const getBisonrelayGCHistory = async (
  gcid: string,
  page = 0,
  pageSize = 100,
): Promise<BisonrelayGCHistory> => {
  const { data } = await api.get<BisonrelayGCHistory>(`/br/gc/${gcid}/history`, {
    params: { page, page_size: pageSize },
  });
  return data;
};

export const partBisonrelayGC = async (gcid: string, reason = ''): Promise<void> => {
  await api.post(`/br/gc/${gcid}/part`, { reason });
};

export const killBisonrelayGC = async (gcid: string, reason = ''): Promise<void> => {
  await api.post(`/br/gc/${gcid}/kill`, { reason });
};

export const kickFromBisonrelayGC = async (
  gcid: string,
  uid: string,
  reason = '',
): Promise<void> => {
  await api.post(`/br/gc/${gcid}/kick`, { uid, reason });
};

export const blockInBisonrelayGC = async (gcid: string, uid: string): Promise<void> => {
  await api.post(`/br/gc/${gcid}/block`, { uid });
};

export const unblockInBisonrelayGC = async (gcid: string, uid: string): Promise<void> => {
  await api.post(`/br/gc/${gcid}/unblock`, { uid });
};

export const modifyBisonrelayGCAdmins = async (
  gcid: string,
  extraAdmins: string[],
  reason = '',
): Promise<void> => {
  await api.post(`/br/gc/${gcid}/admins`, { extra_admins: extraAdmins, reason });
};

export const modifyBisonrelayGCOwner = async (
  gcid: string,
  newOwner: string,
  reason = '',
): Promise<void> => {
  await api.post(`/br/gc/${gcid}/owner`, { new_owner: newOwner, reason });
};

export const upgradeBisonrelayGCVersion = async (
  gcid: string,
  newVersion: number,
): Promise<void> => {
  await api.post(`/br/gc/${gcid}/upgrade`, { new_version: newVersion });
};

export const aliasBisonrelayGC = async (gcid: string, alias: string): Promise<void> => {
  await api.post(`/br/gc/${gcid}/alias`, { alias });
};

export const resendBisonrelayGCList = async (gcid: string, uid?: string): Promise<void> => {
  await api.post(`/br/gc/${gcid}/resend-list`, uid ? { uid } : {});
};

// ---- Pages ---------------------------------------------------------------
// BR "pages" are markdown resources a user hosts and others fetch over the
// relay. The dashboard renders fetched pages server-side into structured
// segments (see services/markdown.go SplitAndRenderBRPage).

export interface BisonrelayPageFormField {
  type: string; // txtinput | intinput | submit | action | asynctarget | hidden
  name?: string;
  label?: string;
  hint?: string;
  value?: string;
  regexp?: string;
  regexpstr?: string;
}

export interface BisonrelayPageSegment {
  kind: 'text' | 'embed' | 'form';
  section_id?: string;
  html?: string;
  name?: string;
  mime?: string;
  data_b64?: string;
  size?: number;
  alt?: string;
  // File-transfer embed (--embed[download=<fid>,cost=,filename=,...]--): the
  // bytes are fetched over BR file transfer (paying cost, in milli-atoms),
  // not delivered inline in data_b64.
  download?: string;
  cost?: number;
  filename?: string;
  fields?: BisonrelayPageFormField[];
}

export interface BisonrelayFetchedPage {
  session_id: number;
  page_id: number;
  parent_page: number;
  status: number; // 200 Ok, 404 NotFound, 400 BadRequest
  meta?: Record<string, string>;
  markdown: string;
  async_target_id?: string;
  segments: BisonrelayPageSegment[] | null;
}

export interface BisonrelayPageFetchRequest {
  uid: string;
  path: string[];
  session_id?: number;
  parent_page?: number;
  data?: Record<string, unknown>;
  async_target_id?: string;
}

export const fetchBisonrelayPage = async (
  req: BisonrelayPageFetchRequest,
): Promise<BisonrelayFetchedPage> => {
  const { data } = await api.post<BisonrelayFetchedPage>('/br/pages/fetch', req);
  return data;
};

export interface BisonrelayLocalPage {
  name: string;
  size: number;
  modified: number;
}

export const listBisonrelayLocalPages = async (): Promise<BisonrelayLocalPage[]> => {
  const { data } = await api.get<{ pages: BisonrelayLocalPage[] | null }>('/br/pages/local');
  return data?.pages ?? [];
};

export const getBisonrelayLocalPage = async (name: string): Promise<string> => {
  const { data } = await api.get<{ name: string; content: string }>('/br/pages/local/file', {
    params: { name },
  });
  return data?.content ?? '';
};

export const saveBisonrelayLocalPage = async (name: string, content: string): Promise<void> => {
  await api.post('/br/pages/local/save', { name, content });
};

export const deleteBisonrelayLocalPage = async (name: string): Promise<void> => {
  await api.post('/br/pages/local/delete', { name });
};
