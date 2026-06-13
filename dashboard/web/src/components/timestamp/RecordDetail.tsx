// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Download,
  ExternalLink,
  Loader2,
  Pencil,
  RotateCcw,
  ShieldCheck,
  Trash2,
  X,
} from 'lucide-react';
import { CopyButton } from '../explorer/CopyButton';
import {
  deleteTimestamp,
  getTimestamp,
  proofDownloadUrl,
  retryTimestamp,
  updateTimestamp,
  validateTimestamp,
  verifyTimestamp,
  type TimestampRecord,
  type Validation,
} from '../../services/timestampApi';
import { StatusBadge } from './StatusBadge';
import { StageList, type Stage, type StageState } from './StageList';
import { fmtBytes, fromUnix, shortHash } from './util';
import { toYMDTime } from '../../utils/date';

interface Props {
  digest: string;
  onClose: () => void;
  onChanged: () => void;
}

function validationStages(v: Validation): Stage[] {
  if (!v.hasProof) {
    return [{ key: 'wait', label: 'Not anchored yet', state: 'active', detail: v.note }];
  }
  const mark = (b: boolean): StageState => (b ? 'done' : 'fail');
  const anchorDate = fromUnix(v.blockTime);
  return [
    { key: 'mpath', label: 'Merkle proof path is valid', state: mark(v.merklePathValid) },
    { key: 'leaf', label: 'Your file is included in the timestamp', state: mark(v.digestInTree) },
    { key: 'root', label: 'Merkle root matches the proof', state: mark(v.rootMatches) },
    {
      key: 'chain',
      label: 'Committed on the Decred blockchain',
      state: mark(v.anchoredOnChain),
      detail: v.anchoredOnChain
        ? `Block ${v.blockHeight ?? '?'}${anchorDate ? ` · ${toYMDTime(anchorDate)}` : ''}${
            v.confirmations ? ` · ${v.confirmations} confirmations` : ''
          }`
        : v.note,
    },
  ];
}

const Row = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="flex flex-col sm:flex-row sm:gap-3 py-1.5 border-b border-border/20 last:border-0">
    <span className="sm:w-40 shrink-0 text-xs text-muted-foreground pt-0.5">{label}</span>
    <span className="min-w-0 flex-1 text-sm break-words">{children}</span>
  </div>
);

export const RecordDetail = ({ digest, onClose, onChanged }: Props) => {
  const [rec, setRec] = useState<TimestampRecord | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [validation, setValidation] = useState<Validation | null>(null);
  const [validating, setValidating] = useState(false);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [showPath, setShowPath] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await getTimestamp(digest);
      setRec(r);
      setTitle(r.title || '');
      setDescription(r.description || '');
      setTagsInput((r.tags || []).join(', '));
    } catch (e: any) {
      setError(e?.message || 'failed to load record');
    }
  }, [digest]);

  useEffect(() => {
    void load();
  }, [load]);

  const validate = async () => {
    setValidating(true);
    setError(null);
    try {
      setValidation(await validateTimestamp({ digest }));
    } catch (e: any) {
      setError(e?.message || 'validation failed');
    } finally {
      setValidating(false);
    }
  };

  const reverify = async () => {
    setBusy(true);
    setError(null);
    try {
      await verifyTimestamp(digest);
      await load();
      onChanged();
    } catch (e: any) {
      setError(e?.message || 'verification failed');
    } finally {
      setBusy(false);
    }
  };

  const retry = async () => {
    setBusy(true);
    setError(null);
    try {
      await retryTimestamp(digest);
      await load();
      onChanged();
    } catch (e: any) {
      setError(e?.message || 'retry failed');
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = async () => {
    setBusy(true);
    setError(null);
    try {
      await updateTimestamp(digest, {
        title: title.trim(),
        description: description.trim(),
        tags: tagsInput.split(',').map((t) => t.trim()).filter(Boolean),
      });
      setEditing(false);
      await load();
      onChanged();
    } catch (e: any) {
      setError(e?.message || 'update failed');
    } finally {
      setBusy(false);
    }
  };

  const doDelete = async () => {
    setBusy(true);
    try {
      await deleteTimestamp(digest);
      onChanged();
      onClose();
    } catch (e: any) {
      setError(e?.message || 'delete failed');
      setBusy(false);
    }
  };

  const anchored = rec?.status === 'anchored';
  const anchorDate = fromUnix(rec?.anchorTime);
  const merklePathText = rec?.merklePath ? JSON.stringify(rec.merklePath, null, 2) : '';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-fade-in">
      <div className="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl">
        <div className="flex items-center justify-between p-5 border-b border-border/50 sticky top-0 bg-card">
          <div className="min-w-0">
            <h3 className="text-lg font-semibold truncate">{rec?.title || rec?.filename || 'Timestamp'}</h3>
            {rec && <div className="mt-1"><StatusBadge status={rec.status} /></div>}
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground shrink-0">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-5 space-y-4">
          {error && (
            <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{error}</span>
            </div>
          )}

          {!rec ? (
            <div className="py-8 text-center text-muted-foreground">Loading…</div>
          ) : (
            <>
              {/* Metadata */}
              <div className="rounded-lg border border-border/40 p-3">
                {!editing ? (
                  <>
                    <Row label="Digest (SHA-256)">
                      <span className="inline-flex items-center gap-1.5">
                        <code className="font-mono text-xs break-all">{rec.digest}</code>
                        <CopyButton text={rec.digest} />
                      </span>
                    </Row>
                    <Row label="File">{rec.filename}</Row>
                    {rec.description && <Row label="Description">{rec.description}</Row>}
                    {rec.tags && rec.tags.length > 0 && (
                      <Row label="Tags">
                        <span className="flex flex-wrap gap-1">
                          {rec.tags.map((t) => (
                            <span key={t} className="px-2 py-0.5 rounded-full bg-muted/30 text-xs">
                              {t}
                            </span>
                          ))}
                        </span>
                      </Row>
                    )}
                    <Row label="Size">{fmtBytes(rec.fileSize)}{rec.mimeType ? ` · ${rec.mimeType}` : ''}</Row>
                    <Row label="Submitted">{rec.submittedAt ? toYMDTime(new Date(rec.submittedAt)) : '-'}</Row>
                    {rec.failReason && <Row label="Failure"><span className="text-destructive">{rec.failReason}</span></Row>}
                  </>
                ) : (
                  <div className="space-y-3">
                    <div>
                      <label className="block text-xs text-muted-foreground mb-1">Title</label>
                      <input
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-muted-foreground mb-1">Description</label>
                      <textarea
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        rows={2}
                        className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60 resize-y"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-muted-foreground mb-1">Tags (comma-separated)</label>
                      <input
                        value={tagsInput}
                        onChange={(e) => setTagsInput(e.target.value)}
                        className="w-full px-3 py-2 rounded-lg bg-background border border-border/60 text-sm focus:outline-none focus:border-primary/60"
                      />
                    </div>
                    <div className="flex gap-2">
                      <button
                        onClick={saveEdit}
                        disabled={busy}
                        className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold hover:opacity-90 disabled:opacity-50"
                      >
                        Save
                      </button>
                      <button
                        onClick={() => setEditing(false)}
                        className="px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                )}
              </div>

              {/* Anchor proof */}
              {(anchored || rec.txId) && (
                <div className="rounded-lg border border-border/40 p-3">
                  <h4 className="text-sm font-semibold mb-2">Anchor proof</h4>
                  {anchorDate && <Row label="Anchored">{toYMDTime(anchorDate)}</Row>}
                  {rec.merkleRoot && (
                    <Row label="Merkle root">
                      <span className="inline-flex items-center gap-1.5">
                        <code className="font-mono text-xs break-all">{shortHash(rec.merkleRoot, 16, 12)}</code>
                        <CopyButton text={rec.merkleRoot} />
                      </span>
                    </Row>
                  )}
                  {rec.txId && (
                    <Row label="Anchor tx">
                      <Link to={`/explorer/tx/${rec.txId}`} className="inline-flex items-center gap-1 text-primary hover:underline font-mono text-xs break-all">
                        {shortHash(rec.txId, 16, 12)}
                        <ExternalLink className="h-3 w-3 shrink-0" />
                      </Link>
                    </Row>
                  )}
                  {(rec.confirmations || rec.minConfirmations) ? (
                    <Row label="Confirmations">{rec.confirmations ?? 0}{rec.minConfirmations ? ` / ${rec.minConfirmations}` : ''}</Row>
                  ) : null}
                  {merklePathText && (
                    <div className="mt-2">
                      <button
                        onClick={() => setShowPath((s) => !s)}
                        className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                      >
                        {showPath ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                        Merkle path
                      </button>
                      {showPath && (
                        <pre className="mt-1 p-2 rounded bg-background border border-border/40 text-xs font-mono overflow-x-auto max-h-48">
                          {merklePathText}
                        </pre>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* On-chain validation */}
              <div className="rounded-lg border border-border/40 p-3">
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-semibold">On-chain validation</h4>
                  <button
                    onClick={validate}
                    disabled={validating}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20 disabled:opacity-50"
                  >
                    {validating ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
                    Validate
                  </button>
                </div>
                {validation ? (
                  <div className="mt-3">
                    <StageList stages={validationStages(validation)} />
                  </div>
                ) : (
                  <p className="mt-2 text-xs text-muted-foreground">
                    Re-checks the proof against the Decred chain using this dashboard's own node.
                  </p>
                )}
              </div>

              {/* Actions */}
              <div className="flex flex-wrap gap-2">
                {anchored && (
                  <a
                    href={proofDownloadUrl(digest)}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
                  >
                    <Download className="h-4 w-4" />
                    Download proof
                  </a>
                )}
                <button
                  onClick={reverify}
                  disabled={busy}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20 disabled:opacity-50"
                >
                  <RotateCcw className="h-4 w-4" />
                  Re-verify
                </button>
                {rec.status === 'failed' && (
                  <button
                    onClick={retry}
                    disabled={busy}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20 disabled:opacity-50"
                  >
                    <RotateCcw className="h-4 w-4" />
                    Retry submit
                  </button>
                )}
                {!editing && (
                  <button
                    onClick={() => setEditing(true)}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
                  >
                    <Pencil className="h-4 w-4" />
                    Edit
                  </button>
                )}
                {confirmDelete ? (
                  <span className="inline-flex items-center gap-1.5">
                    <button
                      onClick={doDelete}
                      disabled={busy}
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-destructive/15 text-destructive border border-destructive/30 text-sm hover:bg-destructive/25 disabled:opacity-50"
                    >
                      <Trash2 className="h-4 w-4" />
                      Confirm delete
                    </button>
                    <button
                      onClick={() => setConfirmDelete(false)}
                      className="px-3 py-1.5 rounded-lg border border-border/60 text-sm hover:bg-muted/20"
                    >
                      Cancel
                    </button>
                  </span>
                ) : (
                  <button
                    onClick={() => setConfirmDelete(true)}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-destructive/30 text-destructive text-sm hover:bg-destructive/10"
                  >
                    <Trash2 className="h-4 w-4" />
                    Delete
                  </button>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
