// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useLatestRequest } from './useLatestRequest';

describe('useLatestRequest', () => {
  it('lets only the latest request through', () => {
    const { result } = renderHook(() => useLatestRequest());
    const first = result.current.start();
    const second = result.current.start();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  it('binds follow-up work to the latest request without superseding it', () => {
    const { result } = renderHook(() => useLatestRequest());
    const load = result.current.start();
    const submit = result.current.current();
    expect(load()).toBe(true);
    expect(submit()).toBe(true);
    result.current.start();
    expect(submit()).toBe(false);
  });

  it('drops everything on cancel and on unmount', () => {
    const { result, unmount } = renderHook(() => useLatestRequest());
    const a = result.current.start();
    result.current.cancel();
    expect(a()).toBe(false);
    const b = result.current.start();
    unmount();
    expect(b()).toBe(false);
  });

  it('keeps the same guard across renders', () => {
    const { result, rerender } = renderHook(() => useLatestRequest());
    const before = result.current;
    rerender();
    expect(result.current).toBe(before);
  });
});
