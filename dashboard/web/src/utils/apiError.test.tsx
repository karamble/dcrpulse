import { describe, expect, it } from 'vitest';
import { needsAppPassword } from './apiError';

describe('needsAppPassword', () => {
  it('matches only the app-password refusal', () => {
    expect(needsAppPassword({ response: { status: 401, headers: { 'x-dashboard-auth': 'password-required' } } })).toBe(true);
    expect(needsAppPassword({ response: { status: 401, headers: {} } })).toBe(false);
    expect(needsAppPassword({ response: { status: 403, headers: { 'x-dashboard-auth': 'password-required' } } })).toBe(false);
    expect(needsAppPassword(new Error('network'))).toBe(false);
    expect(needsAppPassword(undefined)).toBe(false);
  });
});
