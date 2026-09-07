// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { getDexPostBondStatus, postDexBond } from '../../services/dcrdexApi';
import { startVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';

export type DexBondPostPhase = 'idle' | 'submitting' | 'broadcast' | 'error';

// useDexBondPost owns the state of a bond post to one host. The backend answers
// 202 and posts in the background, so "in flight" outlives the request: this
// polls the status endpoint until the bond broadcasts or fails, resumes waiting
// after a reload (a post already in flight is adopted on mount), and treats the
// backend's "already being posted" refusal as that same in-flight state rather
// than as a failure.
export function useDexBondPost(host: string) {
  const [phase, setPhase] = useState<DexBondPostPhase>('idle');
  const [error, setError] = useState<string | null>(null);

  // Only an in-flight post is adopted on mount; an older broadcast or failure
  // belongs to whoever made it, not to this page.
  useEffect(() => {
    let cancelled = false;
    setPhase('idle');
    setError(null);
    getDexPostBondStatus(host)
      .then((s) => {
        if (!cancelled && s.phase === 'submitting') setPhase('submitting');
      })
      .catch(() => {
        /* nothing to resume */
      });
    return () => {
      cancelled = true;
    };
  }, [host]);

  useEffect(() => {
    if (phase !== 'submitting') return;
    let cancelled = false;
    const check = async () => {
      try {
        const s = await getDexPostBondStatus(host);
        if (cancelled) return;
        if (s.phase === 'broadcast') {
          setPhase('broadcast');
        } else if (s.phase === 'error') {
          setError(s.error || 'Bond posting failed');
          setPhase('error');
        } else if (s.phase === 'none') {
          // The dashboard restarted mid-flight; there is nothing left to follow.
          setPhase('idle');
        }
      } catch {
        /* keep polling */
      }
    };
    const stop = startVisiblePoll(check, 3000, false);
    return () => {
      cancelled = true;
      stop();
    };
  }, [phase, host]);

  const submit = useCallback(
    async (bond: number, assetID?: number) => {
      setError(null);
      setPhase('submitting');
      try {
        await postDexBond(host, bond, assetID);
      } catch (e: any) {
        // A 409 whose body says "submitting" means another post to this host
        // is already in flight: keep waiting on that one. Any other refusal
        // (the locked case is also a 409, with a plain-text body) is a failure.
        if (e?.response?.data?.phase === 'submitting') return;
        setError(apiError(e, 'Bond posting failed'));
        setPhase('error');
      }
    },
    [host],
  );

  return { phase, error, submit };
}
