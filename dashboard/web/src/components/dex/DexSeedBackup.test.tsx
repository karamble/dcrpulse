// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { DexSeedBackup } from './DexSeedBackup';
import { exportDexSeed, markDexSeedBackedUp } from '../../services/dcrdexApi';

vi.mock('../../services/dcrdexApi', () => ({
  exportDexSeed: vi.fn(),
  markDexSeedBackedUp: vi.fn(async () => {}),
}));

afterEach(cleanup);

const words = Array.from({ length: 15 }, (_, i) => `seedword${i}`);

// FEMONEY-10: DEX Settings keeps the flow mounted, so a recorded backup must
// end the flow and drop the seed rather than sit on "Saving...".
it('returns to the start and forgets the seed once the backup is recorded', async () => {
  vi.mocked(exportDexSeed).mockResolvedValue(words.join(' '));
  const onDone = vi.fn();
  render(<DexSeedBackup onDone={onDone} />);

  fireEvent.change(screen.getByPlaceholderText('App password'), { target: { value: 'pw' } });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Reveal seed' }));
  });
  fireEvent.click(screen.getByRole('button', { name: /verify/ }));
  const inputs = screen.getAllByPlaceholderText(/^Enter word #\d+$/);
  for (const input of inputs) {
    expect(input.getAttribute('autocomplete')).toBe('off');
    expect(input.getAttribute('spellcheck')).toBe('false');
    const n = Number(input.getAttribute('placeholder')!.replace('Enter word #', ''));
    fireEvent.change(input, { target: { value: words[n - 1] } });
  }
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Confirm backup' }));
  });

  expect(markDexSeedBackedUp).toHaveBeenCalledOnce();
  expect(onDone).toHaveBeenCalledOnce();
  expect(screen.queryByText(/seedword/)).toBeNull();
  expect(screen.queryByText('Saving...')).toBeNull();
  expect(screen.getByPlaceholderText('App password')).toBeTruthy();
});
