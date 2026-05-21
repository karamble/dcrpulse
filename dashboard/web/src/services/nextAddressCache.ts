// In-memory cache of fresh receive addresses derived via getNextAddress.
// Persists across component remounts within a tab session so users navigating
// to/from the Privacy page don't burn a new BIP44 index every time. Lost on
// page reload - that's deliberate, a full reload is a strong signal to refresh.
//
// Callers must invalidate(account, branch) once the cached address has been
// used on-chain (e.g. immediately after a successful send), since dcrwallet
// will continue to derive forward and we don't want to reuse a used address.

const cache = new Map<string, string>();

const key = (account: number, branch: number = 0): string => `${account}:${branch}`;

export const nextAddressCache = {
  get(account: number, branch: number = 0): string | null {
    return cache.get(key(account, branch)) ?? null;
  },
  set(account: number, branch: number = 0, address: string): void {
    cache.set(key(account, branch), address);
  },
  invalidate(account: number, branch: number = 0): void {
    cache.delete(key(account, branch));
  },
};
