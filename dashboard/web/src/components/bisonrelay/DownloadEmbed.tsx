// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import { isImageMime } from './embedParser';
import { bisonrelayContentFileUrl, getBisonrelayRates } from '../../services/bisonrelayApi';
import { ImageViewerModal } from './ImageViewerModal';
import { formatAtomsTrimmed, toDcr } from '../../utils/amounts';
import { formatBytes } from '../../utils/bytes';
import { usePaidDownload } from './usePaidDownload';

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
  // Costs are in atoms, distinct from the milli-atoms used for payment and
  // tip records.
  cost?: number;
}

// DownloadEmbed renders a file-transfer embed
// (--embed[download=<fid>,cost=,filename=,...]--): a file the host shares over
// BR file transfer, optionally behind a Lightning paywall. Clicking starts the
// download; for a paid file we require an explicit confirm first. The embed
// cost is only what the post advertises - the daemon pays at most the amount
// the user approved, and when the host's real share cost is higher it rejects
// with a file-download-cost-rejected event that we turn into a second confirm
// showing the actual price. A file already downloaded shows at once. On
// completion the bytes load from the dashboard's /content/file proxy - images
// render inline, other types as a download link.
export const DownloadEmbed = ({ seg, uid, self }: { seg: DownloadEmbedSeg; uid: string; self?: boolean }) => {
  const fid = seg.download || '';
  const filename = seg.filename || seg.name || 'file';
  const cost = seg.cost || 0;
  const isImage = isImageMime(seg.mime);
  const { phase, err, realCost, progress, slow, request, start, reset } = usePaidDownload({
    uid,
    fid,
    lookupSaved: !self,
  });
  const [usd, setUsd] = useState<{ amount: number; source: string; updatedAt: string } | null>(null);
  const [showViewer, setShowViewer] = useState(false);

  // For a paid file, look up the USD value of the cost (DCR/USD via BR, with a
  // Kraken fallback) so the price can be shown in both DCR and approximate USD.
  useEffect(() => {
    if (cost <= 0) return undefined;
    let cancelled = false;
    getBisonrelayRates()
      .then((r) => {
        if (!cancelled && r.dcr_usd > 0) {
          setUsd({ amount: toDcr(cost) * r.dcr_usd, source: r.source, updatedAt: r.updated_at });
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
  const costLabel = `${formatAtomsTrimmed(cost)} DCR${usdSuffix}`;
  const usdTitle = usd
    ? `USD via ${usd.source || 'unknown'}${usd.updatedAt ? `, updated ${toYMDTime(new Date(usd.updatedAt))}` : ''}`
    : undefined;

  if (!fid) return null;

  if (phase === 'done') {
    const url = bisonrelayContentFileUrl(fid, uid);
    if (isImage) {
      return (
        <>
          <button
            type="button"
            onClick={() => setShowViewer(true)}
            className="block p-0 border-0 bg-transparent cursor-zoom-in"
          >
            <img
              src={url}
              alt={seg.alt || filename}
              className="rounded-lg border border-border/40 max-w-full h-auto"
            />
          </button>
          {showViewer && (
            <ImageViewerModal
              image={{ src: url, name: filename, mime: seg.mime || '' }}
              onClose={() => setShowViewer(false)}
            />
          )}
        </>
      );
    }
    return (
      <a
        href={url}
        download={filename}
        className="inline-block max-w-full break-words text-xs text-primary underline hover:no-underline"
      >
        {filename} ({seg.mime || 'binary'})
      </a>
    );
  }

  const meta = [filename, seg.size ? formatBytes(seg.size) : '', cost > 0 ? costLabel : 'free']
    .filter(Boolean)
    .join(' · ');

  // Our own share: BR cannot fetch a file from ourselves, so render the
  // advertisement without a download action.
  if (self) {
    return (
      <div className="rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-1">
        <div className="text-xs text-muted-foreground break-words" title={usdTitle}>{meta}</div>
        <div className="text-xs text-muted-foreground">Your shared file</div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
      <div className="text-xs text-muted-foreground break-words" title={usdTitle}>{meta}</div>
      {phase === 'confirm' ? (
        <div className="space-y-2">
          <div className="break-words">
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
              onClick={reset}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'mismatch' ? (
        <div className="space-y-2">
          <div className="break-words">
            The sender charges{' '}
            <span className="font-semibold text-foreground">
              {formatAtomsTrimmed(realCost)} DCR
            </span>{' '}
            for <span className="font-mono">{filename}</span>
            {cost > 0
              ? `, the post advertised ${formatAtomsTrimmed(cost)} DCR.`
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
              onClick={reset}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'downloading' ? (
        <div className="space-y-1">
          <div className="text-muted-foreground">
            Downloading{progress && progress.total ? ` ${progress.done}/${progress.total} chunks` : '…'}
          </div>
          {slow && (
            <div className="text-xs text-muted-foreground/80">
              Still waiting for the seller. They may be offline or unable to
              issue the payment invoice right now.
            </div>
          )}
        </div>
      ) : phase === 'error' ? (
        <div className="space-y-2">
          <div className="text-xs text-rose-300 break-words">{err}</div>
          <button
            type="button"
            onClick={reset}
            className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
          >
            Try again
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => request(cost)}
          title={usdTitle}
          className="max-w-full truncate px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
        >
          {cost > 0 ? `Download (${costLabel})` : `Download ${filename}`}
        </button>
      )}
    </div>
  );
};
