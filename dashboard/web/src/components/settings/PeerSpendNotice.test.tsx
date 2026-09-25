import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { MCPGrant, MCPWriteScope } from '../../services/api';
import { PeerSpendNotice, peerCanSteerSpend } from './PeerSpendNotice';

const scopes: MCPWriteScope[] = [
  { key: 'lightning', label: 'Lightning', domain: 'lightning', needsPass: false, fund: true, risk: false },
  { key: 'bisonrelay', label: 'Bison Relay', domain: 'bisonrelay', needsPass: false, fund: false, risk: false },
];
const grant = (over: Partial<MCPGrant> = {}): MCPGrant => ({
  accounts: [], perTxDcr: 1, dailyDcr: 1, spentTodayDcr: 0, remainingTodayDcr: 1,
  allowlist: [], writeScopes: [], ...over,
});

afterEach(cleanup);

describe('peerCanSteerSpend', () => {
  it('warns when peers can reach an agent that can spend without approval', () => {
    expect(peerCanSteerSpend(['bisonrelay'], grant({ writeScopes: ['lightning'] }), scopes, false)).toBe(true);
    expect(peerCanSteerSpend(['bisonrelay'], grant({ accounts: [0] }), scopes, false)).toBe(true);
  });

  it('stays quiet when any condition is missing', () => {
    expect(peerCanSteerSpend(['bisonrelay'], grant({ writeScopes: ['lightning'] }), scopes, true)).toBe(false);
    expect(peerCanSteerSpend(['wallet'], grant({ writeScopes: ['lightning'] }), scopes, false)).toBe(false);
    expect(peerCanSteerSpend(['bisonrelay'], grant({ writeScopes: ['bisonrelay'] }), scopes, false)).toBe(false);
    expect(peerCanSteerSpend(['bisonrelay'], undefined, scopes, false)).toBe(false);
  });

  it('renders the note only when it applies', () => {
    render(<PeerSpendNotice domains={['bisonrelay']} grant={grant({ writeScopes: ['lightning'] })} scopes={scopes} oversightOn={false} />);
    expect(screen.getByRole('note').textContent).toMatch(/oversight/i);
    cleanup();
    render(<PeerSpendNotice domains={['bisonrelay']} grant={grant({ writeScopes: ['lightning'] })} scopes={scopes} oversightOn />);
    expect(screen.queryByRole('note')).toBeNull();
  });
});
