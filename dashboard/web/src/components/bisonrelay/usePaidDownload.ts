// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import {
  BisonrelayLiveEvent,
  getBisonrelayManageDownloads,
  startBisonrelayContentGet,
} from '../../services/bisonrelayApi';
import {
  describeLnPaymentFailure,
  failedLnPaymentHashes,
  findNewFailedLnPayment,
} from '../../services/lightningApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { apiError } from '../../utils/apiError';

export type PaidDownloadPhase = 'idle' | 'confirm' | 'downloading' | 'mismatch' | 'done' | 'error';

// usePaidDownload drives one shared-file download from a contact. A paid file is
// confirmed first; the daemon pays at most the amount approved and, when the
// host's real share cost is higher, rejects with file-download-cost-rejected,
// which becomes a second confirm at the real price. saved starts a file already
// on disk as done; lookupSaved checks the downloads list for it once on mount.
// Fetching a file again pays for it again, so a saved file is never offered
// as a plain download.
export const usePaidDownload = ({
  uid,
  fid,
  saved = false,
  lookupSaved = false,
}: {
  uid: string;
  fid: string;
  saved?: boolean;
  lookupSaved?: boolean;
}) => {
  const [phase, setPhase] = useState<PaidDownloadPhase>(saved ? 'done' : 'idle');
  const [err, setErr] = useState<string | null>(null);
  const [realCost, setRealCost] = useState(0);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  // BR transfers are async and the seller may be offline or unable to issue
  // the invoice (no protocol error reaches us); hint after 30s of waiting.
  const [slow, setSlow] = useState(false);
  // Failed-payment hashes snapshotted when the download starts; a NEW failed
  // payment appearing during the download means our chunk payment could not
  // complete (e.g. no route) and the BR library parked the download silently.
  const failedBaseline = useRef<Set<string>>(new Set());
  const { addListener } = useBisonrelayLive();

  useEffect(() => {
    if (!lookupSaved || !fid) return undefined;
    let cancelled = false;
    getBisonrelayManageDownloads()
      .then((items) => {
        if (!cancelled && items.some((d) => d.uid === uid && d.fid === fid && d.missing_chunks === 0 && d.disk_path)) {
          setPhase((p) => (p === 'idle' ? 'done' : p));
        }
      })
      .catch(() => {
        /* Unknown is idle, as before the lookup. */
      });
    return () => {
      cancelled = true;
    };
  }, [lookupSaved, uid, fid]);

  useEffect(() => {
    if (phase !== 'downloading') {
      setSlow(false);
      return undefined;
    }
    const t = window.setTimeout(() => setSlow(true), 30000);
    return () => window.clearTimeout(t);
  }, [phase]);

  useEffect(() => {
    if (phase !== 'downloading') return undefined;
    return addListener((evt: BisonrelayLiveEvent) => {
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.uid !== uid || payload.fid !== fid) return;
      if (evt.type === 'file-download-cost-rejected') {
        setRealCost(typeof payload.cost_atoms === 'number' ? payload.cost_atoms : 0);
        setPhase('mismatch');
      } else if (evt.type === 'file-download-completed') {
        setPhase('done');
      }
    });
  }, [phase, addListener, uid, fid]);

  useEffect(() => {
    if (phase !== 'downloading') return undefined;
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const it = (await getBisonrelayManageDownloads()).find((d) => d.uid === uid && d.fid === fid);
        if (it && !cancelled) {
          setProgress({ done: Math.max(0, it.total_chunks - it.missing_chunks), total: it.total_chunks });
          if (it.missing_chunks === 0 && it.disk_path) {
            setPhase('done');
            return;
          }
        }
      } catch {
        // Transient list error; keep polling.
      }
      try {
        // A concurrent unrelated payment failing in this window would be
        // attributed to the download; rare and acceptable for surfacing.
        const failed = await findNewFailedLnPayment(failedBaseline.current);
        if (failed && !cancelled) {
          setErr(`Payment failed: ${describeLnPaymentFailure(failed.failureReason)}`);
          setPhase('error');
          return;
        }
      } catch {
        // Transient list error; keep polling.
      }
      if (!cancelled) timer = window.setTimeout(tick, 2000);
    };
    timer = window.setTimeout(tick, 800);
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [phase, uid, fid]);

  // start asks the daemon for the file, paying at most maxAtoms.
  const start = async (maxAtoms: number) => {
    setErr(null);
    try {
      failedBaseline.current = await failedLnPaymentHashes();
    } catch {
      failedBaseline.current = new Set();
    }
    setPhase('downloading');
    try {
      await startBisonrelayContentGet(uid, fid, maxAtoms);
    } catch (e: any) {
      setErr(apiError(e, 'Could not start download'));
      setPhase('error');
    }
  };

  // request confirms a paid file first and starts a free one at once.
  const request = (cost: number) => (cost > 0 ? setPhase('confirm') : start(0));

  return { phase, err, realCost, progress, slow, request, start, reset: () => setPhase('idle') };
};
