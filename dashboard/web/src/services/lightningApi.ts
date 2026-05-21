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
