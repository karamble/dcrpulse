// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, CheckCircle2, ExternalLink, FileUp, Loader2, Search } from 'lucide-react';
import { hashFile } from '../../utils/hashFile';
import { verifyTimestamp, type VerifyResponse } from '../../services/timestampApi';
import { StageList, type Stage, type StageState } from './StageList';
import { fromUnix, shortHash } from './util';
import { toYMDTime } from '../../utils/date';

const HEX64 = /^[0-9a-f]{64}$/;

function buildStages(r: VerifyResponse): Stage[] {
  const stages: Stage[] = [];

  stages.push({
    key: 'archive',
    label: r.inArchive ? 'Found in your local archive' : 'Not stored in your local archive',
    state: r.inArchive ? 'done' : 'skip',
    detail: r.inArchive ? undefined : 'Verification below relies on dcrtime and the chain, not local state.',
  });

  if (r.dcrtimeError) {
    stages.push({ key: 'dcrtime', label: 'Ask dcrtime about this digest', state: 'fail', detail: r.dcrtimeError });
    return stages;
  }
  const found = !!r.dcrtime?.found;
  stages.push({
    key: 'dcrtime',
    label: found ? 'dcrtime recognizes this digest' : 'dcrtime has no record of this digest',
    state: found ? 'done' : 'fail',
  });
  if (!found) return stages;

  const v = r.validation;
  if (!v || !v.hasProof) {
    stages.push({
      key: 'anchor',
      label: 'Waiting to be anchored',
      state: 'active',
      detail:
        r.dcrtime?.state === 'pending'
          ? 'In an anchor transaction; awaiting confirmations.'
          : 'Queued for the next hourly anchor.',
    });
    return stages;
  }

  const mark = (b: boolean): StageState => (b ? 'done' : 'fail');
  stages.push({ key: 'mpath', label: 'Merkle proof path is valid', state: mark(v.merklePathValid) });
  stages.push({ key: 'leaf', label: 'Your file is included in the timestamp', state: mark(v.digestInTree) });
  stages.push({ key: 'root', label: 'Merkle root matches the proof', state: mark(v.rootMatches) });

  const anchorDate = fromUnix(v.blockTime);
  stages.push({
    key: 'chain',
    label: 'Committed on the Decred blockchain',
    state: mark(v.anchoredOnChain),
    detail: v.anchoredOnChain
      ? `Block ${v.blockHeight ?? '?'}${anchorDate ? ` · ${toYMDTime(anchorDate)}` : ''}${
          v.confirmations ? ` · ${v.confirmations} confirmations` : ''
        }`
      : v.note,
  });
  return stages;
}

export const VerifyView = () => {
  const [digest, setDigest] = useState('');
  const [progress, setProgress] = useState(0);
  const [hashing, setHashing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VerifyResponse | null>(null);
  const [fileName, setFileName] = useState('');
  const inputRef = useRef<HTMLInputElement | null>(null);

  const runVerify = async (d: string) => {
    const clean = d.trim().toLowerCase();
    if (!HEX64.test(clean)) {
      setError('Enter a 64-character hex SHA-256 digest, or drop a file to hash.');
      return;
    }
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      setResult(await verifyTimestamp(clean));
    } catch (e: any) {
      setError((typeof e?.response?.data === 'string' && e.response.data) || e?.message || 'verification failed');
    } finally {
      setBusy(false);
    }
  };

  const onFile = async (f: File) => {
    setError(null);
    setResult(null);
    setFileName(f.name);
    setHashing(true);
    setProgress(0);
    try {
      const d = await hashFile(f, setProgress);
      setDigest(d);
      await runVerify(d);
    } catch (e: any) {
      setError(e?.message || 'hashing failed');
    } finally {
      setHashing(false);
    }
  };

  const onPick = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (f) void onFile(f);
  };

  const verified = !!result?.validation?.anchoredOnChain && !!result?.validation?.rootMatches && !!result?.validation?.digestInTree;

  return (
    <div className="space-y-6">
      <input ref={inputRef} type="file" onChange={onPick} className="hidden" />

      <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-3">
        <p className="text-sm text-muted-foreground">
          Drop a file (hashed locally) or paste a digest to check it against dcrtime and the Decred chain — verified
          entirely through this dashboard's own node.
        </p>
        <div className="flex flex-col sm:flex-row gap-2">
          <input
            value={digest}
            onChange={(e) => setDigest(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && runVerify(digest)}
            placeholder="Paste a 64-character SHA-256 digest"
            className="flex-1 px-3 py-2 rounded-lg bg-background border border-border/60 text-sm font-mono focus:outline-none focus:border-primary/60"
          />
          <button
            onClick={() => runVerify(digest)}
            disabled={busy || hashing}
            className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm hover:opacity-90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
            Verify
          </button>
          <button
            onClick={() => inputRef.current?.click()}
            disabled={hashing || busy}
            className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg border border-border/60 text-sm hover:bg-muted/20 disabled:opacity-50"
          >
            <FileUp className="h-4 w-4" />
            Hash a file
          </button>
        </div>
        {hashing && (
          <div>
            <div className="h-2 rounded-full bg-muted/30 overflow-hidden">
              <div className="h-full bg-gradient-primary transition-all" style={{ width: `${Math.round(progress * 100)}%` }} />
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              Hashing {fileName}… {Math.round(progress * 100)}%
            </div>
          </div>
        )}
        {digest && !hashing && <div className="text-xs font-mono text-muted-foreground break-all">{shortHash(digest, 16, 12)}</div>}
      </div>

      {error && (
        <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{error}</span>
        </div>
      )}

      {result && (
        <div className="space-y-4">
          {verified && (
            <div className="p-4 rounded-xl bg-success/10 border border-success/30 flex items-start gap-3">
              <CheckCircle2 className="h-5 w-5 text-success mt-0.5 shrink-0" />
              <div>
                <div className="font-semibold text-success">Verified</div>
                <div className="text-sm text-muted-foreground">
                  This exact file was timestamped on the Decred chain
                  {result.validation?.blockTime ? ` on ${toYMDTime(fromUnix(result.validation.blockTime)!)}` : ''}.
                </div>
              </div>
            </div>
          )}
          <div className="p-4 rounded-xl bg-gradient-card border border-border/50">
            <h3 className="text-sm font-semibold mb-3">Checks</h3>
            <StageList stages={buildStages(result)} />
            {result.validation?.anchoredOnChain && result.validation.txId && (
              <Link
                to={`/explorer/tx/${result.validation.txId}`}
                className="mt-3 inline-flex items-center gap-1.5 text-sm text-primary hover:underline"
              >
                <ExternalLink className="h-3.5 w-3.5" />
                View anchor transaction
              </Link>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
