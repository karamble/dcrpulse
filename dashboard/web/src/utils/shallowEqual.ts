// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// shallowEqual reports whether two flat objects carry the same own keys with
// Object.is-equal values. Pollers and streams use it to keep the previous
// state object when a tick carries no new data, so React skips the re-render.
export function shallowEqual<T extends object>(a: T, b: T): boolean {
  if (Object.is(a, b)) return true;
  const keys = Object.keys(a) as (keyof T)[];
  if (keys.length !== Object.keys(b).length) return false;
  for (const k of keys) {
    if (!Object.is(a[k], b[k])) return false;
  }
  return true;
}
