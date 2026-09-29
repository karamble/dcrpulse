// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { PassphraseModal } from '../components/wallet/PassphraseModal';

afterEach(cleanup);

const sources = import.meta.glob(['../**/*.tsx', '!../**/*.test.tsx'], {
  query: '?raw', import: 'default', eager: true,
}) as Record<string, string>;

const ELEMENT = /<(input|textarea)\b(?:[^<>]|=>|\{[^{}]*\})*?\/>/gs;
const isPassword = (el: string) => /type=("password"|\{[^}]*'password'[^}]*\})/.test(el);

describe('secret fields', () => {
  it('every password field spreads secretFieldProps and sets no hint of its own', () => {
    const bad: string[] = [];
    let seen = 0;
    for (const [file, src] of Object.entries(sources)) {
      for (const [el] of src.matchAll(ELEMENT)) {
        if (!isPassword(el)) continue;
        seen++;
        if (!el.includes('{...secretFieldProps}') || /autoComplete=|spellCheck=|autoCorrect=|autoCapitalize=/.test(el)) {
          bad.push(file);
        }
      }
    }
    expect(seen).toBeGreaterThanOrEqual(36);
    expect(bad).toEqual([]);
  });

  it('no source asks the browser to store a password', () => {
    const hits = Object.entries(sources)
      .filter(([, src]) => /current-password|new-password/.test(src))
      .map(([file]) => file);
    expect(hits).toEqual([]);
  });

  it('renders a passphrase field no browser or extension may fill, save or spellcheck', () => {
    render(<PassphraseModal isOpen title="Unlock" submitLabel="Unlock" onSubmit={async () => {}} onClose={() => {}} />);
    const input = document.getElementById('passphrase-modal-input')!;
    expect(input.getAttribute('type')).toBe('password');
    expect(input.getAttribute('autocomplete')).toBe('off');
    expect(input.getAttribute('spellcheck')).toBe('false');
    expect(input.getAttribute('autocorrect')).toBe('off');
    expect(input.getAttribute('autocapitalize')).toBe('off');
    for (const attr of ['data-lpignore', 'data-1p-ignore', 'data-bwignore']) {
      expect(input.getAttribute(attr)).toBe('true');
    }
  });
});
