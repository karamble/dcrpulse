// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, AlertTriangle, Check, Copy } from 'lucide-react';
import { exportDexSeed, markDexSeedBackedUp } from '../../services/dcrdexApi';

// DexSeedBackup is the guided app-seed backup flow: reveal the 15-word seed
// (re-entering the app password), have the user confirm they have written it
// down, then record the backup. Used inline in DEX Settings and inside the
// unlock backup-reminder modal. onDone fires after the backup is recorded.
export const DexSeedBackup = ({ onDone }: { onDone?: () => void }) => {
  const [appPass, setAppPass] = useState('');
  const [seed, setSeed] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  // Verification step: the user re-types a few random words (seed hidden) to
  // prove they recorded it, mirroring the Decred wallet setup wizard.
  const [verifying, setVerifying] = useState(false);
  const [indices, setIndices] = useState<number[]>([]);
  const [answers, setAnswers] = useState<Record<number, string>>({});

  // pickIndices selects `count` distinct word positions over `total`, sorted.
  const pickIndices = (count: number, total: number): number[] => {
    const picked: number[] = [];
    while (picked.length < count && picked.length < total) {
      const i = Math.floor(Math.random() * total);
      if (!picked.includes(i)) picked.push(i);
    }
    return picked.sort((a, b) => a - b);
  };

  const reveal = async () => {
    setBusy(true);
    setErr(null);
    try {
      const s = await exportDexSeed(appPass);
      setSeed(s);
      setIndices(pickIndices(3, s.split(' ').length));
      setAnswers({});
      setVerifying(false);
      setAppPass('');
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to export seed');
    } finally {
      setBusy(false);
    }
  };

  const copySeed = async () => {
    if (!seed) return;
    try {
      await navigator.clipboard.writeText(seed);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };

  // finish validates the typed words against the seed, then records the backup.
  const finish = async () => {
    if (!seed) return;
    const words = seed.split(' ');
    for (const idx of indices) {
      if ((answers[idx] || '').toLowerCase().trim() !== words[idx].toLowerCase()) {
        setErr(`Word #${idx + 1} does not match. Please check your backup.`);
        return;
      }
    }
    setBusy(true);
    setErr(null);
    try {
      await markDexSeedBackedUp();
      onDone?.();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to record backup');
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
        <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
        <div className="space-y-1.5">
          <p className="font-semibold">Back up your recovery seed now</p>
          <p>
            These 15 words are the <span className="font-semibold">only</span> way to recover your
            DCRDEX account, your fidelity-bond reclaim keys, and every native coin wallet you create
            here (BTC, LTC and others). Lose them and those funds are gone for good.
          </p>
          <p>Write them down and store them offline. Anyone who has them can take your funds.</p>
          <p>
            Your DCR is held by your Decred wallet, so it's recovered by your Decred wallet seed -
            not these 15 words. Keep both seeds safe.
          </p>
        </div>
      </div>

      {!seed ? (
        <>
          <p className="text-xs text-muted-foreground">
            Re-enter your DCRDEX app password to reveal the recovery seed.
          </p>
          <input
            type="password"
            value={appPass}
            onChange={(e) => setAppPass(e.target.value)}
            placeholder="App password"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
          />
          {err && (
            <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}
          <button
            type="button"
            disabled={busy || !appPass}
            onClick={reveal}
            className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {busy ? 'Revealing...' : 'Reveal seed'}
          </button>
        </>
      ) : !verifying ? (
        <>
          <div className="flex items-start gap-2">
            <code className="text-xs font-mono break-all flex-1 p-2 rounded bg-background border border-border">
              {seed}
            </code>
            <button
              type="button"
              onClick={copySeed}
              title="Copy"
              className="p-1.5 rounded-md hover:bg-background/60 shrink-0"
            >
              {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4 text-muted-foreground" />}
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              setErr(null);
              setVerifying(true);
            }}
            className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90"
          >
            I've written it down - verify
          </button>
        </>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            Confirm your backup by entering the following words from your seed.
          </p>
          <div className="space-y-2">
            {indices.map((idx) => (
              <div key={idx} className="space-y-1">
                <label className="text-xs font-medium">Word #{idx + 1}</label>
                <input
                  type="text"
                  autoComplete="off"
                  value={answers[idx] || ''}
                  onChange={(e) => {
                    setAnswers({ ...answers, [idx]: e.target.value });
                    if (err) setErr(null);
                  }}
                  placeholder={`Enter word #${idx + 1}`}
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
                />
              </div>
            ))}
          </div>
          {err && (
            <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                setErr(null);
                setVerifying(false);
              }}
              className="flex-1 px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors text-sm"
            >
              Back
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={finish}
              className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {busy ? 'Saving...' : 'Confirm backup'}
            </button>
          </div>
        </>
      )}
    </div>
  );
};
