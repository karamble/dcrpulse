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

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const words = Array.from({ length: 15 }, (_, i) => `seedword${i}`);

// revealAndVerify walks to the verify step and types the asked words, with
// wrong replacing the word at the given (1-based) position.
const revealAndVerify = async (wrong?: number) => {
  vi.mocked(exportDexSeed).mockResolvedValue(words.join(' '));
  fireEvent.change(screen.getByPlaceholderText('App password'), { target: { value: 'pw' } });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Reveal seed' }));
  });
  fireEvent.click(screen.getByRole('button', { name: /verify/ }));
  const inputs = screen.getAllByPlaceholderText(/^Enter word #\d+$/);
  inputs.forEach((input, i) => {
    expect(input.getAttribute('autocomplete')).toBe('off');
    expect(input.getAttribute('spellcheck')).toBe('false');
    const n = Number(input.getAttribute('placeholder')!.replace('Enter word #', ''));
    fireEvent.change(input, { target: { value: wrong !== undefined && i === 0 ? 'notaword' : words[n - 1] } });
  });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Confirm backup' }));
  });
};

// FEMONEY-10: DEX Settings keeps the flow mounted, so a recorded backup must
// end the flow and drop the seed rather than sit on "Saving...". The user sees
// it confirmed and leaves with Done.
it('confirms a recorded backup, forgets the seed, and starts over on Done', async () => {
  const onRecorded = vi.fn();
  const onDone = vi.fn();
  render(<DexSeedBackup onRecorded={onRecorded} onDone={onDone} />);
  await revealAndVerify();

  expect(markDexSeedBackedUp).toHaveBeenCalledOnce();
  expect(onRecorded).toHaveBeenCalledOnce();
  expect(onDone).not.toHaveBeenCalled();
  expect(screen.queryByText(/seedword/)).toBeNull();
  expect(screen.queryByText('Saving...')).toBeNull();
  expect(screen.getByRole('status').textContent).toContain('Seed backup verified');
  expect(screen.queryByText('Back up your recovery seed now')).toBeNull();
  const done = screen.getByRole('button', { name: 'Done' });
  expect(document.activeElement).toBe(done);

  fireEvent.click(done);
  expect(onDone).toHaveBeenCalledOnce();
  expect(screen.getByPlaceholderText('App password')).toBeTruthy();
});

// A wrong word records nothing and keeps the user on the words to retry.
it('keeps a wrong word on the verify step without recording the backup', async () => {
  const onRecorded = vi.fn();
  render(<DexSeedBackup onRecorded={onRecorded} />);
  await revealAndVerify(1);

  expect(screen.getByText(/does not match/)).toBeTruthy();
  expect(markDexSeedBackedUp).not.toHaveBeenCalled();
  expect(onRecorded).not.toHaveBeenCalled();
  expect(screen.queryByText('Seed backup verified')).toBeNull();
  expect(screen.getByRole('button', { name: 'Confirm backup' })).toBeTruthy();
});
