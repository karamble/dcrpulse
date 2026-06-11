// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import api from '../api';
import { ThemeStore } from './types';

// Theme store persists in the backend global config (/dashboard-data/config.json)
// so the active selection and custom themes follow the user across browsers.

export const getThemeStore = async (): Promise<ThemeStore> => {
  const { data } = await api.get<ThemeStore>('/themes');
  return data;
};

export const saveThemeStore = async (store: ThemeStore): Promise<void> => {
  await api.post('/themes', store);
};
