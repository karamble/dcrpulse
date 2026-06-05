// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, ArrowLeft, Loader2, X } from 'lucide-react';
import {
  BisonrelaySharedFile,
  getBisonrelaySharedFiles,
} from '../../../services/bisonrelayApi';
import { EditorEmbed } from './brEmbedBuilder';

interface Props {
  onClose: () => void;
  onSubmit: (embed: EditorEmbed) => void;
}

// formatShareCost renders an atom amount as a trimmed DCR string. BR
// shared-file costs are in atoms (1 DCR = 1e8), distinct from the
// milli-atoms of payment records.
const formatShareCost = (atoms: number): string =>
  (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

// SharedFilePickerModal is the second toolbar button in the BR editor.
// Two-step modal: list our shared files -> confirm -> return the embed-shape
// the editor splices into the post body. The price is read-only: BR invoices
// downloads from the cost stored on the share (fixed when the file was
// shared), so the embed always advertises exactly what readers are charged.
export const SharedFilePickerModal = ({ onClose, onSubmit }: Props) => {
  const [files, setFiles] = useState<BisonrelaySharedFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [picked, setPicked] = useState<BisonrelaySharedFile | null>(null);

  useEffect(() => {
    getBisonrelaySharedFiles()
      .then((list) => {
        list.sort((a, b) => a.filename.localeCompare(b.filename));
        setFiles(list);
      })
      .catch((e: any) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load shared files');
      });
  }, []);

  // Plain action, not a form submit: this modal is rendered inline by the
  // editor, which can itself sit inside another <form> (the post composer).
  // A nested <form> submits unpredictably, so we avoid one entirely - the
  // editor toolbar uses type="button" for the same reason.
  const confirm = () => {
    if (!picked) return;
    onSubmit({
      displayName: picked.filename || picked.fid.slice(0, 12),
      download: picked.fid,
      filename: picked.filename,
      size: picked.size,
      cost: picked.cost,
    });
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[80vh]"
      >
        <div className="p-5 pb-3 flex items-start justify-between">
          <div>
            <h3 className="text-base font-semibold">
              {picked ? 'Confirm link' : 'Link to shared content'}
            </h3>
            <p className="text-xs text-muted-foreground mt-1">
              {picked
                ? 'Readers are charged the cost that was set when the file was shared. To charge for a file, share it with a cost under Files.'
                : "Pick a file you've already shared. The post will reference it by ID; the bytes don't travel inside the post body."}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {!picked ? (
          <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[160px]">
            {err ? (
              <div className="flex items-start gap-2 text-sm text-destructive px-3 py-4">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            ) : files === null ? (
              <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
                <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                <span>Loading shared files…</span>
              </div>
            ) : files.length === 0 ? (
              <p className="text-xs text-muted-foreground px-3 py-4 text-center">
                You haven't shared any files yet. Share a file first (BR client
                CLI / future dashboard surface) and it will show up here.
              </p>
            ) : (
              files.map((f) => (
                <button
                  key={f.fid}
                  type="button"
                  onClick={() => setPicked(f)}
                  className="w-full text-left px-3 py-2 rounded-md text-sm flex flex-col gap-0.5 hover:bg-muted/30 transition-colors"
                >
                  <span className="truncate font-medium text-foreground">
                    {f.filename || '(unnamed file)'}
                  </span>
                  <span className="text-[10px] text-muted-foreground flex items-center gap-2">
                    <span>{formatBytes(f.size)}</span>
                    {f.global && (
                      <>
                        <span className="opacity-50">·</span>
                        <span>global</span>
                      </>
                    )}
                    {f.cost > 0 && (
                      <>
                        <span className="opacity-50">·</span>
                        <span className="text-primary/80">
                          {formatShareCost(f.cost)} DCR
                        </span>
                      </>
                    )}
                  </span>
                </button>
              ))
            )}
          </div>
        ) : (
          <div className="px-5 pb-5 pt-1 space-y-4">
            <div className="rounded-md bg-muted/20 border border-border/30 p-3 text-xs text-muted-foreground space-y-1">
              <div className="text-foreground font-medium truncate">{picked.filename}</div>
              <div className="font-mono text-[10px] break-all">{picked.fid}</div>
              <div>{formatBytes(picked.size)}</div>
              <div className="pt-1">
                Price:{' '}
                <span className="text-foreground font-medium">
                  {picked.cost > 0
                    ? `${formatShareCost(picked.cost)} DCR`
                    : 'Free download'}
                </span>
              </div>
            </div>
            {picked.cost > 0 && (
              <p className="text-[10px] text-muted-foreground">
                Readers pay over Lightning before BR releases the file content.
              </p>
            )}
            <div className="flex justify-between gap-2 pt-1">
              <button
                type="button"
                onClick={() => setPicked(null)}
                className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors inline-flex items-center gap-1.5"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Pick a different file
              </button>
              <button
                type="button"
                onClick={confirm}
                className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all"
              >
                Insert link
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}
