// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import {
  BisonrelayLiveEvent,
  bisonrelayContentFileUrl,
  getBisonrelayManageDownloads,
  getBisonrelayRates,
  startBisonrelayContentGet,
} from '../../services/bisonrelayApi';
import {
  describeLnPaymentFailure,
  failedLnPaymentHashes,
  findNewFailedLnPayment,
} from '../../services/lightningApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

// DownloadEmbedSeg is the subset of a BR embed segment a file-transfer embed
// needs. Both BisonrelayPostBodySegment and BisonrelayPageSegment satisfy it,
// so posts and pages share one renderer.
export interface DownloadEmbedSeg {
  download?: string;
  filename?: string;
  name?: string;
  mime?: string;
  alt?: string;
  size?: number;
  cost?: number;
}

// formatDcrFromAtoms renders an atom amount as a trimmed DCR string. BR
// shared-file / embed costs are in atoms (1 DCR = 1e8), distinct from the
// milli-atoms used for payment and tip records.
const formatDcrFromAtoms = (atoms: number): string =>
  (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

const formatDownloadBytes = (n: number): string => {
  if (!n) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
};

// DownloadEmbed renders a file-transfer embed
// (--embed[download=<fid>,cost=,filename=,...]--): a file the host shares over
// BR file transfer, optionally behind a Lightning paywall. Clicking starts the
// download; for a paid file we require an explicit confirm first. The embed
// cost is only what the post advertises - the daemon pays at most the amount
// the user approved, and when the host's real share cost is higher it rejects
// with a file-download-cost-rejected event that we turn into a second confirm
// showing the actual price. While in flight we poll the downloads list for
// progress; on completion the bytes load from the dashboard's /content/file
// proxy - images render inline, other types as a download link.
export const DownloadEmbed = ({ seg, uid }: { seg: DownloadEmbedSeg; uid: string }) => {
  const fid = seg.download || '';
  const filename = seg.filename || seg.name || 'file';
  const cost = seg.cost || 0;
  const isImage = !!seg.mime && seg.mime.startsWith('image/');
  const [phase, setPhase] = useState<
    'idle' | 'confirm' | 'downloading' | 'mismatch' | 'ready' | 'error'
  >('idle');
  const [err, setErr] = useState<string | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [usd, setUsd] = useState<{ amount: number; source: string; updatedAt: string } | null>(null);
  const [realCost, setRealCost] = useState(0);
  // Failed-payment hashes snapshotted when the download starts; a NEW failed
  // payment appearing during the download means our chunk payment could not
  // complete (e.g. no route) and the BR library parked the download silently.
  const failedBaseline = useRef<Set<string>>(new Set());
  const { addListener } = useBisonrelayLive();

  // For a paid file, look up the USD value of the cost (DCR/USD via BR, with a
  // Kraken fallback) so the price can be shown in both DCR and approximate USD.
  useEffect(() => {
    if (cost <= 0) return undefined;
    let cancelled = false;
    getBisonrelayRates()
      .then((r) => {
        if (!cancelled && r.dcr_usd > 0) {
          setUsd({ amount: (cost / 1e8) * r.dcr_usd, source: r.source, updatedAt: r.updated_at });
        }
      })
      .catch(() => {
        /* USD is best-effort; the DCR price always shows. */
      });
    return () => {
      cancelled = true;
    };
  }, [cost]);

  // costLabel renders "<n> DCR" plus an approximate USD value when known.
  const usdSuffix = usd
    ? ` (~$${usd.amount < 0.01 ? usd.amount.toFixed(4) : usd.amount.toFixed(2)})`
    : '';
  const costLabel = `${formatDcrFromAtoms(cost)} DCR${usdSuffix}`;
  const usdTitle = usd
    ? `USD via ${usd.source || 'unknown'}${usd.updatedAt ? `, updated ${new Date(usd.updatedAt).toLocaleString()}` : ''}`
    : undefined;

  useEffect(() => {
    if (phase !== 'downloading') return undefined;
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const items = await getBisonrelayManageDownloads();
        const it = items.find((d) => d.fid === fid);
        if (it) {
          setProgress({ done: Math.max(0, it.total_chunks - it.missing_chunks), total: it.total_chunks });
          if (it.missing_chunks === 0 && it.disk_path) {
            if (!cancelled) setPhase('ready');
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
  }, [phase, fid]);

  // The daemon rejects the download when the host's stored share cost
  // exceeds what the user approved; switch to a second confirm showing
  // the real price.
  useEffect(() => {
    if (phase !== 'downloading') return undefined;
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'file-download-cost-rejected') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.uid !== uid || payload.fid !== fid) return;
      setRealCost(typeof payload.cost_atoms === 'number' ? payload.cost_atoms : 0);
      setPhase('mismatch');
    });
  }, [phase, addListener, uid, fid]);

  const start = async (maxCostAtoms: number) => {
    setErr(null);
    try {
      failedBaseline.current = await failedLnPaymentHashes();
    } catch {
      failedBaseline.current = new Set();
    }
    setPhase('downloading');
    try {
      await startBisonrelayContentGet(uid, fid, maxCostAtoms);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not start download');
      setPhase('error');
    }
  };

  if (!fid) return null;

  if (phase === 'ready') {
    const url = bisonrelayContentFileUrl(fid, uid);
    if (isImage) {
      return (
        <img
          src={url}
          alt={seg.alt || filename}
          className="rounded-lg border border-border/40 max-w-full h-auto"
        />
      );
    }
    return (
      <a
        href={url}
        download={filename}
        className="inline-block text-xs text-primary underline hover:no-underline"
      >
        {filename} ({seg.mime || 'binary'})
      </a>
    );
  }

  const meta = [filename, seg.size ? formatDownloadBytes(seg.size) : '', cost > 0 ? costLabel : 'free']
    .filter(Boolean)
    .join(' · ');

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
      <div className="text-xs text-muted-foreground" title={usdTitle}>{meta}</div>
      {phase === 'confirm' ? (
        <div className="space-y-2">
          <div>
            Pay <span className="font-semibold text-foreground" title={usdTitle}>{costLabel}</span> to
            download <span className="font-mono">{filename}</span>?
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => start(cost)}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
            >
              Pay &amp; download
            </button>
            <button
              type="button"
              onClick={() => setPhase('idle')}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'mismatch' ? (
        <div className="space-y-2">
          <div>
            The sender charges{' '}
            <span className="font-semibold text-foreground">
              {formatDcrFromAtoms(realCost)} DCR
            </span>{' '}
            for <span className="font-mono">{filename}</span>
            {cost > 0
              ? `, the post advertised ${formatDcrFromAtoms(cost)} DCR.`
              : ', the post advertised it as free.'}{' '}
            Pay the actual price?
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => start(realCost)}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
            >
              Pay &amp; download
            </button>
            <button
              type="button"
              onClick={() => setPhase('idle')}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'downloading' ? (
        <div className="text-muted-foreground">
          Downloading{progress && progress.total ? ` ${progress.done}/${progress.total} chunks` : '…'}
        </div>
      ) : phase === 'error' ? (
        <div className="space-y-2">
          <div className="text-xs text-rose-300">{err}</div>
          <button
            type="button"
            onClick={() => setPhase('idle')}
            className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
          >
            Try again
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => (cost > 0 ? setPhase('confirm') : start(0))}
          title={usdTitle}
          className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
        >
          {cost > 0 ? `Download (${costLabel})` : `Download ${filename}`}
        </button>
      )}
    </div>
  );
};
