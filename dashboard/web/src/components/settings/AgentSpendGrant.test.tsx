// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../services/api';
import { AgentSpendGrant } from './AgentSpendGrant';

vi.mock('../../services/api', async (original) => ({
  ...(await original<typeof import('../../services/api')>()),
  setMCPGrant: vi.fn(async () => undefined),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const account = { accountName: 'default', accountNumber: 0 } as api.AccountInfo;
const grant = (expiry?: string): api.MCPGrant => ({
  accounts: [0],
  perTxDcr: 5,
  dailyDcr: 20,
  spentTodayDcr: 0,
  remainingTodayDcr: 20,
  allowlist: [],
  expiry,
  writeScopes: [],
});

const editAndSave = async (g: api.MCPGrant) => {
  render(<AgentSpendGrant agentId="a1" grant={g} accounts={[account]} scopes={[]} onChanged={() => {}} />);
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  fireEvent.change(screen.getByLabelText('Per-transaction cap (DCR)'), { target: { value: '2' } });
  fireEvent.change(screen.getByLabelText('Wallet passphrase'), { target: { value: 'pw' } });
  fireEvent.click(screen.getByRole('button', { name: 'Update grant' }));
  await waitFor(() => expect(api.setMCPGrant).toHaveBeenCalledTimes(1));
  return vi.mocked(api.setMCPGrant).mock.calls[0][1];
};

// An edit replaces the whole grant, and an empty expiry means none, so an edit
// that leaves the field alone must keep the deadline the operator set.
describe('AgentSpendGrant edit', () => {
  it('keeps a time-boxed grant time-boxed', async () => {
    const body = await editAndSave(grant(new Date(Date.now() + 6 * 3600e3).toISOString()));
    expect(body.perTxDcr).toBe(2);
    expect(body.expiryHours).toBeGreaterThan(5.9);
    expect(body.expiryHours).toBeLessThanOrEqual(6.01);
  });

  it('never turns a grant past its deadline into a permanent one', async () => {
    const body = await editAndSave(grant(new Date(Date.now() - 60e3).toISOString()));
    expect(body.expiryHours).toBe(0.01);
  });

  it('leaves a grant without expiry without one', async () => {
    const body = await editAndSave(grant());
    expect(body.expiryHours).toBe(0);
  });
});
