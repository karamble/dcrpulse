import api from './api';

export type LightningStage =
  | 'unavailable'
  | 'needs-setup'
  | 'needs-unlock'
  | 'syncing'
  | 'ready';

export interface LightningStatus {
  stage: LightningStage;
  identityPubkey?: string;
  alias?: string;
  blockHeight?: number;
  syncedToChain?: boolean;
  syncedToGraph?: boolean;
  numActiveChans?: number;
  numPendingChans?: number;
}

export interface LightningInfo {
  identityPubkey: string;
  alias: string;
  version: string;
  blockHeight: number;
  blockHash: string;
  syncedToChain: boolean;
  syncedToGraph: boolean;
  numActiveChannels: number;
  numInactiveChannels: number;
  numPendingChannels: number;
  numPeers: number;
  bestHeaderTimestamp: number;
  chains: string[];
}

export interface LightningBalance {
  onChainConfirmed: number;
  onChainUnconfirmed: number;
  onChainTotal: number;
  channelLocal: number;
  channelRemote: number;
  channelPending: number;
}

export interface LightningActivityEntry {
  kind: 'invoice' | 'payment' | 'channel';
  timestamp: number;
  amount: number;
  state: string;
  memo?: string;
}

export interface LightningActivity {
  entries: LightningActivityEntry[];
}

export const getLightningStatus = async (): Promise<LightningStatus> => {
  const { data } = await api.get<LightningStatus>('/wallet/ln/status');
  return data;
};

export const setupLightning = async (passphrase: string): Promise<void> => {
  await api.post('/wallet/ln/setup', { passphrase });
};

export const unlockLightning = async (passphrase: string): Promise<void> => {
  await api.post('/wallet/ln/unlock', { passphrase });
};

export const getLightningInfo = async (): Promise<LightningInfo> => {
  const { data } = await api.get<LightningInfo>('/wallet/ln/info');
  return data;
};

export const getLightningBalance = async (): Promise<LightningBalance> => {
  const { data } = await api.get<LightningBalance>('/wallet/ln/balance');
  return data;
};

export const getLightningActivity = async (): Promise<LightningActivity> => {
  const { data } = await api.get<LightningActivity>('/wallet/ln/activity');
  return data;
};

// ---- Channels --------------------------------------------------------------

export type ChannelStatus =
  | 'open'
  | 'pending-open'
  | 'pending-close-coop'
  | 'pending-close-force'
  | 'pending-wait-close'
  | 'closed';

export interface LightningChannel {
  status: ChannelStatus;
  channelPoint: string;
  channelId?: number;
  remotePubkey: string;
  remoteAlias?: string;
  capacity: number;
  localBalance: number;
  remoteBalance: number;
  commitFee?: number;
  unsettledBalance?: number;
  totalSent?: number;
  totalReceived?: number;
  numUpdates?: number;
  csvDelay?: number;
  active?: boolean;
  private?: boolean;
  initiator?: boolean;
  closeType?: string;
  closingTxHash?: string;
  settledBalance?: number;
  timeLockedBalance?: number;
  limboBalance?: number;
  currentConfs?: number;
  requiredConfs?: number;
}

export interface PeerPreset {
  label: string;
  uri: string;
  isFallback?: boolean;
}

export interface OpenChannelReq {
  peerUri: string;
  localAtoms: number;
  pushAtoms?: number;
  private?: boolean;
}

export interface OpenChannelResp {
  fundingTxid: string;
  outputIndex: number;
}

export interface CloseChannelResp {
  closingTxid?: string;
}

export interface NodeMatch {
  pubkey: string;
  alias?: string;
  color?: string;
}

export interface ChannelEvent {
  type: string;
  channelPoint?: string;
  remotePubkey?: string;
}

export const getLightningChannels = async (): Promise<{ channels: LightningChannel[] }> => {
  const { data } = await api.get<{ channels: LightningChannel[] }>('/wallet/ln/channels');
  return data;
};

export const openLightningChannel = async (
  req: OpenChannelReq,
): Promise<OpenChannelResp> => {
  const { data } = await api.post<OpenChannelResp>('/wallet/ln/channels/open', req);
  return data;
};

export const closeLightningChannel = async (
  channelPoint: string,
  force: boolean,
): Promise<CloseChannelResp> => {
  const { data } = await api.post<CloseChannelResp>('/wallet/ln/channels/close', {
    channelPoint,
    force,
  });
  return data;
};

export const getLightningPeerPresets = async (): Promise<{ presets: PeerPreset[] }> => {
  const { data } = await api.get<{ presets: PeerPreset[] }>('/wallet/ln/peer-presets');
  return data;
};

export const getLightningAutopilot = async (): Promise<{ active: boolean }> => {
  const { data } = await api.get<{ active: boolean }>('/wallet/ln/autopilot');
  return data;
};

export const setLightningAutopilot = async (active: boolean): Promise<void> => {
  await api.post('/wallet/ln/autopilot', { active });
};

export const searchLightningNodes = async (
  query: string,
): Promise<{ matches: NodeMatch[] }> => {
  const { data } = await api.get<{ matches: NodeMatch[] }>(
    `/wallet/ln/graph/search?q=${encodeURIComponent(query)}`,
  );
  return data;
};

export interface LightningNetworkInfo {
  numNodes: number;
  numChannels: number;
  totalNetworkCapacity: number;
  avgChannelSize: number;
  medianChannelSize: number;
  minChannelSize: number;
  maxChannelSize: number;
  graphDiameter: number;
  avgOutDegree: number;
}

export interface TopLightningNode {
  pubkey: string;
  alias?: string;
  color?: string;
  numChannels: number;
  capacityAtoms: number;
}

export interface LightningNetworkPanel {
  info: LightningNetworkInfo;
  topNodes: TopLightningNode[];
}

export const getLightningNetwork = async (): Promise<LightningNetworkPanel> => {
  const { data } = await api.get<LightningNetworkPanel>('/wallet/ln/network');
  return data;
};

// subscribeLightningChannelEvents — opens a same-origin WebSocket to
// /api/wallet/ln/channel-events. Returns a cleanup function. The
// onEvent callback receives every dcrlnd ChannelEventUpdate.
export const subscribeLightningChannelEvents = (
  onEvent: (ev: ChannelEvent) => void,
): (() => void) => {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/api/wallet/ln/channel-events`;
  let ws: WebSocket | null = new WebSocket(url);
  ws.onmessage = (msg) => {
    try {
      onEvent(JSON.parse(msg.data) as ChannelEvent);
    } catch {
      // ignore non-JSON frames
    }
  };
  return () => {
    if (ws) {
      ws.close();
      ws = null;
    }
  };
};

// ---- Send tab --------------------------------------------------------------

export interface LightningDecodedPayReq {
  destination: string;
  paymentHash: string;
  numAtoms: number;
  timestamp: number;
  expiry: number;
  description: string;
  fallbackAddr?: string;
  cltvExpiry: number;
  paymentAddr?: string;
}

export interface LightningHop {
  pubKey: string;
  feeAtoms: number;
  amtToForward: number;
}

export interface LightningHTLC {
  status: string;
  totalAmt: number;
  totalFees: number;
  hops: LightningHop[];
}

export type LightningPaymentStatus = 'confirmed' | 'failed' | 'pending';

export interface LightningPayment {
  paymentHash: string;
  destination?: string;
  valueAtoms: number;
  feeAtoms: number;
  creationDate: number;
  status: LightningPaymentStatus;
  paymentPreimage?: string;
  paymentRequest?: string;
  description?: string;
  failureReason?: string;
  htlcs?: LightningHTLC[];
}

export interface LightningSendPaymentReq {
  payReq: string;
  amt?: number;
  feeLimitAtoms?: number;
}

// decodeLnPayReq calls the unary backend that wraps lnrpc.DecodePayReq.
export const decodeLnPayReq = async (
  payReq: string,
): Promise<LightningDecodedPayReq> => {
  const { data } = await api.post<LightningDecodedPayReq>(
    '/wallet/ln/send/decode',
    { payReq },
  );
  return data;
};

// listLnPayments fetches the wallet's payment history (newest first).
export const listLnPayments = async (): Promise<{ payments: LightningPayment[] }> => {
  const { data } = await api.get<{ payments: LightningPayment[] }>('/wallet/ln/payments');
  return data;
};

// streamLnPayment opens a WebSocket to /wallet/ln/send. The first text
// frame is the LightningSendPaymentReq; every subsequent server frame is
// either a LightningPayment snapshot or an {"error":"..."} terminal
// frame. Mirrors Decrediton's handlePaymentStream (LNActions.js:697-732).
// Returns a cleanup that closes the socket.
export const streamLnPayment = (
  req: LightningSendPaymentReq,
  onSnapshot: (snap: LightningPayment) => void,
  onError: (msg: string) => void,
  onClose: () => void,
): (() => void) => {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/api/wallet/ln/send`;
  let ws: WebSocket | null = new WebSocket(url);
  ws.onopen = () => {
    try {
      ws?.send(JSON.stringify(req));
    } catch (e) {
      onError(String(e));
    }
  };
  ws.onmessage = (msg) => {
    try {
      const frame = JSON.parse(msg.data);
      if (frame && typeof frame.error === 'string') {
        onError(frame.error);
        return;
      }
      onSnapshot(frame as LightningPayment);
    } catch {
      // ignore non-JSON frames
    }
  };
  ws.onerror = () => onError('Lightning send connection failed');
  ws.onclose = () => {
    ws = null;
    onClose();
  };
  return () => {
    if (ws) {
      ws.close();
      ws = null;
    }
  };
};

// ---- Receive tab -----------------------------------------------------------

export type LightningInvoiceStatus = 'open' | 'settled' | 'expired' | 'canceled';

export interface LightningInvoice {
  memo?: string;
  rHashHex: string;
  paymentRequest: string;
  valueAtoms: number;
  amtPaidAtoms: number;
  creationDate: number;
  settleDate?: number;
  expiry: number;
  addIndex: number;
  settleIndex?: number;
  private?: boolean;
  status: LightningInvoiceStatus;
}

export interface LightningAddInvoiceReq {
  memo?: string;
  valueAtoms: number;
  expirySec?: number;
}

export interface LightningInvoiceList {
  invoices: LightningInvoice[];
}

export const addLnInvoice = async (
  req: LightningAddInvoiceReq,
): Promise<LightningInvoice> => {
  const { data } = await api.post<LightningInvoice>('/wallet/ln/invoices/add', req);
  return data;
};

export const listLnInvoices = async (): Promise<LightningInvoiceList> => {
  const { data } = await api.get<LightningInvoiceList>('/wallet/ln/invoices');
  return data;
};

export const cancelLnInvoice = async (paymentHash: string): Promise<void> => {
  await api.post('/wallet/ln/invoices/cancel', { paymentHash });
};

// subscribeLnInvoiceEvents opens a WebSocket to /wallet/ln/invoice-events
// and invokes onSnapshot for every invoice snapshot dcrlnd emits. Returns
// a cleanup that closes the socket. Mirrors Decrediton's subscribeToInvoices.
export const subscribeLnInvoiceEvents = (
  onSnapshot: (inv: LightningInvoice) => void,
  onClose?: () => void,
): (() => void) => {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/api/wallet/ln/invoice-events`;
  let ws: WebSocket | null = new WebSocket(url);
  ws.onmessage = (msg) => {
    try {
      const frame = JSON.parse(msg.data);
      if (frame && typeof frame.error === 'string') return;
      onSnapshot(frame as LightningInvoice);
    } catch {
      // ignore non-JSON frames
    }
  };
  ws.onclose = () => {
    ws = null;
    onClose?.();
  };
  return () => {
    if (ws) {
      ws.close();
      ws = null;
    }
  };
};

// ---- Advanced tab ---------------------------------------------------------

export interface LightningChannelBackup {
  backupBase64: string;
  numChannels: number;
}

export interface LightningVerifyBackupResponse {
  ok: boolean;
  error?: string;
}

export interface LightningWatchtower {
  pubKeyHex: string;
  addresses: string[];
  numSessions: number;
  activeSessionCandidate: boolean;
}

export interface LightningNodePolicy {
  disabled: boolean;
  timeLockDelta: number;
  minHtlcAtoms: number;
  maxHtlcAtoms: number;
  lastUpdate: number;
  feeBaseMAtoms: number;
  feeRateMAtoms: number;
}

export interface LightningNodeChannel {
  channelId: number;
  chanPoint: string;
  capacity: number;
  lastUpdate: number;
  node1Pubkey: string;
  node2Pubkey: string;
  node1Policy?: LightningNodePolicy;
  node2Policy?: LightningNodePolicy;
}

export interface LightningNodeInfo {
  pubKey: string;
  alias: string;
  color: string;
  lastUpdate: number;
  totalCapacity: number;
  channels: LightningNodeChannel[];
}

export interface LightningRouteHop {
  pubKey: string;
  feeAtoms: number;
  amtToForward: number;
}

export interface LightningRoute {
  totalAmtAtoms: number;
  totalFeesAtoms: number;
  hops: LightningRouteHop[];
}

export interface LightningQueryRoutesResponse {
  successProb: number;
  routes: LightningRoute[];
}

export const getLnChannelBackup = async (): Promise<LightningChannelBackup> => {
  const { data } = await api.get<LightningChannelBackup>('/wallet/ln/backup');
  return data;
};

export const verifyLnChannelBackup = async (
  backupBase64: string,
): Promise<LightningVerifyBackupResponse> => {
  const { data } = await api.post<LightningVerifyBackupResponse>(
    '/wallet/ln/backup/verify',
    { backupBase64 },
  );
  return data;
};

export const listLnWatchtowers = async (): Promise<{ towers: LightningWatchtower[] }> => {
  const { data } = await api.get<{ towers: LightningWatchtower[] }>('/wallet/ln/watchtowers');
  return data;
};

export const addLnWatchtower = async (pubKeyHex: string, address: string): Promise<void> => {
  await api.post('/wallet/ln/watchtowers/add', { pubKeyHex, address });
};

export const removeLnWatchtower = async (pubKeyHex: string): Promise<void> => {
  await api.post('/wallet/ln/watchtowers/remove', { pubKeyHex });
};

export const queryLnNode = async (pubkey: string): Promise<LightningNodeInfo> => {
  const { data } = await api.get<LightningNodeInfo>('/wallet/ln/graph/node', { params: { pubkey } });
  return data;
};

export const queryLnRoutes = async (
  pubKey: string,
  amtAtoms: number,
): Promise<LightningQueryRoutesResponse> => {
  const { data } = await api.post<LightningQueryRoutesResponse>(
    '/wallet/ln/graph/routes',
    { pubKey, amtAtoms },
  );
  return data;
};
