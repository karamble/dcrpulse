// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';

vi.mock('../../services/auth', () => ({
  setupAppPassword: vi.fn(),
  changeAppPassword: vi.fn(),
  disableAppPassword: vi.fn(),
}));
vi.mock('../auth/AuthGate', () => ({
  useAuth: () => ({ status: { enabled: true }, refresh: async () => {} }),
}));
vi.mock('../auth/UnprotectedWarning', () => ({ UnprotectedWarning: () => null }));

import { changeAppPassword } from '../../services/auth';
import { SecuritySection } from './SecuritySection';

afterEach(cleanup);

// The server explains each refusal in the reply body; the screen must show it
// rather than a generic status line or a blanket "incorrect password".
const reject = (status: number, body: string) =>
  Object.assign(new Error(`Request failed with status code ${status}`), { response: { status, data: body } });

const submitChange = () => {
  const [current] = screen.getAllByPlaceholderText('Current password');
  const newFields = screen.getAllByPlaceholderText('New password');
  const next = newFields[newFields.length - 1];
  fireEvent.change(current, { target: { value: 'old' } });
  fireEvent.change(next, { target: { value: 'new' } });
  fireEvent.click(screen.getByRole('button', { name: 'Change' }));
};

it('shows why the server refused the change', async () => {
  vi.mocked(changeAppPassword).mockRejectedValue(reject(400, 'new password must not be empty'));
  render(<SecuritySection />);
  submitChange();
  expect(await screen.findByText('new password must not be empty')).not.toBeNull();
});

it('shows the lockout and its retry time', async () => {
  vi.mocked(changeAppPassword).mockRejectedValue(reject(429, 'too many failed attempts: retry in 42s'));
  render(<SecuritySection />);
  submitChange();
  expect(await screen.findByText('too many failed attempts: retry in 42s')).not.toBeNull();
});
