// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { ImageIcon, Trash2, Upload } from 'lucide-react';
import {
  BisonrelayStoreFile,
  bisonrelayStoreFileUrl,
  deleteBisonrelayStoreFile,
  getBisonrelayStoreProducts,
  listBisonrelayStoreFiles,
  uploadBisonrelayStoreFile,
} from '../../services/bisonrelayApi';
import { applyStoreTheme, refreshTheme } from '../../services/bisonrelayStoreTheme';
import { downscaleImageFile } from './storeImage';

const fmtSize = (n: number): string => {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(0)} KB`;
  return `${n} B`;
};

const isImage = (f: BisonrelayStoreFile): boolean => (f.mime ?? '').startsWith('image/');

// BisonrelayStoreAssets is the storefront media manager: browse/upload/delete the
// files under the store dir (cover images, the header banner, digital-download
// goods), with a dedicated banner slot and a one-click themed-layout install.
export const BisonrelayStoreAssets = () => {
  const [files, setFiles] = useState<BisonrelayStoreFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [folder, setFolder] = useState('');

  const load = () => {
    listBisonrelayStoreFiles()
      .then((f) => {
        setFiles(f);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Could not load files'));
  };
  useEffect(load, []);

  // After any file change, refresh the managed theme (no-op if not installed) so
  // the store reloads and ProcessEmbeds re-inlines the changed media.
  const afterChange = async () => {
    try {
      await refreshTheme(await getBisonrelayStoreProducts());
    } catch {
      /* best-effort */
    }
    load();
  };

  const onUpload = async (file: File) => {
    setBusy('upload');
    setErr(null);
    try {
      const dir = folder.trim().replace(/^\/+|\/+$/g, '');
      const path = dir ? `${dir}/${file.name}` : file.name;
      try {
        await uploadBisonrelayStoreFile(path, file);
      } catch (e: any) {
        const msg = String(e?.response?.data || e?.message || '');
        // brclientd refuses to clobber an existing file unless overwrite is set;
        // confirm before replacing.
        if (msg.includes('already exists') && window.confirm(`${path} already exists. Overwrite it?`)) {
          await uploadBisonrelayStoreFile(path, file, true);
        } else {
          throw e;
        }
      }
      await afterChange();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Upload failed');
    } finally {
      setBusy(null);
    }
  };

  const onBanner = async (file: File) => {
    setBusy('banner');
    setErr(null);
    try {
      const jpg = await downscaleImageFile(file, 1280, 0.85);
      await uploadBisonrelayStoreFile('banner.jpg', jpg, true);
      await afterChange();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Banner upload failed');
    } finally {
      setBusy(null);
    }
  };

  const onDelete = async (path: string) => {
    setBusy(path);
    setErr(null);
    try {
      await deleteBisonrelayStoreFile(path);
      await afterChange();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Delete failed');
    } finally {
      setBusy(null);
    }
  };

  const onApplyTheme = async () => {
    setBusy('theme');
    setErr(null);
    try {
      await applyStoreTheme(await getBisonrelayStoreProducts());
      load();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Could not apply theme');
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Store assets</h2>
        <p className="text-xs text-muted-foreground">
          Files under the store directory: cover images, the header banner, and digital-download
          goods. Templates are managed in the Templates tab.
        </p>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      <div className="rounded-xl border border-border/50 bg-gradient-card p-4 space-y-3">
        <div className="text-sm font-medium">Header banner</div>
        <div className="flex items-start gap-3">
          <img
            src={`${bisonrelayStoreFileUrl('banner.jpg')}&t=${files ? files.length : 0}`}
            alt=""
            onError={(e) => {
              (e.currentTarget as HTMLImageElement).style.display = 'none';
            }}
            className="h-16 w-auto rounded-md border border-border/40 bg-muted/20"
          />
          <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-muted/40 text-foreground text-xs font-medium hover:bg-muted/60 cursor-pointer">
            <ImageIcon className="h-4 w-4" />
            {busy === 'banner' ? 'Working…' : 'Replace banner'}
            <input
              type="file"
              accept="image/*"
              className="hidden"
              disabled={busy === 'banner'}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) onBanner(f);
                e.target.value = '';
              }}
            />
          </label>
        </div>
      </div>

      <div className="rounded-xl border border-border/50 bg-gradient-card p-4 space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex-1 min-w-[10rem]">
            <span className="text-xs text-muted-foreground">Upload to folder (optional)</span>
            <input
              value={folder}
              onChange={(e) => setFolder(e.target.value)}
              placeholder="covers, downloads, …"
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono"
            />
          </label>
          <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 cursor-pointer">
            <Upload className="h-4 w-4" />
            {busy === 'upload' ? 'Uploading…' : 'Upload file'}
            <input
              type="file"
              className="hidden"
              disabled={busy === 'upload'}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) onUpload(f);
                e.target.value = '';
              }}
            />
          </label>
        </div>

        {files === null ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : files.length === 0 ? (
          <div className="text-sm text-muted-foreground">No files yet.</div>
        ) : (
          <ul className="divide-y divide-border/40">
            {files.map((f) => (
              <li key={f.path} className="flex items-center gap-3 py-2">
                {isImage(f) ? (
                  <img
                    src={bisonrelayStoreFileUrl(f.path)}
                    alt=""
                    className="h-10 w-10 rounded object-cover border border-border/40"
                  />
                ) : (
                  <div className="h-10 w-10 rounded border border-border/40 bg-muted/20" />
                )}
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-mono truncate">{f.path}</div>
                  <div className="text-[11px] text-muted-foreground">
                    {fmtSize(f.size)}
                    {f.mime ? ` · ${f.mime}` : ''}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => onDelete(f.path)}
                  disabled={busy === f.path}
                  className="px-2 py-1 rounded text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 disabled:opacity-50"
                  title="Delete"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="rounded-xl border border-border/50 bg-gradient-card p-4 space-y-2">
        <div className="text-sm font-medium">Themed layout</div>
        <p className="text-xs text-muted-foreground">
          Installs a banner header, a category grid with cover images, and per-category pages,
          generated from your products. After this, adding a product or cover updates the
          storefront automatically. Overwrites the index, header, product and category templates.
        </p>
        <button
          type="button"
          onClick={onApplyTheme}
          disabled={busy === 'theme'}
          className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
        >
          {busy === 'theme' ? 'Applying…' : 'Apply storefront theme'}
        </button>
      </div>
    </div>
  );
};
