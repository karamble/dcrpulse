// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useRef, useState } from 'react';
import { apiError } from '../utils/apiError';

// Action is what happened to one thing somebody pressed.
//
// Keyed per target rather than held as one flag, because a page with a dozen
// buttons and a single `busy` disables all twelve whichever was pressed, and
// tells nobody which one is working.
export type Action<R = void> =
  | { phase: 'idle' }
  | { phase: 'running' }
  | { phase: 'done'; result: R }
  | { phase: 'failed'; error: string };

const idle: Action<never> = { phase: 'idle' };

export interface ActionMap<R = void> {
  get: (key: string) => Action<R>;
  isRunning: (key: string) => boolean;
  anyRunning: boolean;
  // runningKeys lets a caller exclude siblings that genuinely conflict without
  // reaching for a global flag.
  runningKeys: string[];
  run: (
    key: string,
    fn: () => Promise<R>,
    opts?: {
      fallback?: string;
      // onError may replace the recorded outcome, or return 'panel' to say the
      // failure was never about this target - a bridge refusing to carry the
      // request at all says nothing about the coin, and belongs to the section.
      onError?: (err: unknown, text: string) => Action<R> | 'panel';
      onPanelError?: (text: string) => void;
    },
  ) => Promise<void>;
  clear: (key: string) => void;
  clearAll: () => void;
}

export function useActionMap<R = void>(): ActionMap<R> {
  const [map, setMap] = useState<Record<string, Action<R>>>({});
  // gen per key, so a late answer from a superseded press cannot overwrite the
  // one the person is actually waiting on.
  const gen = useRef<Record<string, number>>({});

  const get = useCallback((key: string): Action<R> => map[key] ?? (idle as Action<R>), [map]);

  const clear = useCallback((key: string) => {
    setMap((s) => {
      if (!(key in s)) return s;
      const next = { ...s };
      delete next[key];
      return next;
    });
  }, []);

  const run = useCallback<ActionMap<R>['run']>(async (key, fn, opts) => {
    const mine = (gen.current[key] = (gen.current[key] ?? 0) + 1);
    setMap((s) => ({ ...s, [key]: { phase: 'running' } }));
    try {
      const result = await fn();
      if (mine !== gen.current[key]) return;
      setMap((s) => ({ ...s, [key]: { phase: 'done', result } }));
    } catch (err) {
      if (mine !== gen.current[key]) return;
      const text = apiError(err, opts?.fallback ?? 'That did not work');
      const decided = opts?.onError?.(err, text);
      if (decided === 'panel') {
        setMap((s) => {
          const next = { ...s };
          delete next[key];
          return next;
        });
        opts?.onPanelError?.(text);
        return;
      }
      setMap((s) => ({ ...s, [key]: decided ?? { phase: 'failed', error: text } }));
    }
  }, []);

  const runningKeys = Object.keys(map).filter((k) => map[k].phase === 'running');

  return {
    get,
    isRunning: (key) => (map[key]?.phase ?? 'idle') === 'running',
    anyRunning: runningKeys.length > 0,
    runningKeys,
    run,
    clear,
    clearAll: () => setMap({}),
  };
}
