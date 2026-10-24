import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { saveSettings } from '../../services/api';
import { PrivacySection, savedText } from './PrivacySection';

vi.mock('../../services/api', () => ({
  getSettings: async () => ({
    global: { externalRequests: { vspListing: true, politeia: true, brseeder: true, exchangeRates: true }, decredPulseBotUrl: '' },
  }),
  getMixerDebug: async () => ({ enabled: false }),
  setMixerDebug: vi.fn(),
  saveSettings: vi.fn(async () => ({ notApplied: ['dcrdex'] })),
}));

afterEach(cleanup);

describe('savedText', () => {
  it('says what still waits on a daemon', () => {
    expect(savedText([])).toBe('Preferences saved.');
    expect(savedText(['brclientd', 'dcrdex'])).toBe(
      'Preferences saved. Bison Relay is not running; start it and save again. DCRDEX picks this up the next time it is unlocked.',
    );
  });
});

describe('Exchange rates toggle', () => {
  it('saves exchange rates off and shows what is still pending', async () => {
    render(<PrivacySection />);
    const row = (await screen.findByText('Exchange rates')).closest('div')!.parentElement!;
    const button = row.querySelector('button')!;
    await waitFor(() => expect(button.textContent).toBe('On'));
    fireEvent.click(button);
    await waitFor(() => expect(saveSettings).toHaveBeenCalled());
    const sent = vi.mocked(saveSettings).mock.calls[0][0];
    expect(sent.global?.externalRequests).toEqual({ vspListing: true, politeia: true, brseeder: true, exchangeRates: false });
    expect(await screen.findByText(/DCRDEX picks this up the next time it is unlocked/)).toBeTruthy();
  });
});

// switchFor is the on/off button of a privacy switch row.
const switchFor = async (label: string) => {
  const row = (await screen.findByText(label)).closest('div')!.parentElement!;
  const button = row.querySelector('button')!;
  await waitFor(() => expect(button.textContent).toBe('On'));
  return button;
};

describe('Saving without the Decred Pulse bot', () => {
  it('saves a switch without the bot URL', async () => {
    render(<PrivacySection />);
    fireEvent.click(await switchFor('Politeia'));
    await waitFor(() => expect(saveSettings).toHaveBeenCalled());
    expect(vi.mocked(saveSettings).mock.calls[0][0]).toEqual({
      global: { externalRequests: { vspListing: true, politeia: false, brseeder: true, exchangeRates: true } },
    });
  });

  it('puts a switch back when its save fails', async () => {
    vi.mocked(saveSettings).mockRejectedValueOnce(new Error('settings could not be written'));
    render(<PrivacySection />);
    const button = await switchFor('Politeia');
    fireEvent.click(button);
    expect(await screen.findByText('settings could not be written')).toBeTruthy();
    expect(button.textContent).toBe('On');
  });

  it('saves the bot URL on its own', async () => {
    render(<PrivacySection />);
    fireEvent.change(screen.getByPlaceholderText('https://brulse.decredcommunity.org'), {
      target: { value: 'https://bot.example' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalled());
    expect(vi.mocked(saveSettings).mock.calls[0][0]).toEqual({ global: { decredPulseBotUrl: 'https://bot.example' } });
  });
});
