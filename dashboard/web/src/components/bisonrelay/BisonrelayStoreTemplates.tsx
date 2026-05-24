// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { ExternalLink, FileCode, Plus, Trash2 } from 'lucide-react';
import {
  BisonrelayStoreTemplate,
  deleteBisonrelayStoreTemplate,
  getBisonrelayIdentity,
  getBisonrelayStoreTemplates,
  getBisonrelayStoreTemplateFile,
  saveBisonrelayStoreTemplate,
} from '../../services/bisonrelayApi';

// toHex converts the base64 own-identity to the hex uid the Pages viewer uses.
const toHex = (b64: string): string => {
  try {
    const bin = atob(b64);
    let h = '';
    for (let i = 0; i < bin.length; i += 1) h += bin.charCodeAt(i).toString(16).padStart(2, '0');
    return h;
  } catch {
    return '';
  }
};

// BisonrelayStoreTemplates is a plain code editor for the storefront's Go
// templates (*.tmpl). Saving writes the file; the store live-reloads, so the
// change is visible by opening the storefront in the Pages viewer. A bad
// template is skipped by the store (it keeps the previous one).
export const BisonrelayStoreTemplates = () => {
  const [list, setList] = useState<BisonrelayStoreTemplate[] | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState('');
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [ownHex, setOwnHex] = useState('');

  const loadList = () => {
    getBisonrelayStoreTemplates()
      .then(setList)
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Could not load templates'));
  };
  useEffect(loadList, []);
  useEffect(() => {
    getBisonrelayIdentity()
      .then((id) => setOwnHex(toHex(id.identity ?? '')))
      .catch(() => {});
  }, []);

  const openTemplate = async (name: string) => {
    setErr(null);
    try {
      const text = await getBisonrelayStoreTemplateFile(name);
      setSelected(name);
      setContent(text);
      setDirty(false);
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Could not open template');
    }
  };

  const newTemplate = () => {
    const name = window.prompt('New template name (e.g. header.tmpl):');
    if (!name) return;
    setSelected(name.trim());
    setContent('');
    setDirty(true);
  };

  const save = async () => {
    if (!selected) return;
    setSaving(true);
    setErr(null);
    try {
      await saveBisonrelayStoreTemplate(selected, content);
      setDirty(false);
      loadList();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const remove = async (name: string) => {
    setErr(null);
    try {
      await deleteBisonrelayStoreTemplate(name);
      if (selected === name) {
        setSelected(null);
        setContent('');
      }
      loadList();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Delete failed');
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Templates</h2>
          <p className="text-xs text-muted-foreground">
            Go templates the storefront renders from. Changes apply live; a syntax error keeps the
            previous version.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {ownHex && (
            <a
              href={`#pages/visit/${ownHex}/index.md`}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30"
              title="Open your storefront in the Pages viewer (renders the templates)"
            >
              <ExternalLink className="h-4 w-4" />
              View storefront
            </a>
          )}
          <button
            type="button"
            onClick={newTemplate}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30"
          >
            <Plus className="h-4 w-4" />
            New template
          </button>
        </div>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      <div className="flex gap-4">
        <ul className="w-52 shrink-0 rounded-xl border border-border/50 bg-gradient-card divide-y divide-border/40 self-start">
          {(list ?? []).map((t) => (
            <li key={t.name} className="flex items-center gap-2 px-3 py-2">
              <button
                type="button"
                onClick={() => openTemplate(t.name)}
                className={`flex-1 min-w-0 flex items-center gap-2 text-left text-xs ${
                  selected === t.name ? 'text-primary' : 'text-foreground hover:text-primary'
                }`}
              >
                <FileCode className="h-3.5 w-3.5 shrink-0" />
                <span className="truncate font-mono">{t.name}</span>
              </button>
              <button
                type="button"
                onClick={() => remove(t.name)}
                className="text-rose-400 hover:text-rose-300"
                title="Delete"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </li>
          ))}
          {list && list.length === 0 && (
            <li className="px-3 py-2 text-xs text-muted-foreground">No templates.</li>
          )}
        </ul>

        <div className="flex-1 min-w-0">
          {selected ? (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="font-mono text-sm">{selected}</span>
                <button
                  type="button"
                  onClick={save}
                  disabled={saving || !dirty}
                  className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold hover:bg-primary/30 disabled:opacity-50"
                >
                  {saving ? 'Saving…' : dirty ? 'Save' : 'Saved'}
                </button>
              </div>
              <textarea
                value={content}
                onChange={(e) => {
                  setContent(e.target.value);
                  setDirty(true);
                }}
                spellCheck={false}
                rows={22}
                className="w-full rounded-lg border border-border/50 bg-background px-3 py-2 font-mono text-xs leading-relaxed"
              />
            </div>
          ) : (
            <div className="rounded-xl border border-border/50 bg-gradient-card p-6 text-sm text-muted-foreground">
              Select a template to edit, or create one (e.g. <span className="font-mono">header.tmpl</span>{' '}
              as a partial included with <span className="font-mono">{'{{ template "header.tmpl" . }}'}</span>).
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
