import { describe, expect, it } from 'vitest';
import { AccountInfo } from '../../services/api';
import { boundAccountState } from './BisonrelayGamingTab';

const account = (name: string, reserved = false): AccountInfo =>
  ({ accountName: name, accountNumber: name.length, reserved } as unknown as AccountInfo);

describe('boundAccountState', () => {
  const accounts = [account('default'), account('games'), account('unmixed', true), account('lightning', true)];

  it('allows an ordinary account', () => {
    expect(boundAccountState('games', accounts)).toBe('allowed');
  });

  it('refuses an account another part of the stack owns', () => {
    expect(boundAccountState('unmixed', accounts)).toBe('reserved');
    expect(boundAccountState('lightning', accounts)).toBe('reserved');
  });

  it('keeps a stored name the wallet has not listed', () => {
    expect(boundAccountState('games', [])).toBe('unknown');
  });

  it('reads no binding as none', () => {
    expect(boundAccountState('  ', accounts)).toBe('none');
  });
});
