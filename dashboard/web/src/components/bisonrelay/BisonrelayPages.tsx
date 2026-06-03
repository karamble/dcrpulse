// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { FormEvent, MouseEvent, useCallback, useEffect, useMemo, useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  FileText,
  Globe,
  Home,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react';
import {
  BisonrelayFetchedPage,
  BisonrelayLocalPage,
  BisonrelayPageFormField,
  BisonrelayPageSegment,
  bisonrelayContentFileUrl,
  deleteBisonrelayLocalPage,
  fetchBisonrelayPage,
  getBisonrelayIdentity,
  getBisonrelayLocalPage,
  getBisonrelayManageDownloads,
  getBisonrelayRates,
  listBisonrelayLocalPages,
  saveBisonrelayLocalPage,
  startBisonrelayContentGet,
} from '../../services/bisonrelayApi';
import { BisonrelayEditor, composeBRBody, EditorEmbedMap } from './editor';
import { BisonrelayStoreModePanel } from './BisonrelayStoreMode';
import { BisonrelayStoreManager } from './BisonrelayStoreManager';
import { BR_PROSE_CLASSES } from './bisonrelayProse';

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

// toHexId normalizes a BR identity to 64-char hex. /br/identity returns the
// local identity base64-encoded, which contains '/' and breaks the '/'-
// delimited page hash; contacts are already hex. Hex is URL/hash-safe and is
// the form brclientd's /pages/fetch expects.
const toHexId = (s: string): string => {
  if (/^[0-9a-f]{64}$/i.test(s)) return s;
  try {
    const bin = atob(s);
    if (bin.length !== 32) return s;
    let hex = '';
    for (let i = 0; i < bin.length; i++) {
      hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  } catch {
    return s;
  }
};

type View =
  | { kind: 'mine' }
  | { kind: 'new' }
  | { kind: 'edit'; name: string }
  | { kind: 'visit'; uid: string; path: string[] };

// readHash parses the Pages sub-route out of the URL hash:
//   #pages, #pages/mine          -> My Pages list
//   #pages/new                   -> new local page editor
//   #pages/edit/<name>           -> edit a local page
//   #pages/visit/<uid>/<path...> -> view a (local or remote) page
const readHash = (): View => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('pages')) return { kind: 'mine' };
  const rest = h.slice('pages'.length).replace(/^\//, '');
  if (rest === '' || rest === 'mine') return { kind: 'mine' };
  if (rest === 'new') return { kind: 'new' };
  if (rest.startsWith('edit/')) {
    return { kind: 'edit', name: decodeURIComponent(rest.slice('edit/'.length)) };
  }
  if (rest.startsWith('visit/')) {
    const segs = rest
      .slice('visit/'.length)
      .split('/')
      .filter(Boolean)
      .map(decodeURIComponent);
    const uid = segs.shift() ?? '';
    return { kind: 'visit', uid, path: segs.length ? segs : ['index.md'] };
  }
  return { kind: 'mine' };
};

export const BisonrelayPages = () => {
  const [view, setView] = useState<View>(readHash);
  const [ownId, setOwnId] = useState<string>('');

  useEffect(() => {
    const onHashChange = () => setView(readHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    getBisonrelayIdentity()
      .then((id) => setOwnId(toHexId(id.identity ?? '')))
      .catch(() => {
        /* identity unavailable; "view my page" stays disabled */
      });
  }, []);

  const content = (() => {
    switch (view.kind) {
      case 'new':
        return <PageEditorView ownId={ownId} />;
      case 'edit':
        return <PageEditorView ownId={ownId} name={view.name} />;
      case 'visit':
        return (
          <PageView
            key={`${view.uid}/${view.path.join('/')}`}
            initialUid={view.uid}
            initialPath={view.path}
            ownId={ownId}
          />
        );
      default:
        return <MyPagesView ownId={ownId} />;
    }
  })();

  const section: 'mine' | 'visit' = view.kind === 'visit' ? 'visit' : 'mine';

  return (
    <div className="flex flex-col md:flex-row gap-4">
      <PagesSidebar active={section} ownId={ownId} />
      <div className="flex-1 min-w-0">{content}</div>
    </div>
  );
};

const PagesSidebar = ({ active, ownId }: { active: 'mine' | 'visit'; ownId: string }) => {
  const items: { id: 'mine' | 'visit'; label: string; hash: string; icon: typeof FileText }[] = [
    { id: 'mine', label: 'My Pages', hash: 'pages', icon: FileText },
    { id: 'visit', label: 'Visit', hash: 'pages/visit', icon: Globe },
  ];
  return (
    <aside className="md:w-44 shrink-0 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-2 md:self-start">
      <nav className="flex md:flex-col gap-1 overflow-x-auto md:overflow-visible">
        {items.map((item) => {
          const isActive = item.id === active;
          const Icon = item.icon;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => navigateTo(item.hash)}
              className={`shrink-0 whitespace-nowrap md:w-full px-3 py-2 rounded-md text-sm flex items-center gap-2 text-left transition-colors ${
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
      {ownId && (
        <button
          type="button"
          onClick={() => navigateTo(`pages/visit/${ownId}/index.md`)}
          className="mt-3 w-full px-3 py-2 rounded-md text-xs flex items-center gap-2 text-left text-muted-foreground hover:bg-muted/30 hover:text-foreground transition-colors"
          title="Render your own hosted page as visitors see it"
        >
          <Home className="h-3.5 w-3.5 shrink-0" />
          <span>Preview my page</span>
        </button>
      )}
    </aside>
  );
};

// ---- My Pages (hosting) --------------------------------------------------

const MyPagesView = ({ ownId }: { ownId: string }) => {
  const [pages, setPages] = useState<BisonrelayLocalPage[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [hostMode, setHostMode] = useState<'off' | 'pages' | 'store'>('off');

  const refresh = useCallback(() => {
    setLoading(true);
    listBisonrelayLocalPages()
      .then((p) => {
        setPages(p);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.message || 'Could not load pages'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const onDelete = async (name: string) => {
    if (!window.confirm(`Delete page "${name}"? This cannot be undone.`)) return;
    try {
      await deleteBisonrelayLocalPage(name);
      refresh();
    } catch (e: any) {
      setErr(e?.message || 'Delete failed');
    }
  };

  return (
    <div className="space-y-4">
      <BisonrelayStoreModePanel onModeChange={(m) => setHostMode(m.mode)} />

      {hostMode === 'store' ? (
        <BisonrelayStoreManager />
      ) : (
        <>
      {hostMode === 'off' && (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
          Hosting is deactivated. These pages are saved on disk but are not served over Bison Relay
          until you switch hosting to static pages above.
        </div>
      )}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">My Pages</h2>
          <p className="text-xs text-muted-foreground">
            Markdown pages you host. Others fetch them over Bison Relay; index.md is your root.
          </p>
        </div>
        <button
          type="button"
          onClick={() => navigateTo('pages/new')}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
        >
          <Plus className="h-4 w-4" />
          New page
        </button>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      {loading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : pages.length === 0 ? (
        <div className="rounded-xl border border-border/50 bg-gradient-card p-6 text-sm text-muted-foreground">
          No pages yet. Create one to start hosting.
        </div>
      ) : (
        <ul className="rounded-xl border border-border/50 bg-gradient-card divide-y divide-border/40">
          {pages.map((p) => (
            <li key={p.name} className="flex items-center gap-3 px-4 py-3">
              <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="font-mono text-sm truncate">{p.name}</div>
                <div className="text-[11px] text-muted-foreground">
                  {p.size} bytes · {new Date(p.modified * 1000).toLocaleString()}
                </div>
              </div>
              {ownId && (
                <button
                  type="button"
                  onClick={() => navigateTo(`pages/visit/${ownId}/${encodeURIComponent(p.name)}`)}
                  className="px-2 py-1 rounded text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30"
                  title="Preview as visitors see it"
                >
                  <Globe className="h-4 w-4" />
                </button>
              )}
              <button
                type="button"
                onClick={() => navigateTo(`pages/edit/${encodeURIComponent(p.name)}`)}
                className="px-2 py-1 rounded text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30"
                title="Edit"
              >
                <Pencil className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={() => onDelete(p.name)}
                className="px-2 py-1 rounded text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                title="Delete"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
        </>
      )}
    </div>
  );
};

const PageEditorView = ({ ownId, name }: { ownId: string; name?: string }) => {
  const editing = !!name;
  const [fileName, setFileName] = useState(name ?? '');
  const [body, setBody] = useState('');
  const [embeds, setEmbeds] = useState<EditorEmbedMap>({});
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(!editing);

  useEffect(() => {
    if (!editing || !name) return;
    getBisonrelayLocalPage(name)
      .then((content) => {
        setBody(content);
        setLoaded(true);
      })
      .catch((e: any) => {
        setErr(e?.message || 'Could not load page');
        setLoaded(true);
      });
  }, [editing, name]);

  const normalizedName = (raw: string): string => {
    const trimmed = raw.trim();
    if (!trimmed) return '';
    return trimmed.endsWith('.md') ? trimmed : `${trimmed}.md`;
  };

  const onSave = async () => {
    const finalName = normalizedName(fileName);
    // A .md file, optionally under subdirectories (docs/intro.md); mirrors
    // brclientd's pageNameRE. The ".." guard keeps it inside PagesDir.
    if (!/^([A-Za-z0-9_.-]+\/)*[A-Za-z0-9_.-]+\.md$/.test(finalName) || finalName.includes('..')) {
      setErr('Name must be a .md file (letters, digits, dash, underscore, dot; / for subfolders).');
      return;
    }
    setSaving(true);
    try {
      await saveBisonrelayLocalPage(finalName, composeBRBody(body, embeds));
      navigateTo('pages');
    } catch (e: any) {
      setErr(e?.message || 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{editing ? `Edit ${name}` : 'New page'}</h2>
        <button
          type="button"
          onClick={() => navigateTo('pages')}
          className="text-sm text-muted-foreground hover:text-foreground"
        >
          Cancel
        </button>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      <label className="block">
        <span className="text-xs text-muted-foreground">File name</span>
        <input
          type="text"
          value={fileName}
          disabled={editing}
          onChange={(e) => setFileName(e.target.value)}
          placeholder="about.md"
          className="mt-1 w-full max-w-xs rounded-md border border-border bg-background px-3 py-1.5 text-sm font-mono disabled:opacity-60"
        />
      </label>

      {loaded && (
        <BisonrelayEditor
          value={body}
          onChange={setBody}
          embeds={embeds}
          onEmbedsChange={setEmbeds}
          features={{ pageBlocks: true }}
          placeholder="# My page&#10;&#10;Write markdown here. Link other pages with [text](other.md)."
        />
      )}

      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onSave}
          disabled={saving || !ownId}
          className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50 transition-colors"
        >
          {saving ? 'Saving…' : 'Save page'}
        </button>
      </div>
    </div>
  );
};

// ---- Viewer (fetch + render + navigate) ----------------------------------

interface PageTarget {
  uid: string;
  path: string[];
  sessionId: number;
  parentPage: number;
}

const PageView = ({
  initialUid,
  initialPath,
  ownId,
}: {
  initialUid: string;
  initialPath: string[];
  ownId: string;
}) => {
  const [history, setHistory] = useState<PageTarget[]>([
    { uid: initialUid, path: initialPath, sessionId: 0, parentPage: 0 },
  ]);
  const [cursor, setCursor] = useState(0);
  const [page, setPage] = useState<BisonrelayFetchedPage | null>(null);
  const [segments, setSegments] = useState<BisonrelayPageSegment[]>([]);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const current = history[cursor];

  const load = useCallback(async (target: PageTarget): Promise<PageTarget> => {
    setLoading(true);
    setErr(null);
    try {
      const res = await fetchBisonrelayPage({
        uid: target.uid,
        path: target.path,
        session_id: target.sessionId || undefined,
        parent_page: target.parentPage || undefined,
      });
      setPage(res);
      setSegments(res.segments ?? []);
      if (res.status !== 200) {
        setErr(`Page returned status ${res.status}.`);
      }
      return { ...target, sessionId: res.session_id, parentPage: res.page_id };
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Fetch failed');
      setPage(null);
      setSegments([]);
      return target;
    } finally {
      setLoading(false);
    }
  }, []);

  // (Re)load whenever the cursor moves to a target whose page isn't shown yet.
  useEffect(() => {
    let cancelled = false;
    load(history[cursor]).then((updated) => {
      if (cancelled) return;
      // Persist the session id learned from the reply onto this history slot
      // so navigation within the session reuses it.
      setHistory((h) => h.map((t, i) => (i === cursor ? updated : t)));
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cursor]);

  // navigate pushes a new target (truncating any forward history). Same-uid
  // navigation reuses the current session for continuity; a different uid
  // starts a fresh session.
  const navigate = useCallback(
    (uid: string, path: string[]) => {
      setHistory((h) => {
        const base = h.slice(0, cursor + 1);
        const cur = h[cursor];
        const sameUser = cur && cur.uid === uid;
        base.push({
          uid,
          path,
          sessionId: sameUser ? cur.sessionId : 0,
          parentPage: sameUser ? cur.parentPage : 0,
        });
        return base;
      });
      setCursor((c) => c + 1);
    },
    [cursor],
  );

  const goBack = () => cursor > 0 && setCursor((c) => c - 1);
  const goForward = () => cursor < history.length - 1 && setCursor((c) => c + 1);

  // applyAsync replaces the segments belonging to one --section id=X-- region
  // with the reply's segments (tagged so a later update can replace them too).
  const applyAsync = useCallback((targetId: string, replySegs: BisonrelayPageSegment[]) => {
    setSegments((segs) => {
      const tagged = replySegs.map((s) => ({ ...s, section_id: targetId }));
      const firstIdx = segs.findIndex((s) => s.section_id === targetId);
      if (firstIdx < 0) return segs; // section not present; ignore
      const before = segs.slice(0, firstIdx).filter((s) => s.section_id !== targetId);
      const after = segs.slice(firstIdx).filter((s) => s.section_id !== targetId);
      return [...before, ...tagged, ...after];
    });
  }, []);

  const submitForm = useCallback(
    async (
      actionPath: string[],
      formData: Record<string, unknown>,
      asyncTargetId: string,
    ): Promise<string | null> => {
      const cur = history[cursor];
      try {
        const res = await fetchBisonrelayPage({
          uid: cur.uid,
          path: actionPath,
          session_id: cur.sessionId || undefined,
          parent_page: cur.parentPage || undefined,
          data: formData,
          async_target_id: asyncTargetId || undefined,
        });
        // A non-200 reply (e.g. a static host can't process the action, or a
        // dynamic host rejected it) carries no usable content; report it back
        // to the form so the feedback appears where the user clicked, instead
        // of blanking the page or wiping the target section.
        if (res.status !== 200) {
          return `Form action failed: page returned status ${res.status}.`;
        }
        if (asyncTargetId) {
          applyAsync(asyncTargetId, res.segments ?? []);
        } else {
          setPage(res);
          setSegments(res.segments ?? []);
        }
        return null;
      } catch (e: any) {
        const body = e?.response?.data;
        return typeof body === 'string' ? body : e?.message || 'Form submit failed';
      }
    },
    [history, cursor, applyAsync],
  );

  const title = current ? `${shortUid(current.uid, ownId)} / ${current.path.join('/')}` : '';

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={goBack}
          disabled={cursor === 0}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-30"
          title="Back"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={goForward}
          disabled={cursor >= history.length - 1}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-30"
          title="Forward"
        >
          <ArrowRight className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={() => load(history[cursor])}
          className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30"
          title="Reload"
        >
          <RefreshCw className="h-4 w-4" />
        </button>
        <div className="flex-1 min-w-0 truncate font-mono text-xs text-muted-foreground" title={title}>
          {title}
        </div>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      <div className="rounded-xl border border-border/50 bg-gradient-card p-5 min-h-[8rem]">
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading page…</div>
        ) : page && page.status === 200 ? (
          <PageSegments
            segments={segments}
            currentUid={current.uid}
            onNavigate={navigate}
            onSubmitForm={submitForm}
          />
        ) : !err ? (
          <div className="text-sm text-muted-foreground">Nothing to show.</div>
        ) : null}
      </div>
    </div>
  );
};

const shortUid = (uid: string, ownId: string): string => {
  if (uid && uid === ownId) return 'me';
  return uid ? `${uid.slice(0, 8)}…` : '';
};

// PageSegments renders the structured page and intercepts in-app links.
const PageSegments = ({
  segments,
  currentUid,
  onNavigate,
  onSubmitForm,
}: {
  segments: BisonrelayPageSegment[];
  currentUid: string;
  onNavigate: (uid: string, path: string[]) => void;
  onSubmitForm: (
    actionPath: string[],
    data: Record<string, unknown>,
    asyncTargetId: string,
  ) => Promise<string | null>;
}) => {
  // Intercept clicks on br:// and relative links so they navigate in-app
  // rather than reloading the dashboard. Fully-qualified http(s) links keep
  // their default (open in a new tab via the renderer's target=_blank).
  const onClick = (e: MouseEvent<HTMLDivElement>) => {
    const anchor = (e.target as HTMLElement).closest('a');
    if (!anchor) return;
    const href = anchor.getAttribute('href') || '';
    // Leave the browser to handle external links, mail, and embed download
    // links (data:/blob: URIs carry a download attr); only in-app page links
    // (relative or br://) are intercepted below.
    if (
      /^https?:\/\//i.test(href) ||
      href.startsWith('mailto:') ||
      href.startsWith('data:') ||
      href.startsWith('blob:') ||
      anchor.hasAttribute('download')
    )
      return;
    e.preventDefault();
    const resolved = resolvePageLink(href, currentUid);
    if (resolved) onNavigate(resolved.uid, resolved.path);
  };

  return (
    <div className="space-y-3" onClick={onClick}>
      {segments.map((seg, i) => {
        if (seg.kind === 'text' && seg.html) {
          return (
            <div key={i} className={BR_PROSE_CLASSES} dangerouslySetInnerHTML={{ __html: seg.html }} />
          );
        }
        if (seg.kind === 'embed' && seg.download && !seg.data_b64) {
          return <PageDownloadEmbed key={i} seg={seg} uid={currentUid} />;
        }
        if (seg.kind === 'embed' && seg.data_b64) {
          const isImage = !!seg.mime && seg.mime.startsWith('image/');
          if (isImage) {
            return (
              <img
                key={i}
                src={`data:${seg.mime};base64,${seg.data_b64}`}
                alt={seg.alt || seg.name || ''}
                className="rounded-lg border border-border/40 max-w-full h-auto"
              />
            );
          }
          const href = `data:${seg.mime || 'application/octet-stream'};base64,${seg.data_b64}`;
          return (
            <a
              key={i}
              href={href}
              download={seg.name || 'attachment'}
              className="inline-block text-xs text-primary underline hover:no-underline"
            >
              {seg.name || 'attachment'} ({seg.mime || 'binary'})
            </a>
          );
        }
        if (seg.kind === 'form' && seg.fields) {
          return <PageForm key={i} fields={seg.fields} onSubmit={onSubmitForm} />;
        }
        return null;
      })}
    </div>
  );
};

// resolvePageLink turns a markdown href into a (uid, path) navigation target.
// br://UID/seg/seg -> that user's page; a relative href -> the current user's
// page at that absolute path (matches bruig's launchUrlAwait behaviour).
const resolvePageLink = (href: string, currentUid: string): { uid: string; path: string[] } | null => {
  if (!href) return null;
  if (href.startsWith('br://')) {
    const rest = href.slice('br://'.length);
    const segs = rest.split('/').filter(Boolean).map(decodeURIComponent);
    const uid = segs.shift() ?? '';
    if (!uid) return null;
    return { uid, path: segs.length ? segs : ['index.md'] };
  }
  const segs = href.split('/').filter(Boolean).map(decodeURIComponent);
  // A bare "/" (or empty) targets the host's index, matching bruig.
  return { uid: currentUid, path: segs.length ? segs : ['index.md'] };
};

// formatDcrFromAtoms renders an atom amount as a trimmed DCR string. BR
// shared-file / embed costs are in atoms (1 DCR = 1e8), distinct from the
// milli-atoms used for payment and tip records.
const formatDcrFromAtoms = (atoms: number): string =>
  (atoms / 1e8).toFixed(8).replace(/\.?0+$/, '');

const formatDownloadBytes = (n: number): string => {
  if (!n) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
};

// PageDownloadEmbed renders a file-transfer embed
// (--embed[download=<fid>,cost=,filename=,...]--): a file the host shares over
// BR file transfer, optionally behind a Lightning paywall. Clicking starts the
// download (the daemon auto-pays); for a paid file we require an explicit
// confirm first. While in flight we poll the downloads list for progress; on
// completion the bytes load from the dashboard's /content/file proxy - images
// render inline, other types as a download link.
const PageDownloadEmbed = ({ seg, uid }: { seg: BisonrelayPageSegment; uid: string }) => {
  const fid = seg.download || '';
  const filename = seg.filename || seg.name || 'file';
  const cost = seg.cost || 0;
  const isImage = !!seg.mime && seg.mime.startsWith('image/');
  const [phase, setPhase] = useState<'idle' | 'confirm' | 'downloading' | 'ready' | 'error'>('idle');
  const [err, setErr] = useState<string | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [usd, setUsd] = useState<{ amount: number; source: string; updatedAt: string } | null>(null);

  // For a paid file, look up the USD value of the cost (DCR/USD via BR, with a
  // Kraken fallback) so the price can be shown in both DCR and approximate USD.
  useEffect(() => {
    if (cost <= 0) return undefined;
    let cancelled = false;
    getBisonrelayRates()
      .then((r) => {
        if (!cancelled && r.dcr_usd > 0) {
          setUsd({ amount: (cost / 1e8) * r.dcr_usd, source: r.source, updatedAt: r.updated_at });
        }
      })
      .catch(() => {
        /* USD is best-effort; the DCR price always shows. */
      });
    return () => {
      cancelled = true;
    };
  }, [cost]);

  // costLabel renders "<n> DCR" plus an approximate USD value when known.
  const usdSuffix = usd
    ? ` (~$${usd.amount < 0.01 ? usd.amount.toFixed(4) : usd.amount.toFixed(2)})`
    : '';
  const costLabel = `${formatDcrFromAtoms(cost)} DCR${usdSuffix}`;
  const usdTitle = usd
    ? `USD via ${usd.source || 'unknown'}${usd.updatedAt ? `, updated ${new Date(usd.updatedAt).toLocaleString()}` : ''}`
    : undefined;

  useEffect(() => {
    if (phase !== 'downloading') return undefined;
    let cancelled = false;
    let timer: number | undefined;
    const tick = async () => {
      try {
        const items = await getBisonrelayManageDownloads();
        const it = items.find((d) => d.fid === fid);
        if (it) {
          setProgress({ done: Math.max(0, it.total_chunks - it.missing_chunks), total: it.total_chunks });
          if (it.missing_chunks === 0 && it.disk_path) {
            if (!cancelled) setPhase('ready');
            return;
          }
        }
      } catch {
        // Transient list error; keep polling.
      }
      if (!cancelled) timer = window.setTimeout(tick, 2000);
    };
    timer = window.setTimeout(tick, 800);
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [phase, fid]);

  const start = async () => {
    setErr(null);
    setPhase('downloading');
    try {
      await startBisonrelayContentGet(uid, fid);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not start download');
      setPhase('error');
    }
  };

  if (!fid) return null;

  if (phase === 'ready') {
    const url = bisonrelayContentFileUrl(fid, uid);
    if (isImage) {
      return (
        <img
          src={url}
          alt={seg.alt || filename}
          className="rounded-lg border border-border/40 max-w-full h-auto"
        />
      );
    }
    return (
      <a
        href={url}
        download={filename}
        className="inline-block text-xs text-primary underline hover:no-underline"
      >
        {filename} ({seg.mime || 'binary'})
      </a>
    );
  }

  const meta = [filename, seg.size ? formatDownloadBytes(seg.size) : '', cost > 0 ? costLabel : 'free']
    .filter(Boolean)
    .join(' · ');

  return (
    <div className="rounded-lg border border-border/50 bg-background/40 p-3 text-sm space-y-2">
      <div className="text-xs text-muted-foreground" title={usdTitle}>{meta}</div>
      {phase === 'confirm' ? (
        <div className="space-y-2">
          <div>
            Pay <span className="font-semibold text-foreground" title={usdTitle}>{costLabel}</span> to
            download <span className="font-mono">{filename}</span>?
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={start}
              className="px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
            >
              Pay &amp; download
            </button>
            <button
              type="button"
              onClick={() => setPhase('idle')}
              className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : phase === 'downloading' ? (
        <div className="text-muted-foreground">
          Downloading{progress && progress.total ? ` ${progress.done}/${progress.total} chunks` : '…'}
        </div>
      ) : phase === 'error' ? (
        <div className="space-y-2">
          <div className="text-xs text-rose-300">{err}</div>
          <button
            type="button"
            onClick={() => setPhase('idle')}
            className="px-3 py-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30 text-sm"
          >
            Try again
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => (cost > 0 ? setPhase('confirm') : start())}
          title={usdTitle}
          className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 transition-colors"
        >
          {cost > 0 ? `Download (${costLabel})` : `Download ${filename}`}
        </button>
      )}
    </div>
  );
};

const PageForm = ({
  fields,
  onSubmit,
}: {
  fields: BisonrelayPageFormField[];
  onSubmit: (
    actionPath: string[],
    data: Record<string, unknown>,
    asyncTargetId: string,
  ) => Promise<string | null>;
}) => {
  const initial = useMemo(() => {
    const m: Record<string, string> = {};
    for (const f of fields) {
      if (f.name) m[f.name] = f.value ?? '';
    }
    return m;
  }, [fields]);
  const [values, setValues] = useState<Record<string, string>>(initial);
  const [fieldErrs, setFieldErrs] = useState<Record<string, string>>({});
  const [submitErr, setSubmitErr] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const action = fields.find((f) => f.type === 'action')?.value ?? '';
  const asyncTarget = fields.find((f) => f.type === 'asynctarget')?.value ?? '';
  const submitField = fields.find((f) => f.type === 'submit');

  // validateField mirrors bruig's FormField validator (forms.dart): a field is
  // checked only when it carries a regexp, and the value must match it or the
  // field's regexpstr is shown. Authors mark a field required with regexp=".+".
  // A malformed author regexp is treated as no constraint rather than blocking.
  const validateField = (f: BisonrelayPageFormField, value: string): string | null => {
    if (!f.regexp) return null;
    try {
      return new RegExp(f.regexp).test(value) ? null : f.regexpstr || 'Invalid value';
    } catch {
      return null;
    }
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!action || submitting) return;
    const data: Record<string, unknown> = {};
    const errs: Record<string, string> = {};
    for (const f of fields) {
      if (!f.name) continue;
      const v = values[f.name] ?? f.value ?? '';
      data[f.name] = v;
      if (f.type === 'txtinput' || f.type === 'intinput') {
        const msg = validateField(f, v);
        if (msg) errs[f.name] = msg;
      }
    }
    if (Object.keys(errs).length > 0) {
      setFieldErrs(errs);
      return;
    }
    setFieldErrs({});
    const actionPath = action.split('/').filter(Boolean).map(decodeURIComponent);
    setSubmitErr(null);
    setSubmitting(true);
    const err = await onSubmit(actionPath, data, asyncTarget);
    setSubmitting(false);
    setSubmitErr(err);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3 rounded-lg border border-border/50 bg-background/40 p-4">
      {fields.map((f, i) => {
        if (f.type === 'txtinput' || f.type === 'intinput') {
          const fieldErr = f.name ? fieldErrs[f.name] : undefined;
          return (
            <label key={i} className="block">
              {f.label && <span className="text-xs text-muted-foreground">{f.label}</span>}
              <input
                type={f.type === 'intinput' ? 'number' : 'text'}
                value={f.name ? values[f.name] ?? '' : ''}
                placeholder={f.hint}
                aria-invalid={!!fieldErr}
                onChange={(e) => {
                  if (!f.name) return;
                  const v = e.target.value;
                  setValues((prev) => ({ ...prev, [f.name as string]: v }));
                  // Clear the field's error once the new value passes, so the
                  // message disappears as the user corrects it.
                  if (fieldErr && !validateField(f, v)) {
                    setFieldErrs((prev) => {
                      const next = { ...prev };
                      delete next[f.name as string];
                      return next;
                    });
                  }
                }}
                className={`mt-1 w-full rounded-md border bg-background px-3 py-1.5 text-sm ${
                  fieldErr ? 'border-rose-500/60' : 'border-border'
                }`}
              />
              {fieldErr && <span className="mt-1 block text-xs text-rose-300">{fieldErr}</span>}
            </label>
          );
        }
        return null;
      })}
      <button
        type="submit"
        disabled={!action || submitting}
        className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50 transition-colors"
      >
        {submitting ? 'Submitting…' : submitField?.label || 'Submit'}
      </button>
      {submitErr && <div className="text-xs text-rose-300">{submitErr}</div>}
    </form>
  );
};
