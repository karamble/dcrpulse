// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useRef, useState } from 'react';
import { AlertCircle, FileUp, RotateCcw, ShieldCheck, UploadCloud } from 'lucide-react';
import { CopyButton } from '../explorer/CopyButton';
import { hashFile } from '../../utils/hashFile';
import { createTimestamp, type TimestampRecord } from '../../services/timestampApi';
import { StatusBadge } from './StatusBadge';
import { StageList, type Stage, type StageState } from './StageList';
import { fmtBytes, shortHash } from './util';

type Phase = 'idle' | 'hashing' | 'ready' | 'submitting' | 'done' | 'error';

interface Props {
  onStamped?: () => void;
}

export const StampView = ({ onStamped }: Props) => {
  const [file, setFile] = useState<File | null>(null);
  const [digest, setDigest] = useState('');
  const [progress, setProgress] = useState(0);
  const [phase, setPhase] = useState<Phase>('idle');
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [record, setRecord] = useState<TimestampRecord | null>(null);
  const [duplicate, setDuplicate] = useState<TimestampRecord | null>(null);
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const reset = () => {
    setFile(null);
    setDigest('');
    setProgress(0);
    setPhase('idle');
    setError(null);
    setTitle('');
    setDescription('');
    setTagsInput('');
    setRecord(null);
    setDuplicate(null);
  };

  const onFile = useCallback(async (f: File) => {
    reset();
    setFile(f);
    setPhase('hashing');
    try {
      const d = await hashFile(f, setProgress);
      setDigest(d);
      setPhase('ready');
    } catch (err: any) {
      setError(err?.message || 'hashing failed');
      setPhase('error');
    }
  }, []);

  const onPick = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (f) void onFile(f);
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const f = e.dataTransfer.files?.[0];
    if (f) void onFile(f);
  };

  const submit = async () => {
    if (!file || !digest) return;
    setPhase('submitting');
    setError(null);
    setDuplicate(null);
    const tags = tagsInput
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);
    try {
      const rec = await createTimestamp({
        digest,
        filename: file.name,
        title: title.trim(),
        description: description.trim(),
        fileSize: file.size,
        mimeType: file.type,
        fileMtime: new Date(file.lastModified).toISOString(),
        tags,
      });
      setRecord(rec);
      setPhase('done');
      onStamped?.();
    } catch (err: any) {
      if (err?.response?.status === 409 && err.response.data?.record) {
        setDuplicate(err.response.data.record as TimestampRecord);
        setPhase('ready');
      } else {
        setError(
          (typeof err?.response?.data === 'string' && err.response.data) ||
            err?.message ||
            'submission failed',
        );
        setPhase('error');
      }
    }
  };

  const stages: Stage[] = [];
  if (file) {
    const hashState: StageState = phase === 'hashing' ? 'active' : digest ? 'done' : phase === 'error' ? 'fail' : 'pending';
    stages.push({
      key: 'hash',
      label: 'Hash the file in your browser',
      state: hashState,
      detail:
        phase === 'hashing'
          ? `${Math.round(progress * 100)}%`
          : digest
            ? shortHash(digest, 12, 8)
            : 'The file never leaves your device.',
    });
    stages.push({
      key: 'submit',
      label: 'Submit the digest to dcrtime',
      state: record ? 'done' : phase === 'submitting' ? 'active' : 'pending',
    });
    stages.push({
      key: 'anchor',
      label: 'Anchor to the Decred blockchain',
      state: record ? (record.status === 'anchored' ? 'done' : 'active') : 'pending',
      detail: record
        ? record.status === 'anchored'
          ? 'Committed on-chain.'
          : 'Anchored hourly; the Library updates automatically.'
        : undefined,
    });
  }

  return (
    <div className="space-y-6">
      <input ref={inputRef} type="file" onChange={onPick} className="hidden" />

      {!file ? (
        <div
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={`cursor-pointer rounded-xl border-2 border-dashed p-10 text-center transition-colors ${
            dragging ? 'border-primary bg-primary/5' : 'border-border/60 hover:border-primary/50 hover:bg-muted/10'
          }`}
        >
          <UploadCloud className="mx-auto h-10 w-10 text-muted-foreground" />
          <p className="mt-3 font-medium">Drop a file here, or click to choose</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Your file is hashed locally; only its 32-byte fingerprint is ever sent.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left: file + metadata */}
          <div className="space-y-4">
            <div className="p-4 rounded-xl bg-gradient-card border border-border/50">
              <div className="flex items-start gap-3">
                <FileUp className="h-5 w-5 text-primary mt-0.5 shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="font-medium truncate">{file.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {fmtBytes(file.size)}
                    {file.type ? ` · ${file.type}` : ''}
                  </div>
                </div>
                <button
                  onClick={() => inputRef.current?.click()}
                  className="text-xs text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                  Replace
                </button>
              </div>
              {phase === 'hashing' && (
                <div className="mt-3">
                  <div className="h-2 rounded-full bg-muted/30 overflow-hidden">
                    <div
                      className="h-full bg-gradient-primary transition-all"
                      style={{ width: `${Math.round(progress * 100)}%` }}
                    />
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">Hashing… {Math.round(progress * 100)}%</div>
                </div>
              )}
              {digest && (
                <div className="mt-3">
                  <div className="text-xs text-muted-foreground mb-1">SHA-256 digest</div>
                  <div className="flex items-center gap-2">
                    <code className="text-xs font-mono break-all flex-1">{digest}</code>
                    <CopyButton text={digest} />
                  </div>
                </div>
              )}
            </div>

            <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-3">
              <div>
                <label className="block text-xs text-muted-foreground mb-1">Title (optional)</label>
                <input
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="e.g. Signed supplier contract"
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
                />
              </div>
              <div>
                <label className="block text-xs text-muted-foreground mb-1">Description (optional)</label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  rows={2}
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60 resize-y"
                />
              </div>
              <div>
                <label className="block text-xs text-muted-foreground mb-1">Tags (comma-separated, optional)</label>
                <input
                  value={tagsInput}
                  onChange={(e) => setTagsInput(e.target.value)}
                  placeholder="contract, 2026"
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
                />
              </div>
              <button
                onClick={submit}
                disabled={!digest || phase === 'submitting' || phase === 'done'}
                className="w-full inline-flex items-center justify-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <ShieldCheck className="h-4 w-4" />
                {phase === 'submitting' ? 'Submitting…' : 'Timestamp this file'}
              </button>
            </div>
          </div>

          {/* Right: staged trace + outcome */}
          <div className="space-y-4">
            <div className="p-4 rounded-xl bg-gradient-card border border-border/50">
              <h3 className="text-sm font-semibold mb-3">Progress</h3>
              <StageList stages={stages} />
            </div>

            {error && (
              <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{error}</span>
              </div>
            )}

            {duplicate && (
              <div className="p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm">
                <div className="font-medium text-warning">This file is already in your archive.</div>
                <div className="mt-1 text-muted-foreground">
                  Stamped as <span className="text-foreground">{duplicate.title || duplicate.filename}</span> —{' '}
                  <StatusBadge status={duplicate.status} />
                </div>
              </div>
            )}

            {record && (
              <div className="p-4 rounded-xl bg-success/10 border border-success/30 space-y-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium text-success">Timestamp recorded</span>
                  <StatusBadge status={record.status} />
                </div>
                <p className="text-sm text-muted-foreground">
                  Your proof is saved locally and will finish anchoring within the hour. Track it in the Library.
                </p>
                <button
                  onClick={reset}
                  className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
                >
                  <UploadCloud className="h-4 w-4" />
                  Stamp another file
                </button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
