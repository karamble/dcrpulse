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
