// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, ArrowLeft, Download, Loader2, X } from 'lucide-react';
import {
  BisonrelaySharedFile,
  getBisonrelayRates,
  getBisonrelaySharedFiles,
  shareBisonrelayFile,
} from '../../services/bisonrelayApi';
import { MAX_OFFER_BYTES, StagedOffer, canOfferTo } from './chatOffer';
import { formatAtomsTrimmed, parseDcrAmount, toDcr } from '../../utils/amounts';
import { formatBytes } from '../../utils/bytes';
import { apiError } from '../../utils/apiError';
import { toYMDTime } from '../../utils/date';

interface Props {
  // The file to offer, when the user came in through "Offer for download" or
  // converted a staged attachment. Absent means start on the shared-file list.
  file?: File | null;
  // Set when file is a compressed variant and the original is what gets sold.
  compressedFrom?: File | null;
  // Null in a group chat: there is no group share, so an offer must be global.
  targetUid: string | null;
  targetNick: string;
  onClose: () => void;
  onStaged: (offer: StagedOffer) => void;
}

type Step = 'new' | 'uploading' | 'list' | 'confirm';

const scopeLabel = (global: boolean): string => (global ? 'all subscribers' : 'one contact');

export const ChatFileOfferModal = ({
  file,
  compressedFrom,
  targetUid,
  targetNick,
  onClose,
  onStaged,
}: Props) => {
  const group = targetUid === null;
  const [step, setStep] = useState<Step>(file ? 'new' : 'list');
  const [pick, setPick] = useState<File | null>(file ?? null);
  const [costDcr, setCostDcr] = useState('0');
  const [global, setGlobal] = useState(group);
  const [pct, setPct] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [files, setFiles] = useState<BisonrelaySharedFile[] | null>(null);
  // Kept apart from err: a failed list must not shout at someone who came in
  // to offer a brand new file.
  const [listErr, setListErr] = useState<string | null>(null);
  const [picked, setPicked] = useState<BisonrelaySharedFile | null>(null);
  const [rate, setRate] = useState<{ usd: number; source: string; at: string } | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const busy = step === 'uploading';

  useEffect(() => {
    if (busy) return undefined;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [busy, onClose]);

  useEffect(() => {
    let cancelled = false;
    getBisonrelaySharedFiles()
      .then((list) => {
        if (cancelled) return;
        list.sort((a, b) => a.filename.localeCompare(b.filename));
        setFiles(list);
      })
      .catch((e: any) => {
        if (!cancelled) setListErr(apiError(e, 'Could not load shared files'));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    getBisonrelayRates()
      .then((r) => {
        if (!cancelled && r.dcr_usd > 0) {
          setRate({ usd: r.dcr_usd, source: r.source, at: r.updated_at });
        }
      })
      .catch(() => {
        /* USD is best-effort; the DCR price always shows. */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const { atoms: costAtoms, error: costErr } = parseDcrAmount(costDcr, {
    optional: true,
    allowZero: true,
  });
  const dcr = toDcr(costAtoms);
  // The offer sells what the peer will receive, so a compressed preview copy is
  // never the thing shared.
  const source = compressedFrom ?? pick;
  const usdTitle = rate
    ? `USD via ${rate.source || 'unknown'}${rate.at ? `, updated ${toYMDTime(new Date(rate.at))}` : ''}`
    : undefined;
  const usdSuffix =
    rate && dcr > 0
      ? `≈ $${dcr * rate.usd < 0.01 ? (dcr * rate.usd).toFixed(4) : (dcr * rate.usd).toFixed(2)}`
      : '';

  const consequence = (() => {
    const who = group ? 'Members' : targetNick;
    if (dcr <= 0) {
      if (group) return `${who} can fetch it at any time at no cost.`;
      return global
        ? `${who} can fetch it at any time. So can anyone subscribed to you.`
        : `${who} can fetch it at any time at no cost.`;
    }
    const price = `${costDcr.trim()} DCR`;
    if (group) return `Each download costs ${price}, paid over Lightning.`;
    return global
      ? `Each download costs ${price}, paid over Lightning. Anyone subscribed to you can download it.`
      : `${who} pays ${price} over Lightning when the download starts.`;
  })();

  const pickFromDisk = (f: File | null) => {
    if (!f) return;
    if (f.size > MAX_OFFER_BYTES) {
      setErr(`File is ${formatBytes(f.size)}. A download offer can be at most ${formatBytes(MAX_OFFER_BYTES)}.`);
      return;
    }
    setErr(null);
    setPick(f);
    setStep('new');
  };

  const share = async () => {
    if (!source || costErr) return;
    // BR addresses a share by content, so identical bytes come back as a fid we
    // may already hold. Without the list we cannot tell, and claiming the share
    // would let a later remove withdraw one we never made.
    const known = files === null ? null : new Set(files.map((f) => f.fid));
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setStep('uploading');
    setPct(0);
    setErr(null);
    setNote(null);
    try {
      const res = await shareBisonrelayFile(
        source,
        costAtoms,
        global ? '' : (targetUid ?? ''),
        '',
        setPct,
        ctrl.signal,
      );
      onStaged({
        mode: 'offer',
        fid: res.fid,
        filename: res.filename || source.name,
        size: source.size,
        mime: source.type || '',
        cost: res.cost,
        scope: global ? 'global' : 'targeted',
        targetUid: global ? '' : (targetUid ?? ''),
        createdHere: known !== null && !known.has(res.fid),
      });
      onClose();
    } catch (e: any) {
      setStep('new');
      if (ctrl.signal.aborted) {
        setNote('Upload cancelled. Nothing was shared.');
      } else {
        setErr(apiError(e, 'Could not share the file'));
      }
    } finally {
      abortRef.current = null;
    }
  };

  const attachPicked = () => {
    if (!picked) return;
    onStaged({
      mode: 'offer',
      fid: picked.fid,
      filename: picked.filename,
      size: picked.size,
      mime: '',
      cost: picked.cost,
      scope: picked.global ? 'global' : 'targeted',
      targetUid: picked.global ? '' : (targetUid ?? ''),
      createdHere: false,
    });
    onClose();
  };

  const choiceRow = (isGlobal: boolean, label: string, caption: string) => (
    <label
      className={`flex items-start gap-2.5 rounded-lg border px-3 py-2 cursor-pointer transition-colors ${
        global === isGlobal
          ? 'border-primary/50 bg-primary/10'
          : 'border-border/50 bg-background/40 hover:border-primary/30'
      }`}
    >
      <input
        type="radio"
        name="br-offer-scope"
        checked={global === isGlobal}
        onChange={() => setGlobal(isGlobal)}
        className="mt-0.5 accent-[hsl(var(--primary))]"
      />
      <span className="min-w-0 flex-1">
        <span className="block text-sm text-foreground">{label}</span>
        <span className="block text-[10px] text-muted-foreground mt-0.5">{caption}</span>
      </span>
    </label>
  );

  const title = step === 'list' ? 'Your shared files' : step === 'confirm' ? 'Attach a shared file' : 'Offer for download';
  const subtitle = (() => {
    if (step === 'list') {
      return 'Attach a file you already share. The bytes stay on your machine until someone downloads it.';
    }
    if (step === 'confirm') {
      return `The price was set when the file was shared, so the chat advertises exactly what ${
        group ? 'members' : targetNick
      } will be charged.`;
    }
    return group
      ? 'Members get a download chip in the chat and fetch the file when they choose.'
      : `${targetNick} gets a download chip in the chat and fetches the file when they choose.`;
  })();

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md max-h-[85vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl"
      >
        <div className="flex items-start justify-between p-5 pb-3 gap-3">
          <div className="min-w-0">
            <h3 className="flex items-center gap-2 text-base font-semibold text-foreground">
              <Download className="h-4 w-4 text-primary" />
              {title}
            </h3>
            <p className="text-xs text-muted-foreground mt-1">{subtitle}</p>
          </div>
          {!busy && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>

        {(err || (listErr && step === 'list')) && (
          <div className="px-5 pb-2 flex items-start gap-2 text-xs text-destructive">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">{err || listErr}</span>
          </div>
        )}
        {note && !err && (
          <p className="px-5 pb-2 text-[11px] text-muted-foreground">{note}</p>
        )}

        {(step === 'new' || step === 'uploading') && source && (
          <>
            <div className="px-5">
              <div className="rounded-md bg-muted/20 border border-border/30 p-3">
                <p className="text-sm text-foreground truncate">{source.name}</p>
                {step === 'uploading' ? (
                  <>
                    <p className="text-[11px] text-muted-foreground mt-0.5">
                      {pct >= 100 ? 'Registering the share…' : `Uploading ${pct}%`}
                    </p>
                    <div className="mt-1 h-1 w-full overflow-hidden rounded bg-muted/30">
                      <div
                        className="h-full bg-primary transition-[width] duration-150"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </>
                ) : (
                  <p className="text-[11px] text-muted-foreground mt-0.5">
                    {formatBytes(source.size)}
                    {source.type ? ` · ${source.type}` : ''}
                  </p>
                )}
              </div>
              {step === 'new' && compressedFrom && (
                <p className="text-[10px] text-muted-foreground mt-2">
                  Offering the original file, {formatBytes(compressedFrom.size)}. The compressed copy
                  was only for sending inside the message.
                </p>
              )}
            </div>

            {step === 'new' && (
              <>
                <div className="px-5 pt-4">
                  <label htmlFor="br-offer-cost" className="block text-xs text-muted-foreground mb-1">
                    Price (DCR)
                  </label>
                  <div className="flex items-center gap-2">
                    <input
                      id="br-offer-cost"
                      type="number"
                      min={0}
                      step="0.00000001"
                      inputMode="decimal"
                      value={costDcr}
                      onChange={(e) => setCostDcr(e.target.value)}
                      className="flex-1 min-w-0 px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
                    />
                    {usdSuffix && (
                      <span className="shrink-0 text-xs text-muted-foreground tabular-nums" title={usdTitle}>
                        {usdSuffix}
                      </span>
                    )}
                  </div>
                  {costErr ? (
                    <p className="text-[11px] text-destructive mt-1">{costErr}</p>
                  ) : (
                    <p className="text-[10px] text-muted-foreground mt-1">
                      Leave it at 0 to offer the file for free.
                    </p>
                  )}
                </div>

                <div className="px-5 pt-4">
                  <p className="block text-xs text-muted-foreground mb-1">Who can download it</p>
                  {group ? (
                    <p className="text-[11px] text-muted-foreground">
                      All subscribers. Bison Relay has no group-only share.
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {choiceRow(false, `Only ${targetNick}`, 'Nobody else can request this file.')}
                      {choiceRow(true, 'All subscribers', 'Anyone subscribed to you can request it too.')}
                    </div>
                  )}
                </div>

                <div className="px-5 pt-4 space-y-1">
                  <p className="text-[11px] text-muted-foreground">{consequence}</p>
                  <p className="text-[10px] text-muted-foreground">
                    This registers the file under Files. Remove it there to stop offering it.
                  </p>
                </div>
              </>
            )}

            <div className="flex justify-end gap-2 p-5 pt-4">
              {step === 'uploading' ? (
                <button
                  type="button"
                  onClick={() => abortRef.current?.abort()}
                  disabled={pct >= 100}
                  className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm text-foreground transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Cancel upload
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    onClick={onClose}
                    className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm text-foreground transition-colors"
                  >
                    Cancel
                  </button>
                  <button
                    type="button"
                    onClick={share}
                    disabled={!!costErr}
                    className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Share and attach
                  </button>
                </>
              )}
            </div>
          </>
        )}

        {step === 'list' && (
          <>
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0] ?? null;
                e.target.value = '';
                pickFromDisk(f);
              }}
            />
            <div className="px-2 pb-2 min-h-[160px]">
              {files === null && !listErr ? (
                <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
                  <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                  <span>Loading shared files…</span>
                </div>
              ) : files && files.length === 0 ? (
                <div className="px-3 py-4 text-center space-y-3">
                  <p className="text-xs text-muted-foreground">You have not shared any files yet.</p>
                  <button
                    type="button"
                    onClick={() => fileInputRef.current?.click()}
                    className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition"
                  >
                    Share a file now
                  </button>
                </div>
              ) : (
                files?.map((f) => {
                  const usable = canOfferTo(f, targetUid);
                  return (
                    <button
                      key={f.fid}
                      type="button"
                      disabled={!usable}
                      onClick={() => {
                        setPicked(f);
                        setStep('confirm');
                      }}
                      className={`w-full text-left px-3 py-2 rounded-md text-sm flex flex-col gap-0.5 transition-colors ${
                        usable ? 'hover:bg-muted/30' : 'opacity-50 cursor-not-allowed'
                      }`}
                    >
                      <span className="truncate font-medium text-foreground">
                        {f.filename || '(unnamed file)'}
                      </span>
                      <span className="text-[10px] text-muted-foreground flex items-center gap-2">
                        <span>{formatBytes(f.size)}</span>
                        <span className="opacity-50">·</span>
                        <span>{scopeLabel(f.global)}</span>
                        {f.cost > 0 && (
                          <>
                            <span className="opacity-50">·</span>
                            <span className="text-primary/80">{formatAtomsTrimmed(f.cost)} DCR</span>
                          </>
                        )}
                      </span>
                      {!usable && (
                        <span className="text-[10px] text-muted-foreground">
                          {group
                            ? 'Shared with one contact. A group can only use a global share.'
                            : `Shared with someone else. ${targetNick} cannot download it.`}
                        </span>
                      )}
                    </button>
                  );
                })
              )}
            </div>
          </>
        )}

        {step === 'confirm' && picked && (
          <div className="px-5 pb-5 pt-1 space-y-4">
            <div className="rounded-md bg-muted/20 border border-border/30 p-3 text-xs text-muted-foreground space-y-1">
              <div className="text-foreground font-medium truncate">{picked.filename}</div>
              <div className="font-mono text-[10px] break-all">{picked.fid}</div>
              <div>
                {formatBytes(picked.size)} · {scopeLabel(picked.global)}
              </div>
              <div className="pt-1">
                Price:{' '}
                <span className="text-foreground font-medium">
                  {picked.cost > 0 ? `${formatAtomsTrimmed(picked.cost)} DCR` : 'Free download'}
                </span>
              </div>
            </div>
            {picked.cost > 0 && (
              <p className="text-[10px] text-muted-foreground">
                {group ? 'Members pay' : `${targetNick} pays`} over Lightning before BR releases the
                file.
              </p>
            )}
            <p className="text-[10px] text-muted-foreground">
              To change the price, unshare the file under Files and share it again.
            </p>
            <div className="flex justify-between gap-2 pt-1">
              <button
                type="button"
                onClick={() => {
                  setPicked(null);
                  setStep('list');
                }}
                className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors inline-flex items-center gap-1.5"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Pick a different file
              </button>
              <button
                type="button"
                onClick={attachPicked}
                className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition"
              >
                Attach
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
