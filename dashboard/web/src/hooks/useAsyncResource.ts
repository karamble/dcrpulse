// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiError } from '../utils/apiError';

// Async is what a panel knows about one thing it fetched.
//
// Four states and no fifth. The shape is the point: loading carries no error
// and error carries no data, so a panel cannot hold a message it never renders,
// and cannot render a spinner while holding one.
//
// "Empty" is deliberately not a state. It is ready with data that happens to be
// empty, because emptiness is a property of the answer and unreachability is a
// property of the asking. Keeping them in different places is what stops a
// failed request being described to a person as "there is nothing".
export type Async<T> =
  | { status: 'loading' }
  | { status: 'ready'; data: T; at: number }
  // stale is an answer we still hold and an ask that has since failed. The data
  // is the best available truth, so it stays on screen and the error goes
  // beside it rather than replacing it.
  | { status: 'stale'; data: T; at: number; error: string }
  | { status: 'error'; error: string };

// dataOf returns what we hold, or undefined when we have never had an answer.
export const dataOf = <T>(s: Async<T>): T | undefined =>
  s.status === 'ready' || s.status === 'stale' ? s.data : undefined;

// errorOf returns the last failure, whether or not we still hold data.
export const errorOf = <T>(s: Async<T>): string | undefined =>
  s.status === 'error' || s.status === 'stale' ? s.error : undefined;

export interface AsyncResource<T> {
  state: Async<T>;
  // refreshing is deliberately outside the union. A refresh does not change
  // what is true on screen, only whether a spinner sits by the reload control,
  // and folding it in would let a background poll blank a good panel.
  refreshing: boolean;
  refresh: () => Promise<void>;
}

// useAsyncResource fetches one thing and keeps the four states above.
//
// The rules callers used to get wrong are in here instead: a failure that has
// prior data keeps the data, a failure without prior data is an error a person
// can retry, and an answer that arrives after its question was replaced is
// discarded rather than shown against the wrong subject.
export function useAsyncResource<T>(
  load: () => Promise<T>,
  fallback: string,
  deps: unknown[] = [],
): AsyncResource<T> {
  const [state, setState] = useState<Async<T>>({ status: 'loading' });
  const [refreshing, setRefreshing] = useState(false);

  const loadRef = useRef(load);
  loadRef.current = load;

  // gen rises whenever the question changes or another ask starts, so a slow
  // answer for one game cannot land in another game's panel.
  const gen = useRef(0);
  const held = useRef<{ data: T; at: number } | null>(null);

  const run = useCallback(async () => {
    const mine = ++gen.current;
    setRefreshing(true);
    try {
      const data = await loadRef.current();
      if (mine !== gen.current) return;
      const at = Date.now();
      held.current = { data, at };
      setState({ status: 'ready', data, at });
    } catch (err) {
      if (mine !== gen.current) return;
      const text = apiError(err, fallback);
      const have = held.current;
      setState(
        have
          ? { status: 'stale', data: have.data, at: have.at, error: text }
          : { status: 'error', error: text },
      );
    } finally {
      if (mine === gen.current) setRefreshing(false);
    }
  }, [fallback]);

  useEffect(() => {
    // A changed question invalidates what we hold: showing the previous
    // subject's data as though it were this one's is the bug the generation
    // counter exists to prevent, and it applies to the held copy too.
    gen.current++;
    held.current = null;
    setState({ status: 'loading' });
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { state, refreshing, refresh: run };
}
