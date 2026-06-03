// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useRef, useState } from 'react';
import { AlertCircle, Check, CheckCircle2, Copy, Download } from 'lucide-react';
import { Theme } from '../../../services/themes/types';
import { hslChannelsToHex } from '../../../services/themes/themeMath';
import { useTheme } from '../../../services/themes/ThemeProvider';
import { ThemeEditor } from './ThemeEditor';

const SWATCH_KEYS: (keyof Theme['colors'])[] = [
  'background',
  'card',
  'primary',
  'secondary',
  'success',
  'destructive',
];

export const ThemesSection = () => {
  const {
    activeThemeId,
    builtins,
    allThemes,
    setActiveTheme,
    deleteCustomTheme,
    importTheme,
  } = useTheme();

  const [editing, setEditing] = useState<Theme | null>(null);
  const [importText, setImportText] = useState('');
  const [showImport, setShowImport] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'info' | 'error'; text: string } | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const flash = (kind: 'info' | 'error', text: string) => {
    setFeedback({ kind, text });
    if (kind === 'info') window.setTimeout(() => setFeedback(null), 4000);
  };

  const draftFrom = (t: Theme, mode: 'edit' | 'copy'): Theme => {
    if (mode === 'edit' && !t.builtin) return t; // edit a custom theme in place
    return { ...t, id: '', builtin: false, name: `${t.name} copy` };
  };

  const onSaved = async (saved: Theme) => {
    try {
      await setActiveTheme(saved.id);
    } catch {
      /* applying is best-effort; the theme is saved regardless */
    }
    setEditing(null);
    flash('info', `Saved and applied "${saved.name}".`);
  };

  const handleApply = async (id: string) => {
    try {
      await setActiveTheme(id);
    } catch (err) {
      flash('error', err instanceof Error ? err.message : 'Failed to apply theme');
    }
  };

  const handleDelete = async (t: Theme) => {
    if (!window.confirm(`Delete theme "${t.name}"? This cannot be undone.`)) return;
    try {
      await deleteCustomTheme(t.id);
      flash('info', `Deleted "${t.name}".`);
    } catch (err) {
      flash('error', err instanceof Error ? err.message : 'Failed to delete theme');
    }
  };

  const handleExport = (t: Theme) => {
    const blob = new Blob([JSON.stringify(t, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `dcrpulse-theme-${t.id || t.name.toLowerCase().replace(/\s+/g, '-')}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const doImport = async (json: string) => {
    try {
      const saved = await importTheme(json);
      setImportText('');
      setShowImport(false);
      flash('info', `Imported "${saved.name}".`);
    } catch (err) {
      flash('error', err instanceof Error ? err.message : 'Import failed');
    }
  };

  const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    doImport(await f.text());
  };

  if (editing) {
    return (
      <div className="max-w-2xl space-y-4 rounded-xl border border-border/50 bg-gradient-card p-6 backdrop-blur-sm">
        <h3 className="text-lg font-semibold">
          {editing.id ? 'Edit theme' : 'New theme'}
        </h3>
        <ThemeEditor initial={editing} onSaved={onSaved} onCancel={() => setEditing(null)} />
      </div>
    );
  }

  return (
    <div className="max-w-3xl space-y-4 rounded-xl border border-border/50 bg-gradient-card p-6 backdrop-blur-sm">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-lg font-semibold">Themes</h3>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setShowImport((v) => !v)}
            className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted/30"
          >
            Import
          </button>
          <button
            type="button"
            onClick={() => setEditing(draftFrom(builtins[0], 'copy'))}
            className="rounded-lg bg-gradient-primary px-3 py-1.5 text-xs font-semibold text-white"
          >
            New theme
          </button>
        </div>
      </div>

      <p className="text-sm text-muted-foreground">
        Pick a theme to apply it everywhere, or edit one to build your own. Custom themes
        are saved on this instance and follow you across browsers.
      </p>

      {showImport && (
        <div className="space-y-2 rounded-lg border border-border/50 bg-muted/10 p-3">
          <textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder="Paste theme JSON here…"
            rows={4}
            className="w-full rounded border border-border bg-background px-2 py-1 font-mono text-xs focus:border-primary focus:outline-none"
          />
          <div className="flex gap-2">
            <button
              type="button"
              disabled={!importText.trim()}
              onClick={() => doImport(importText)}
              className="rounded-lg bg-primary/20 px-3 py-1.5 text-xs font-medium text-primary disabled:opacity-50"
            >
              Import from text
            </button>
            <button
              type="button"
              onClick={() => fileRef.current?.click()}
              className="rounded-lg bg-muted/20 px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-muted/30"
            >
              Import from file…
            </button>
            <input ref={fileRef} type="file" accept="application/json,.json" className="hidden" onChange={handleImportFile} />
          </div>
        </div>
      )}

      {feedback && (
        <div
          className={`flex items-center gap-2 text-sm ${
            feedback.kind === 'error' ? 'text-destructive' : 'text-success'
          }`}
        >
          {feedback.kind === 'error' ? <AlertCircle className="h-4 w-4" /> : <CheckCircle2 className="h-4 w-4" />}
          {feedback.text}
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        {allThemes.map((t) => {
          const isActive = t.id === activeThemeId;
          return (
            <div
              key={t.id}
              className={`rounded-lg border p-3 transition-colors ${
                isActive ? 'border-primary ring-1 ring-primary' : 'border-border/50'
              }`}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{t.name}</span>
                <span className="rounded bg-muted/30 px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">
                  {t.builtin ? 'Built-in' : 'Custom'} · {t.appearance}
                </span>
              </div>

              <div className="mt-2 flex gap-1">
                {SWATCH_KEYS.map((k) => (
                  <span
                    key={k}
                    title={k}
                    className="h-5 flex-1 rounded"
                    style={{ backgroundColor: hslChannelsToHex(t.colors[k]) }}
                  />
                ))}
              </div>

              <div className="mt-3 flex flex-wrap items-center gap-1.5 text-xs">
                <button
                  type="button"
                  onClick={() => handleApply(t.id)}
                  disabled={isActive}
                  className={`inline-flex items-center gap-1 rounded px-2 py-1 font-medium ${
                    isActive
                      ? 'bg-success/15 text-success'
                      : 'bg-primary/15 text-primary hover:bg-primary/25'
                  }`}
                >
                  {isActive ? <><Check className="h-3 w-3" /> Active</> : 'Apply'}
                </button>
                <button
                  type="button"
                  onClick={() => setEditing(draftFrom(t, 'edit'))}
                  className="rounded px-2 py-1 text-muted-foreground hover:bg-muted/20 hover:text-foreground"
                >
                  {t.builtin ? 'Customize' : 'Edit'}
                </button>
                <button
                  type="button"
                  onClick={() => setEditing(draftFrom(t, 'copy'))}
                  className="inline-flex items-center gap-1 rounded px-2 py-1 text-muted-foreground hover:bg-muted/20 hover:text-foreground"
                >
                  <Copy className="h-3 w-3" /> Duplicate
                </button>
                <button
                  type="button"
                  onClick={() => handleExport(t)}
                  className="inline-flex items-center gap-1 rounded px-2 py-1 text-muted-foreground hover:bg-muted/20 hover:text-foreground"
                >
                  <Download className="h-3 w-3" /> Export
                </button>
                {!t.builtin && (
                  <button
                    type="button"
                    onClick={() => handleDelete(t)}
                    className="rounded px-2 py-1 text-destructive hover:bg-destructive/15"
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
