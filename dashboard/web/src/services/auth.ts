// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from './api';

export interface AuthStatus {
  enabled: boolean;
  configured: boolean;
  authenticated: boolean;
  setupDismissed: boolean;
}

export const getAuthStatus = async (): Promise<AuthStatus> => {
  const { data } = await api.get<AuthStatus>('/auth/status');
  return data;
};

export const login = async (password: string): Promise<void> => {
  await api.post('/auth/login', { password });
};

export const setupAppPassword = async (password: string): Promise<void> => {
  await api.post('/auth/setup', { password });
};

export const skipAppPasswordSetup = async (): Promise<void> => {
  await api.post('/auth/skip-setup');
};

export const logout = async (): Promise<void> => {
  await api.post('/auth/logout');
};

export const changeAppPassword = async (current: string, next: string): Promise<void> => {
  await api.post('/auth/change', { current, new: next });
};

export const disableAppPassword = async (current: string): Promise<void> => {
  await api.post('/auth/disable', { current });
};
