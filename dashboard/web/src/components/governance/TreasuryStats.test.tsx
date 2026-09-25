import { cloneElement, type ReactElement } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { getTreasuryRunway } from '../../services/treasuryApi';
import { RunwayCard, TreasuryStats, runwayText } from './TreasuryStats';

// jsdom lays nothing out, so the chart gets a fixed size.
vi.mock('recharts', async (original) => ({
  ...(await original<typeof import('recharts')>()),
  ResponsiveContainer: ({ children }: { children: ReactElement }) => cloneElement(children, { width: 800, height: 300 }),
}));
vi.mock('../../hooks/useVisiblePoll', () => ({ useVisiblePoll: () => {} }));
vi.mock('../../services/dcrdexApi', () => ({ getDexRates: async () => ({}) }));
vi.mock('../../services/treasuryApi', () => ({
  getTreasuryOutlook: vi.fn(async () => ({ fromHeight: 1, targetBlockSeconds: 300, months: [] })),
  getTreasuryRunway: vi.fn(async (spend: number) => ({
    fromHeight: 1, balanceAtoms: 1, monthlySpendAtoms: spend, firstMonthNetAtoms: 1,
    targetBlockSeconds: 300, projectionMonths: 1200, months: 324, exhaustedMonth: '2053-10', beyond: false,
  })),
}));
vi.mock('../../services/treasuryStorage', () => ({
  getAllTSpends: () => [{ txHash: 'a', amountAtoms: 600000000000, feeAtoms: 14020, payee: 'Ds', blockHeight: 1080576, timestamp: '2026-05-17T04:02:09Z', voteResult: 'approved' }],
  getAllTAdds: () => [{ txHash: 'b', amountAtoms: 100000000, blockHeight: 1080000, timestamp: '2026-05-10T00:00:00Z' }],
  getTBaseByMonth: () => ({ '2026-05': 470000000000 }),
  getTreasuryStats: () => ({ totalSpentAtoms: 600000014020, count: 1, lastSyncHeight: 1118350 }),
}));

afterEach(cleanup);

describe('runwayText', () => {
  it('writes whole months as years and months', () => {
    expect(runwayText(324)).toBe('27 years');
    expect(runwayText(325)).toBe('27 years 1 month');
    expect(runwayText(13)).toBe('1 year 1 month');
    expect(runwayText(5)).toBe('5 months');
    expect(runwayText(0)).toBe('0 months');
  });
});

describe('inflow vs outflow legend', () => {
  it('hides and shows a series when its legend entry is clicked', async () => {
    const { container } = render(<TreasuryStats balanceAtoms={88000000000000} series={[]} />);
    const bars = () => container.querySelectorAll('.recharts-bar').length;
    await waitFor(() => expect(bars()).toBe(3));

    fireEvent.click(screen.getByText('Spends'));
    await waitFor(() => expect(bars()).toBe(2));
    expect(screen.getByText('Spends').style.textDecoration).toBe('line-through');

    fireEvent.click(screen.getByText('Spends'));
    await waitFor(() => expect(bars()).toBe(3));
    expect(screen.getByText('Spends').style.textDecoration).toBe('');
  });
});

describe('RunwayCard', () => {
  const runway = vi.mocked(getTreasuryRunway);
  const measured = 367354236468;

  it('projects the measured average until the viewer types an amount', async () => {
    runway.mockClear();
    render(<RunwayCard measuredAtoms={measured} />);
    await waitFor(() => expect(runway).toHaveBeenLastCalledWith(measured));
    expect(await screen.findByText('27 years')).toBeTruthy();
    expect(screen.getByText(/last 12 months' average/)).toBeTruthy();

    const field = screen.getByLabelText('Monthly spend in DCR');
    fireEvent.change(field, { target: { value: '5000' } });
    await act(() => new Promise((r) => setTimeout(r, 200)));
    expect(runway).toHaveBeenCalledTimes(1); // waits for typing to pause
    await waitFor(() => expect(runway).toHaveBeenLastCalledWith(500000000000));
    expect(await screen.findByText(/your amount/)).toBeTruthy();

    fireEvent.change(field, { target: { value: 'abc' } });
    expect(screen.getByText('Use a positive number with up to 8 decimals')).toBeTruthy();
    await act(() => new Promise((r) => setTimeout(r, 500)));
    expect(runway).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByText('Use measured'));
    await waitFor(() => expect(runway).toHaveBeenCalledTimes(3));
    expect(runway).toHaveBeenLastCalledWith(measured);
  });
});
