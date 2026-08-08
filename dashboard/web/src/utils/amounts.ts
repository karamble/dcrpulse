// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const ATOMS_PER_DCR = 1e8;

// toDcr converts an atom count to DCR.
export const toDcr = (atoms: number): number => atoms / ATOMS_PER_DCR;

// formatDcr renders an atom count with a fixed number of decimals.
export const formatDcr = (atoms: number, decimals = 8): string =>
  toDcr(atoms).toFixed(decimals);

// formatDcrTrimmed renders an atom count without trailing zeros.
export const formatDcrTrimmed = (atoms: number): string =>
  formatDcr(atoms).replace(/\.?0+$/, '');

export interface AmountRules {
  // optional accepts an empty field; allowZero accepts a zero amount.
  optional?: boolean;
  allowZero?: boolean;
}

// validateDcrAmount checks the shape of a user-typed DCR amount, returning the
// message to show or null. Anything but digits and up to eight decimals is
// refused, so the exponent and hex forms Number() accepts cannot slip through.
// The supply ceiling is not checked here; the daemons own that.
export const validateDcrAmount = (
  raw: string,
  { optional = false, allowZero = false }: AmountRules = {},
): string | null => {
  const trimmed = raw.trim();
  if (!trimmed) return optional ? null : 'Amount required';
  if (!/^\d*\.?\d{0,8}$/.test(trimmed)) return 'Use a positive number with up to 8 decimals';
  const n = Number(trimmed);
  if (!Number.isFinite(n) || (allowZero ? n < 0 : n <= 0)) {
    return allowZero ? 'Amount must be zero or positive' : 'Amount must be positive';
  }
  return null;
};

// parseDcrAmount validates and converts in one step. atoms is 0 when invalid.
export const parseDcrAmount = (
  raw: string,
  rules: AmountRules = {},
): { atoms: number; error: string | null } => {
  const error = validateDcrAmount(raw, rules);
  if (error) return { atoms: 0, error };
  return { atoms: Math.round(Number(raw.trim()) * ATOMS_PER_DCR), error: null };
};
