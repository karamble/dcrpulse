// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import type { AuthStatus } from '../../services/auth';
import { getAuthStatus } from '../../services/auth';
import { AuthGate, useAuth } from './AuthGate';

vi.mock('../../services/auth', () => ({ getAuthStatus: vi.fn(), login: vi.fn() }));
let onUnauthorized: (() => void) | null = null;
vi.mock('../../services/api', () => ({
  setUnauthorizedHandler: (fn: (() => void) | null) => {
    onUnauthorized = fn;
  },
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const open: AuthStatus = { enabled: false, configured: true, authenticated: false, setupDismissed: true, locked: false };
const NOTICE = /Couldn't check whether you're signed in/;

const App = () => {
  const { refresh } = useAuth();
  return <button onClick={() => refresh()}>app</button>;
};

// FESHELL-4: an unreadable status used to open the app as if unprotected.
it('shows the login while the status cannot be read and opens once it can', async () => {
  vi.useFakeTimers();
  vi.mocked(getAuthStatus).mockRejectedValueOnce(new Error('down')).mockResolvedValue(open);
  render(<AuthGate><App /></AuthGate>);
  await act(async () => {});
  expect(screen.getByText(NOTICE)).toBeTruthy();
  expect(screen.getByPlaceholderText('App password')).toBeTruthy();
  expect(screen.queryByText('app')).toBeNull();

  await act(async () => {
    vi.advanceTimersByTime(5000);
  });
  expect(screen.getByText('app')).toBeTruthy();
});

it('returns to the login when a later status read fails', async () => {
  vi.mocked(getAuthStatus).mockResolvedValueOnce(open).mockRejectedValue(new Error('down'));
  render(<AuthGate><App /></AuthGate>);
  await act(async () => {});
  await act(async () => {
    fireEvent.click(screen.getByText('app'));
  });
  expect(screen.getByText(NOTICE)).toBeTruthy();
  expect(screen.queryByText('app')).toBeNull();
});

it('shows the login on a gate 401 although the status said unprotected', async () => {
  vi.mocked(getAuthStatus).mockResolvedValue(open);
  render(<AuthGate><App /></AuthGate>);
  await act(async () => {});
  expect(screen.getByText('app')).toBeTruthy();
  await act(async () => {
    onUnauthorized?.();
  });
  expect(screen.getByPlaceholderText('App password')).toBeTruthy();
  expect(screen.queryByText('app')).toBeNull();
});
