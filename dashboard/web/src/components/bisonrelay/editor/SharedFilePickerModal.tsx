// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, ArrowLeft, Coins, Loader2, X } from 'lucide-react';
import {
  BisonrelaySharedFile,
  getBisonrelaySharedFiles,
} from '../../../services/bisonrelayApi';
import { EditorEmbed } from './brEmbedBuilder';

interface Props {
  onClose: () => void;
  onSubmit: (embed: EditorEmbed) => void;
}

// SharedFilePickerModal is the second toolbar button in the BR editor.
// Two-step modal: list our shared files → set DCR price → return the
// embed-shape the editor splices into the post body. BR's `cost=` is in
// milliatoms (1 DCR = 1e11), but we collect DCR for UX and convert.
export const SharedFilePickerModal = ({ onClose, onSubmit }: Props) => {
  const [files, setFiles] = useState<BisonrelaySharedFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [picked, setPicked] = useState<BisonrelaySharedFile | null>(null);
  const [priceDcr, setPriceDcr] = useState('');

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

  const confirm = (e: React.FormEvent) => {
    e.preventDefault();
    if (!picked) return;
    const parsedDcr = parseFloat(priceDcr);
    const dcrAmount = Number.isFinite(parsedDcr) && parsedDcr > 0 ? parsedDcr : 0;
    // BR wire uses milli-atoms; 1 DCR = 1e11 matoms.
    const matoms = Math.round(dcrAmount * 1e11);
    onSubmit({
      displayName: picked.filename || picked.fid.slice(0, 12),
      download: picked.fid,
      filename: picked.filename,
      size: picked.size,
      cost: matoms,
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
              {picked ? 'Set price' : 'Link to shared content'}
            </h3>
            <p className="text-xs text-muted-foreground mt-1">
              {picked
                ? "Set a price for readers to pay before the file can be downloaded. Leave at zero for a free link."
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
                  onClick={() => {
                    setPicked(f);
                    // Pre-fill the DCR price if BR already has a non-zero
                    // cost configured on the share itself.
                    if (f.cost > 0) {
                      setPriceDcr(String(f.cost / 1e11));
                    }
                  }}
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
                          default {(f.cost / 1e11).toFixed(8).replace(/\.?0+$/, '')} DCR
                        </span>
                      </>
                    )}
                  </span>
                </button>
              ))
            )}
          </div>
        ) : (
          <form onSubmit={confirm} className="px-5 pb-5 pt-1 space-y-4">
            <div className="rounded-md bg-muted/20 border border-border/30 p-3 text-xs text-muted-foreground space-y-1">
              <div className="text-foreground font-medium truncate">{picked.filename}</div>
              <div className="font-mono text-[10px] break-all">{picked.fid}</div>
              <div>{formatBytes(picked.size)}</div>
            </div>
            <div>
              <label
                className="block text-xs text-muted-foreground mb-1"
                htmlFor="br-share-price"
              >
                Price (DCR)
              </label>
              <div className="relative">
                <Coins className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground/70 pointer-events-none" />
                <input
                  id="br-share-price"
                  type="number"
                  min="0"
                  step="any"
                  inputMode="decimal"
                  value={priceDcr}
                  onChange={(e) => setPriceDcr(e.target.value)}
                  placeholder="0"
                  className="w-full pl-8 pr-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
                />
              </div>
              <p className="text-[10px] text-muted-foreground mt-1">
                Leave at zero for a free download link. Readers pay over
                Lightning before BR releases the file content.
              </p>
            </div>
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
                type="submit"
                className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all"
              >
                Insert link
              </button>
            </div>
          </form>
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
