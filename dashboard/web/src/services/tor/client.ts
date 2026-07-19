// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from '../api';

export interface TorSettings {
  enabled: boolean;
  isolation: boolean;
  dcrdOnion: boolean;
  lnOnion: boolean;
  circuitLimit: number;
  rev: number;
}

export interface TorDaemonState {
  name: string;
  running: boolean;
  tor: boolean;
  torRev: string;
}

export interface TorStatus {
  settings: TorSettings;
  proxyReachable: boolean;
  onionAddress: string;
  daemons: TorDaemonState[];
}

export interface TorControl {
  reachable: boolean;
  bootstrapPct: number;
  bootstrapTag: string;
  circuits: number;
  bytesRead: number;
  bytesWritten: number;
  version: string;
  error?: string;
}

export const getTorSettings = async (): Promise<TorSettings> => {
  const { data } = await api.get<TorSettings>('/tor');
  return data;
};

export const saveTorSettings = async (settings: TorSettings): Promise<TorSettings> => {
  const { data } = await api.post<TorSettings>('/tor', settings);
  return data;
};

export const getTorStatus = async (): Promise<TorStatus> => {
  const { data } = await api.get<TorStatus>('/tor/status');
  return data;
};

export const getTorControl = async (): Promise<TorControl> => {
  const { data } = await api.get<TorControl>('/tor/control');
  return data;
};

export const torNewIdentity = async (): Promise<void> => {
  await api.post('/tor/newidentity');
};
