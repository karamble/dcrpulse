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
  deleteBisonrelayLocalPage,
  fetchBisonrelayPage,
  getBisonrelayIdentity,
  getBisonrelayLocalPage,
  listBisonrelayLocalPages,
  saveBisonrelayLocalPage,
} from '../../services/bisonrelayApi';
import { BisonrelayEditor, composeBRBody, EditorEmbedMap } from './editor';
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
    <div className="flex gap-4">
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
    <aside className="w-44 shrink-0 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-2 self-start">
      <nav className="flex flex-col gap-1">
        {items.map((item) => {
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
    if (!/^[A-Za-z0-9_.-]+\.md$/.test(finalName) || finalName.includes('..')) {
      setErr('Name must be a single .md file (letters, digits, dash, underscore, dot).');
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
    async (actionPath: string[], formData: Record<string, unknown>, asyncTargetId: string) => {
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
        if (asyncTargetId) {
          applyAsync(asyncTargetId, res.segments ?? []);
        } else {
          setPage(res);
          setSegments(res.segments ?? []);
        }
      } catch (e: any) {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Form submit failed');
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
  onSubmitForm: (actionPath: string[], data: Record<string, unknown>, asyncTargetId: string) => void;
}) => {
  // Intercept clicks on br:// and relative links so they navigate in-app
  // rather than reloading the dashboard. Fully-qualified http(s) links keep
  // their default (open in a new tab via the renderer's target=_blank).
  const onClick = (e: MouseEvent<HTMLDivElement>) => {
    const anchor = (e.target as HTMLElement).closest('a');
    if (!anchor) return;
    const href = anchor.getAttribute('href') || '';
    if (/^https?:\/\//i.test(href) || href.startsWith('mailto:')) return;
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
  if (segs.length === 0) return null;
  return { uid: currentUid, path: segs };
};

const PageForm = ({
  fields,
  onSubmit,
}: {
  fields: BisonrelayPageFormField[];
  onSubmit: (actionPath: string[], data: Record<string, unknown>, asyncTargetId: string) => void;
}) => {
  const initial = useMemo(() => {
    const m: Record<string, string> = {};
    for (const f of fields) {
      if (f.name) m[f.name] = f.value ?? '';
    }
    return m;
  }, [fields]);
  const [values, setValues] = useState<Record<string, string>>(initial);

  const action = fields.find((f) => f.type === 'action')?.value ?? '';
  const asyncTarget = fields.find((f) => f.type === 'asynctarget')?.value ?? '';
  const submitField = fields.find((f) => f.type === 'submit');

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!action) return;
    const data: Record<string, unknown> = {};
    for (const f of fields) {
      if (!f.name) continue;
      data[f.name] = values[f.name] ?? f.value ?? '';
    }
    const actionPath = action.split('/').filter(Boolean).map(decodeURIComponent);
    onSubmit(actionPath, data, asyncTarget);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3 rounded-lg border border-border/50 bg-background/40 p-4">
      {fields.map((f, i) => {
        if (f.type === 'txtinput' || f.type === 'intinput') {
          return (
            <label key={i} className="block">
              {f.label && <span className="text-xs text-muted-foreground">{f.label}</span>}
              <input
                type={f.type === 'intinput' ? 'number' : 'text'}
                value={f.name ? values[f.name] ?? '' : ''}
                placeholder={f.hint}
                onChange={(e) =>
                  f.name && setValues((v) => ({ ...v, [f.name as string]: e.target.value }))
                }
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
              />
            </label>
          );
        }
        return null;
      })}
      <button
        type="submit"
        disabled={!action}
        className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50 transition-colors"
      >
        {submitField?.label || 'Submit'}
      </button>
    </form>
  );
};
