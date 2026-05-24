// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  Archive,
  Download,
  Loader2,
  Plus,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayDownloadItem,
  BisonrelayLiveEvent,
  BisonrelaySharedFile,
  cancelBisonrelayDownload,
  getBisonrelayContacts,
  getBisonrelayManageDownloads,
  getBisonrelaySharedFiles,
  shareBisonrelayFile,
  unshareBisonrelayFile,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

type Section = 'add' | 'shared' | 'downloads';

const readHashSection = (): Section => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('files')) return 'add';
  const rest = h.slice('files'.length);
  if (rest === '/shared') return 'shared';
  if (rest === '/downloads') return 'downloads';
  return 'add';
};

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

const formatBytes = (n: number): string => {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
};

// Shared-file costs are in atoms (1 DCR = 1e8), not the milli-atoms used for
// payment/tip records.
const formatDCR = (atoms: number): string => {
  if (!atoms) return 'Free';
  const dcr = atoms / 1e8;
  if (dcr < 0.0001) return `${atoms} atoms`;
  return `${dcr.toFixed(8).replace(/0+$/, '').replace(/\.$/, '')} DCR`;
};

export const BisonrelayFiles = () => {
  const [section, setSection] = useState<Section>(readHashSection);

  useEffect(() => {
    const onHashChange = () => setSection(readHashSection());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const content = (() => {
    if (section === 'shared') return <SharedListView />;
    if (section === 'downloads') return <DownloadsView />;
    return <AddContentView onShared={() => navigateTo('files/shared')} />;
  })();

  return (
    <div className="flex gap-4">
      <FilesSidebar active={section} />
      <div className="flex-1 min-w-0">{content}</div>
    </div>
  );
};

const sidebarItems: { id: Section; label: string; hash: string; icon: typeof Plus }[] = [
  { id: 'add', label: 'Add', hash: 'files', icon: Plus },
  { id: 'shared', label: 'Shared', hash: 'files/shared', icon: Archive },
  { id: 'downloads', label: 'Downloads', hash: 'files/downloads', icon: Download },
];

const FilesSidebar = ({ active }: { active: Section }) => (
  <aside className="w-44 shrink-0 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-2 self-start">
    <nav className="flex flex-col gap-1">
      {sidebarItems.map((item) => {
        const isActive = item.id === active;
        const Icon = item.icon;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => navigateTo(item.hash)}
            className={`w-full px-3 py-2 rounded-md text-sm flex items-center gap-2 text-left transition-colors ${
              isActive
                ? 'bg-primary/20 text-primary font-semibold'
                : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground'
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span>{item.label}</span>
          </button>
        );
      })}
    </nav>
  </aside>
);

// ---- Add view ------------------------------------------------------------

const AddContentView = ({ onShared }: { onShared: () => void }) => {
  const [file, setFile] = useState<File | null>(null);
  const [costDcr, setCostDcr] = useState('0');
  const [target, setTarget] = useState(''); // '' = global
  const [descr, setDescr] = useState('');
  const [contacts, setContacts] = useState<BisonrelayContact[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getBisonrelayContacts()
      .then(setContacts)
      .catch(() => {
        /* leave empty — Add still works for global shares */
      });
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file || submitting) return;
    const dcr = Number(costDcr);
    if (!Number.isFinite(dcr) || dcr < 0) {
      setErr('Cost must be a non-negative number');
      return;
    }
    setSubmitting(true);
    setErr(null);
    try {
      await shareBisonrelayFile(file, dcr, target, descr.trim());
      setFile(null);
      setCostDcr('0');
      setTarget('');
      setDescr('');
      onShared();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Share failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-4">
        <div>
          <h3 className="text-base font-semibold">Share a file</h3>
          <p className="text-xs text-muted-foreground mt-1">
            Make a local file available for download. Set an optional cost
            (in DCR) the requester pays before getting the bytes. Use
            "Sharing Preference" to share globally with subscribers, or
            target one specific contact.
          </p>
        </div>

        <div>
          <label
            htmlFor="br-share-file"
            className="block text-xs text-muted-foreground mb-1"
          >
            File
          </label>
          <label
            htmlFor="br-share-file"
            className="flex items-center gap-2 px-3 py-2 rounded-lg bg-background border border-border text-sm cursor-pointer hover:border-primary/60 transition-colors"
          >
            <Upload className="h-4 w-4 text-muted-foreground" />
            <span className="truncate text-foreground/90">
              {file ? `${file.name} (${formatBytes(file.size)})` : 'Choose a file…'}
            </span>
          </label>
          <input
            id="br-share-file"
            type="file"
            className="hidden"
            disabled={submitting}
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label
              htmlFor="br-share-cost"
              className="block text-xs text-muted-foreground mb-1"
            >
              Cost (DCR)
            </label>
            <input
              id="br-share-cost"
              type="number"
              min={0}
              step="0.00000001"
              value={costDcr}
              onChange={(e) => setCostDcr(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </div>
          <div>
            <label
              htmlFor="br-share-target"
              className="block text-xs text-muted-foreground mb-1"
            >
              Sharing Preference
            </label>
            <select
              id="br-share-target"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              disabled={submitting}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
            >
              <option value="">Global (all subscribers)</option>
              {contacts.map((c) => {
                const uid = c.id?.identity ?? '';
                const nick = c.nick_alias || c.id?.nick || uid.slice(0, 12);
                if (!uid) return null;
                return (
                  <option key={uid} value={uid}>
                    {nick}
                  </option>
                );
              })}
            </select>
          </div>
        </div>

        <div>
          <label
            htmlFor="br-share-descr"
            className="block text-xs text-muted-foreground mb-1"
          >
            Description (optional)
          </label>
          <input
            id="br-share-descr"
            type="text"
            value={descr}
            onChange={(e) => setDescr(e.target.value)}
            disabled={submitting}
            maxLength={200}
            placeholder="Shown to requesters in some clients"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
          />
        </div>

        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <button
            type="submit"
            disabled={!file || submitting}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            <Upload className="h-4 w-4" />
            Share
          </button>
        </div>
      </div>
    </form>
  );
};

// ---- Shared list view ----------------------------------------------------

const SharedListView = () => {
  const [items, setItems] = useState<BisonrelaySharedFile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const list = await getBisonrelaySharedFiles();
      setItems(list);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load shared files');
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const handleUnshare = async (fid: string) => {
    if (removing) return;
    setRemoving(fid);
    try {
      await unshareBisonrelayFile(fid);
      await reload();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Unshare failed');
    } finally {
      setRemoving(null);
    }
  };

  return (
    <div className="space-y-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {items === null && !err ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading shares…</span>
        </div>
      ) : items && items.length === 0 ? (
        <EmptyState
          icon={Archive}
          title="No shared files"
          hint='Use "Add" in the sidebar to share a file with a contact or globally with subscribers.'
        />
      ) : (
        <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden divide-y divide-border/30">
          {items?.map((f) => (
            <div
              key={f.fid}
              className="px-4 py-3 flex items-center gap-3 hover:bg-muted/20 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-foreground truncate flex items-center gap-2">
                  <span className="truncate">{f.filename || '(unnamed)'}</span>
                  {f.global ? (
                    <span className="shrink-0 text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-primary/15 text-primary">
                      Global
                    </span>
                  ) : (
                    <span className="shrink-0 text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-muted/30 text-muted-foreground">
                      Per-user
                    </span>
                  )}
                </div>
                <div className="text-[11px] text-muted-foreground mt-0.5 truncate">
                  {formatBytes(f.size)}
                  <span className="mx-1.5 opacity-50">·</span>
                  {formatDCR(f.cost)}
                  <span className="mx-1.5 opacity-50">·</span>
                  <span className="font-mono">{f.fid.slice(0, 16)}…</span>
                </div>
              </div>
              <button
                type="button"
                onClick={() => handleUnshare(f.fid)}
                disabled={removing === f.fid}
                title="Stop sharing this file"
                className="shrink-0 px-3 py-1.5 rounded-md border border-border/50 text-xs text-muted-foreground hover:text-destructive hover:border-destructive/40 transition-colors disabled:opacity-50 inline-flex items-center gap-1.5"
              >
                {removing === f.fid ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <Trash2 className="h-3 w-3" />
                )}
                Unshare
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

// ---- Downloads view ------------------------------------------------------

interface ProgressOverride {
  missing: number;
  total: number;
}

const DownloadsView = () => {
  const [items, setItems] = useState<BisonrelayDownloadItem[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [cancelling, setCancelling] = useState<string | null>(null);
  // Live progress overrides keyed by fid. Server snapshots refresh on
  // event arrival; in between we apply notif deltas so the bar moves
  // even before a re-fetch lands.
  const [progress, setProgress] = useState<Record<string, ProgressOverride>>({});
  const { addListener } = useBisonrelayLive();

  const reload = useCallback(async () => {
    try {
      const list = await getBisonrelayManageDownloads();
      setItems(list);
      setProgress({}); // server snapshot now authoritative
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load downloads');
    }
  }, []);

  useEffect(() => {
    reload();
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'file-download-progress') {
        const p = (evt.payload ?? {}) as Record<string, unknown>;
        const fid = String(p.fid ?? '');
        const total = Number(p.total_chunks ?? 0);
        const missing = Number(p.missing_chunks ?? 0);
        if (!fid) return;
        setProgress((prev) => ({ ...prev, [fid]: { missing, total } }));
      } else if (evt.type === 'file-download-completed') {
        // Re-fetch so the row flips to completed + grabs disk_path.
        reload();
      }
    });
  }, [addListener, reload]);

  const handleCancel = async (fid: string) => {
    if (cancelling) return;
    setCancelling(fid);
    try {
      await cancelBisonrelayDownload(fid);
      await reload();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Cancel failed');
    } finally {
      setCancelling(null);
    }
  };

  const rows = useMemo(() => {
    if (!items) return null;
    return items.map((it) => {
      const ov = progress[it.fid];
      const total = ov?.total ?? it.total_chunks;
      const missing = ov?.missing ?? it.missing_chunks;
      const pct = total > 0 ? Math.max(0, Math.min(100, ((total - missing) / total) * 100)) : 0;
      const done = total > 0 && missing === 0;
      return { it, total, missing, pct, done };
    });
  }, [items, progress]);

  return (
    <div className="space-y-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {rows === null && !err ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading downloads…</span>
        </div>
      ) : rows && rows.length === 0 ? (
        <EmptyState
          icon={Download}
          title="No active downloads"
          hint="In-flight and recently completed transfers appear here. Request a file from a contact (via a paid post or shared-file link) and progress will show up live."
        />
      ) : (
        <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden divide-y divide-border/30">
          {rows?.map(({ it, total, missing, pct, done }) => (
            <div key={it.fid} className="px-4 py-3 flex items-center gap-3">
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-foreground truncate flex items-center gap-2">
                  <span className="truncate">{it.filename || '(unnamed)'}</span>
                  <span
                    className={`shrink-0 text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded ${
                      it.is_sent
                        ? 'bg-muted/30 text-muted-foreground'
                        : 'bg-primary/15 text-primary'
                    }`}
                  >
                    {it.is_sent ? 'Sending' : 'Receiving'}
                  </span>
                </div>
                <div className="text-[11px] text-muted-foreground mt-0.5 truncate">
                  {it.nick || it.uid.slice(0, 12)}
                  <span className="mx-1.5 opacity-50">·</span>
                  {formatBytes(it.size)}
                  {total > 0 && (
                    <>
                      <span className="mx-1.5 opacity-50">·</span>
                      {total - missing}/{total} chunks
                    </>
                  )}
                </div>
                {!done && total > 0 && (
                  <div className="mt-1.5 h-1.5 rounded-full bg-muted/40 overflow-hidden">
                    <div
                      className="h-full bg-primary transition-[width] duration-200"
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                )}
                {done && it.disk_path && (
                  <div className="mt-1 text-[10px] text-muted-foreground font-mono break-all">
                    {it.disk_path}
                  </div>
                )}
              </div>
              {!done && !it.is_sent && (
                <button
                  type="button"
                  onClick={() => handleCancel(it.fid)}
                  disabled={cancelling === it.fid}
                  title="Cancel this download"
                  className="shrink-0 px-3 py-1.5 rounded-md border border-border/50 text-xs text-muted-foreground hover:text-destructive hover:border-destructive/40 transition-colors disabled:opacity-50 inline-flex items-center gap-1.5"
                >
                  {cancelling === it.fid ? (
                    <Loader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <X className="h-3 w-3" />
                  )}
                  Cancel
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

const EmptyState = ({
  icon: Icon,
  title,
  hint,
}: {
  icon: typeof Plus;
  title: string;
  hint: string;
}) => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-start gap-3">
    <div className="p-2 rounded-lg bg-primary/10 border border-primary/20 shrink-0">
      <Icon className="h-5 w-5 text-primary" />
    </div>
    <div className="space-y-1">
      <h3 className="text-sm font-semibold">{title}</h3>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  </div>
);
