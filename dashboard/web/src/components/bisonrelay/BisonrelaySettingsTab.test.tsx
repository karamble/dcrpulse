// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { BisonrelaySettingsTab } from './BisonrelaySettingsTab';
import * as api from '../../services/bisonrelayApi';

vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...(await original<typeof import('../../services/bisonrelayApi')>()),
  getBisonrelayFilters: vi.fn(async () => []),
  getBisonrelayContacts: vi.fn(async () => []),
  listBisonrelayGCs: vi.fn(async () => []),
  upsertBisonrelayFilter: vi.fn(async () => ({})),
}));

afterEach(cleanup);

// FEBR-13: the Bison Relay client checks filter patterns with Go's regexp when
// they are saved, so a Go pattern the browser's engine rejects must go through.
it('saves a case-insensitive Go filter pattern', async () => {
  window.location.hash = '#settings/filters';
  render(<BisonrelaySettingsTab />);
  fireEvent.click(await screen.findByRole('button', { name: /Add filter/ }));
  fireEvent.change(screen.getByPlaceholderText('e.g. spam|casino'), { target: { value: '(?i)casino' } });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /Save filter/ }));
  });
  expect(api.upsertBisonrelayFilter).toHaveBeenCalledTimes(1);
  expect(vi.mocked(api.upsertBisonrelayFilter).mock.calls[0][0].regexp).toBe('(?i)casino');
});
