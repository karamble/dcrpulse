import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AuthStatus } from '../../services/auth';
import { AuthGate } from './AuthGate';
import { AppPasswordFirstRun } from './AppPasswordFirstRun';
import { UnprotectedBanner } from './UnprotectedWarning';
import { SecuritySection } from '../settings/SecuritySection';
import * as auth from '../../services/auth';

vi.mock('../../services/auth', () => ({
  getAuthStatus: vi.fn(),
  setupAppPassword: vi.fn(),
  skipAppPasswordSetup: vi.fn(),
  changeAppPassword: vi.fn(),
  disableAppPassword: vi.fn(),
}));

const status = (over: Partial<AuthStatus>): AuthStatus => ({
  enabled: false,
  configured: false,
  authenticated: false,
  setupDismissed: true,
  locked: false,
  ...over,
});

const withGate = (ui: React.ReactNode) =>
  render(
    <MemoryRouter>
      <AuthGate>{ui}</AuthGate>
    </MemoryRouter>,
  );

afterEach(cleanup);

describe('UnprotectedBanner', () => {
  it('stays on screen while no app password is set', async () => {
    vi.mocked(auth.getAuthStatus).mockResolvedValue(status({}));
    withGate(<UnprotectedBanner />);
    expect(await screen.findByText(/this dashboard is unprotected/)).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Set one' }).getAttribute('href')).toBe('/wallet/settings/security');
  });

  it('is gone once a password protects the dashboard', async () => {
    vi.mocked(auth.getAuthStatus).mockResolvedValue(status({ enabled: true, configured: true, authenticated: true }));
    withGate(<><UnprotectedBanner /><span>app</span></>);
    await screen.findByText('app');
    expect(screen.queryByText(/this dashboard is unprotected/)).toBeNull();
  });

  it('does not claim the dashboard is unprotected when the status could not be read', async () => {
    vi.mocked(auth.getAuthStatus).mockImplementation(async () => {
      throw new Error('down');
    });
    withGate(<><UnprotectedBanner /><span>app</span></>);
    await screen.findByText('app');
    expect(screen.queryByText(/this dashboard is unprotected/)).toBeNull();
  });
});

describe('AppPasswordFirstRun', () => {
  it('skips only after the risk is acknowledged', async () => {
    vi.mocked(auth.skipAppPasswordSetup).mockResolvedValue();
    const onDone = vi.fn();
    render(<AppPasswordFirstRun onDone={onDone} />);
    fireEvent.click(screen.getByRole('button', { name: 'Continue without a password' }));

    const leave = screen.getByRole('button', { name: 'Leave unprotected' }) as HTMLButtonElement;
    expect(leave.disabled).toBe(true);
    fireEvent.click(leave);
    expect(auth.skipAppPasswordSetup).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('checkbox', { name: /can spend my funds/ }));
    expect(leave.disabled).toBe(false);
    fireEvent.click(leave);
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(auth.skipAppPasswordSetup).toHaveBeenCalledOnce();
  });
});

describe('SecuritySection', () => {
  it('disables the password only after the same acknowledgement', async () => {
    vi.mocked(auth.getAuthStatus).mockResolvedValue(status({ enabled: true, configured: true, authenticated: true }));
    vi.mocked(auth.disableAppPassword).mockResolvedValue();
    withGate(<SecuritySection />);
    const disable = (await screen.findByRole('button', { name: 'Disable' })) as HTMLButtonElement;
    const [, current] = screen.getAllByPlaceholderText('Current password');
    fireEvent.change(current, { target: { value: 'hunter2' } });
    expect(disable.disabled).toBe(true);

    fireEvent.click(screen.getByRole('checkbox', { name: /can spend my funds/ }));
    fireEvent.click(disable);
    await waitFor(() => expect(auth.disableAppPassword).toHaveBeenCalledWith('hunter2'));
  });
});
