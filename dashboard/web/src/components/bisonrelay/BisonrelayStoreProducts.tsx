// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Pencil, Plus, Trash2 } from 'lucide-react';
import {
  BisonrelayStoreProduct,
  deleteBisonrelayStoreProduct,
  getBisonrelayStoreProducts,
  saveBisonrelayStoreProduct,
  uploadBisonrelayStoreFile,
} from '../../services/bisonrelayApi';

const emptyProduct = (): BisonrelayStoreProduct => ({
  title: '',
  sku: '',
  description: '',
  tags: [],
  price: 0,
  shipping: false,
  disabled: false,
});

// BisonrelayStoreProducts manages the storefront catalog: list, add, edit and
// delete products. brclientd writes them as TOML and the simplestore
// live-reloads, so changes reach customers without a restart.
export const BisonrelayStoreProducts = () => {
  const [products, setProducts] = useState<BisonrelayStoreProduct[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState<BisonrelayStoreProduct | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  const load = () => {
    getBisonrelayStoreProducts()
      .then((p) => {
        setProducts(p);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Could not load products'));
  };
  useEffect(load, []);

  const startAdd = () => {
    setEditing(emptyProduct());
    setIsNew(true);
  };
  const startEdit = (p: BisonrelayStoreProduct) => {
    setEditing({ ...p, tags: p.tags ?? [] });
    setIsNew(false);
  };

  const save = async () => {
    if (!editing) return;
    setSaving(true);
    setErr(null);
    try {
      await saveBisonrelayStoreProduct(editing);
      setEditing(null);
      load();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const remove = async (sku: string) => {
    setErr(null);
    try {
      await deleteBisonrelayStoreProduct(sku);
      load();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Delete failed');
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Storefront products</h2>
          <p className="text-xs text-muted-foreground">
            Your catalog. Prices are in USD; the store charges the DCR equivalent at order time.
          </p>
        </div>
        {!editing && (
          <button
            type="button"
            onClick={startAdd}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
          >
            <Plus className="h-4 w-4" />
            Add product
          </button>
        )}
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      {editing ? (
        <ProductForm
          value={editing}
          isNew={isNew}
          saving={saving}
          onChange={setEditing}
          onCancel={() => setEditing(null)}
          onSave={save}
        />
      ) : products === null ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : products.length === 0 ? (
        <div className="rounded-xl border border-border/50 bg-gradient-card p-6 text-sm text-muted-foreground">
          No products yet. Add one to stock your storefront.
        </div>
      ) : (
        <ul className="rounded-xl border border-border/50 bg-gradient-card divide-y divide-border/40">
          {products.map((p) => (
            <li key={p.sku} className="flex items-center gap-3 px-4 py-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm truncate">{p.title || '(untitled)'}</span>
                  {p.disabled && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/50 text-muted-foreground">
                      hidden
                    </span>
                  )}
                  {p.shipping && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted/50 text-muted-foreground">
                      ships
                    </span>
                  )}
                </div>
                <div className="text-[11px] text-muted-foreground font-mono truncate">
                  {p.sku} · ${p.price}
                  {p.tags && p.tags.length > 0 ? ` · ${p.tags.join(', ')}` : ''}
                </div>
              </div>
              <button
                type="button"
                onClick={() => startEdit(p)}
                className="px-2 py-1 rounded text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30"
                title="Edit"
              >
                <Pencil className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={() => remove(p.sku)}
                className="px-2 py-1 rounded text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                title="Delete"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

const ProductForm = ({
  value,
  isNew,
  saving,
  onChange,
  onCancel,
  onSave,
}: {
  value: BisonrelayStoreProduct;
  isNew: boolean;
  saving: boolean;
  onChange: (p: BisonrelayStoreProduct) => void;
  onCancel: () => void;
  onSave: () => void;
}) => {
  const set = (patch: Partial<BisonrelayStoreProduct>) => onChange({ ...value, ...patch });
  const [uploading, setUploading] = useState(false);
  const [uploadErr, setUploadErr] = useState<string | null>(null);
  const onUpload = async (file: File) => {
    setUploading(true);
    setUploadErr(null);
    try {
      const stored = await uploadBisonrelayStoreFile(value.sendfilename ?? '', file);
      set({ sendfilename: stored });
    } catch (e: any) {
      setUploadErr(e?.response?.data || e?.message || 'Upload failed');
    } finally {
      setUploading(false);
    }
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSave();
      }}
      className="space-y-3 rounded-xl border border-border/50 bg-gradient-card p-4 text-sm"
    >
      <label className="block">
        <span className="text-xs text-muted-foreground">Title</span>
        <input
          value={value.title}
          onChange={(e) => set({ title: e.target.value })}
          className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </label>
      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <span className="text-xs text-muted-foreground">SKU</span>
          <input
            value={value.sku}
            onChange={(e) => set({ sku: e.target.value })}
            disabled={!isNew}
            placeholder="letters, digits, dash, underscore"
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono disabled:opacity-60"
          />
        </label>
        <label className="block">
          <span className="text-xs text-muted-foreground">Price (USD)</span>
          <input
            type="number"
            step="0.01"
            value={value.price}
            onChange={(e) => set({ price: parseFloat(e.target.value) || 0 })}
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </label>
      </div>
      <label className="block">
        <span className="text-xs text-muted-foreground">Description</span>
        <textarea
          value={value.description}
          onChange={(e) => set({ description: e.target.value })}
          rows={4}
          className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </label>
      <label className="block">
        <span className="text-xs text-muted-foreground">Tags (comma-separated)</span>
        <input
          value={(value.tags ?? []).join(', ')}
          onChange={(e) =>
            set({
              tags: e.target.value
                .split(',')
                .map((t) => t.trim())
                .filter(Boolean),
            })
          }
          className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </label>
      <div className="rounded-md border border-border/50 p-3 space-y-2">
        <div className="text-xs font-medium text-foreground">Digital download (optional)</div>
        <p className="text-[11px] text-muted-foreground">
          Delivered to the buyer automatically once their order's invoice is paid.
        </p>
        <label className="block">
          <span className="text-xs text-muted-foreground">File path (under the store dir)</span>
          <input
            value={value.sendfilename ?? ''}
            onChange={(e) => set({ sendfilename: e.target.value })}
            placeholder="ebooks/title.pdf"
            className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono"
          />
        </label>
        <div className="flex items-center gap-2">
          <label className="px-3 py-1.5 rounded-md bg-muted/40 text-foreground text-xs font-medium hover:bg-muted/60 cursor-pointer">
            {uploading ? 'Uploading…' : 'Upload file'}
            <input
              type="file"
              className="hidden"
              disabled={uploading}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) onUpload(f);
                e.target.value = '';
              }}
            />
          </label>
          <span className="text-[11px] text-muted-foreground">
            uploads to the path above, or the file's name if blank
          </span>
        </div>
        {uploadErr && <div className="text-[11px] text-rose-300">{uploadErr}</div>}
      </div>

      <div className="flex items-center gap-4">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={value.shipping}
            onChange={(e) => set({ shipping: e.target.checked })}
          />
          Requires shipping address
        </label>
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={value.disabled}
            onChange={(e) => set({ disabled: e.target.checked })}
          />
          Hidden (not listed)
        </label>
      </div>
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
        >
          {saving ? 'Saving…' : isNew ? 'Add product' : 'Save changes'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
        >
          Cancel
        </button>
      </div>
    </form>
  );
};
