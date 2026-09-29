// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, render } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { BlockDetail } from './BlockDetail';
import { getBlockByHash, getBlockByHeight } from '../services/explorerApi';

vi.mock('../services/explorerApi', () => ({
  getBlockByHeight: vi.fn(() => new Promise(() => {})),
  getBlockByHash: vi.fn(() => new Promise(() => {})),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const open = async (param: string) => {
  render(
    <MemoryRouter initialEntries={[`/explorer/block/${param}`]}>
      <Routes>
        <Route path="/explorer/block/:heightOrHash" element={<BlockDetail />} />
      </Routes>
    </MemoryRouter>,
  );
  await act(async () => {});
};

// FESHELL-15: parseInt read this hash as height 1.
it('opens a block by the hash in its URL', async () => {
  const hash = '00000000000000001a' + 'b'.repeat(46);
  await open(hash);
  expect(getBlockByHash).toHaveBeenCalledWith(hash);
  expect(getBlockByHeight).not.toHaveBeenCalled();
});

it('opens a block by the height in its URL', async () => {
  await open('12');
  expect(getBlockByHeight).toHaveBeenCalledWith(12);
  expect(getBlockByHash).not.toHaveBeenCalled();
});
