// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, Archive, Download, Loader2, Upload } from 'lucide-react';
import {
  PoliteiaArchiveImportResult,
  exportPoliteiaArchive,
  getPoliteiaArchive,
  importPoliteiaArchive,
} from '../../services/api';
import { apiError } from '../../utils/apiError';
import { downloadBlob } from '../../utils/files';

// PoliteiaArchiveCard shows the archive of finished proposals kept on the
// dashboard, with its export and import.
export const PoliteiaArchiveCard = ({ onImported }: { onImported: () => void }) => {
  const [count, setCount] = useState<number | null>(null);
  const [busy, setBusy] = useState<'export' | 'import' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PoliteiaArchiveImportResult | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const loadCount = async () => {
    try {
      setCount((await getPoliteiaArchive()).proposals);
    } catch {
      setCount(null);
    }
  };

  useEffect(() => {
    loadCount();
  }, []);

  const onExport = async () => {
    setBusy('export');
    setError(null);
    try {
      const resp = await exportPoliteiaArchive();
      downloadBlob(resp.data, 'dcrpulse-politeia-archive.json', 'application/json');
    } catch (err) {
      setError(apiError(err, 'Export failed'));
    } finally {
      setBusy(null);
    }
  };

  const onImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    setBusy('import');
    setError(null);
    setResult(null);
    try {
      setResult(await importPoliteiaArchive(f));
      await loadCount();
      onImported();
    } catch (err) {
      setError(apiError(err, 'Import failed'));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="p-4 rounded-xl bg-gradient-card border border-border/50 space-y-2 text-sm">
      <div className="flex flex-wrap items-center gap-3">
        <Archive className="h-4 w-4 text-muted-foreground" />
        <span className="text-muted-foreground">
          {count === null
            ? 'Archive of finished proposals'
            : `${count} finished proposal${count === 1 ? '' : 's'} archived on this dashboard, ` +
              'signatures verified.'}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            onClick={onExport}
            disabled={busy !== null || !count}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {busy === 'export' ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Download className="h-4 w-4" />
            )}
            Export
          </button>
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            disabled={busy !== null}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border/50 bg-background hover:bg-muted/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {busy === 'import' ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Upload className="h-4 w-4" />
            )}
            Import
          </button>
          <input
            ref={fileRef}
            type="file"
            accept="application/json,.json"
            aria-label="Archive file to import"
            className="hidden"
            onChange={onImportFile}
          />
        </div>
      </div>
      {result && (
        <div className="space-y-1">
          <p className="text-success">
            Imported {result.imported}, kept {result.kept} already archived at the same time or later.
          </p>
          {result.refused && result.refused.length > 0 && (
            <div className="text-destructive">
              <p>{result.refused.length} refused:</p>
              <ul className="list-disc pl-5">
                {result.refused.map((r) => (
                  <li key={r.token} className="break-all">
                    <span className="font-mono">{r.token}</span>: {r.reason}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
      {error && (
        <div className="text-destructive flex items-center gap-1">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      )}
    </div>
  );
};
