// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Every password and seed field spreads these: no browser or extension may
// fill, save, spellcheck or correct a secret.
export const secretFieldProps = {
  autoComplete: 'off',
  spellCheck: false,
  autoCorrect: 'off',
  autoCapitalize: 'off',
  'data-lpignore': 'true',
  'data-1p-ignore': 'true',
  'data-bwignore': 'true',
  'data-form-type': 'other',
} as const;
