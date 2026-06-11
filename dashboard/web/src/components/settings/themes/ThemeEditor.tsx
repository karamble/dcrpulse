// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { AlertCircle, Check, RotateCcw } from 'lucide-react';
import { Theme, ThemeColors } from '../../../services/themes/types';
import { contrastRatio } from '../../../services/themes/themeMath';
import { useTheme } from '../../../services/themes/ThemeProvider';
import { ColorField } from './ColorField';

interface ThemeEditorProps {
  initial: Theme;
  onSaved: (theme: Theme) => void;
  onCancel: () => void;
}

const GROUPS: { title: string; keys: (keyof ThemeColors)[] }[] = [
  { title: 'Brand & accent', keys: ['primary', 'primaryForeground', 'secondary', 'secondaryForeground'] },
  { title: 'Backgrounds & surfaces', keys: ['background', 'card', 'cardForeground', 'muted'] },
  { title: 'Text', keys: ['foreground', 'mutedForeground'] },
  { title: 'Status', keys: ['success', 'successForeground', 'destructive', 'destructiveForeground', 'warning', 'warningForeground'] },
  { title: 'Borders', keys: ['border'] },
];

const LABELS: Record<keyof ThemeColors, string> = {
  background: 'Background',
  foreground: 'Foreground (body text)',
  card: 'Card surface',
  cardForeground: 'Card text',
  primary: 'Primary / accent',
  primaryForeground: 'On primary',
  secondary: 'Secondary accent',
  secondaryForeground: 'On secondary',
  success: 'Success',
  successForeground: 'On success',
  destructive: 'Destructive / error',
  destructiveForeground: 'On destructive',
  warning: 'Warning',
  warningForeground: 'On warning',
  muted: 'Muted surface',
  mutedForeground: 'Muted text',
  border: 'Border',
};

// Warn when a text/background pair fails the WCAG AA body-text ratio.
function contrastWarn(fg: string, bg: string): string | null {
  return contrastRatio(fg, bg) < 4.5 ? 'Low contrast (below WCAG AA)' : null;
}

const HEADING_WEIGHTS = [400, 500, 600, 700, 800];

export const ThemeEditor = ({ initial, onSaved, onCancel }: ThemeEditorProps) => {
  const { preview, saveCustomTheme } = useTheme();
  const [draft, setDraft] = useState<Theme>(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Live-apply the draft to the whole app as it changes; restore the real
  // active theme when the editor closes. previewRef avoids re-running on
  // preview identity churn.
  const previewRef = useRef(preview);
  previewRef.current = preview;
  useEffect(() => {
    previewRef.current(draft);
  }, [draft]);
  useEffect(() => () => previewRef.current(null), []);

  const setColor = (key: keyof ThemeColors, channels: string) =>
    setDraft((d) => ({ ...d, colors: { ...d.colors, [key]: channels } }));

  const c = draft.colors;
  const typo = draft.typography ?? {};
  const setTypo = (patch: Partial<NonNullable<Theme['typography']>>) =>
    setDraft((d) => ({ ...d, typography: { ...d.typography, ...patch } }));

  const gradientsOn = !!draft.gradients;
  const toggleGradients = (on: boolean) =>
    setDraft((d) => ({
      ...d,
      gradients: on
        ? {
            primaryFrom: d.colors.primary,
            primaryTo: d.colors.secondary,
            cardFrom: d.colors.card,
            cardTo: d.colors.card,
          }
        : undefined,
    }));
  const setGradient = (key: keyof NonNullable<Theme['gradients']>, channels: string) =>
    setDraft((d) => ({ ...d, gradients: { ...d.gradients, [key]: channels } }));

  const warnFor = (key: keyof ThemeColors): string | null => {
    switch (key) {
      case 'foreground':
        return contrastWarn(c.foreground, c.background);
      case 'mutedForeground':
        return contrastWarn(c.mutedForeground, c.background);
      case 'cardForeground':
        return contrastWarn(c.cardForeground, c.card);
      case 'primaryForeground':
        return contrastWarn(c.primaryForeground, c.primary);
      case 'secondaryForeground':
        return contrastWarn(c.secondaryForeground, c.secondary);
      default:
        return null;
    }
  };

  const handleSave = async () => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      const saved = await saveCustomTheme({ ...draft, name: draft.name.trim() || 'My theme' });
      onSaved(saved);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save theme');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-5">
      <div>
        <label className="mb-1 block text-xs text-muted-foreground" htmlFor="theme-name">
          Theme name
        </label>
        <input
          id="theme-name"
          type="text"
          value={draft.name}
          maxLength={60}
          onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
          className="w-full max-w-sm rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none"
        />
        <p className="mt-1 text-xs text-muted-foreground">
          Changes preview live across the app. Save to keep this theme; it is stored on this instance.
        </p>
      </div>

      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Base appearance</label>
        <div className="flex gap-2">
          {(['dark', 'light'] as const).map((mode) => (
            <button
              key={mode}
              type="button"
              onClick={() => setDraft((d) => ({ ...d, appearance: mode }))}
              className={`rounded-lg px-3 py-1.5 text-xs font-medium capitalize transition-colors ${
                draft.appearance === mode
                  ? 'bg-primary/20 text-primary'
                  : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
              }`}
            >
              {mode}
            </button>
          ))}
        </div>
      </div>

      {GROUPS.map((group) => (
        <div key={group.title}>
          <h4 className="mb-1 text-sm font-semibold">{group.title}</h4>
          <div className="rounded-lg border border-border/50 bg-muted/5 px-3 py-1">
            {group.keys.map((key) => (
              <ColorField
                key={key}
                label={LABELS[key]}
                value={c[key]}
                onChange={(v) => setColor(key, v)}
                warn={warnFor(key)}
              />
            ))}
          </div>
          {group.title === 'Text' && (
            <div className="mt-2 rounded-lg border border-border/50 bg-muted/5 px-3 py-2 space-y-2">
              <ColorField
                label="Heading color"
                value={typo.headingColor ?? c.foreground}
                onChange={(v) => setTypo({ headingColor: v })}
              />
              <div className="flex items-center gap-3">
                <span className="flex-1 text-sm">Heading weight</span>
                <select
                  value={typo.headingWeight ?? 700}
                  onChange={(e) => setTypo({ headingWeight: Number(e.target.value) })}
                  className="rounded border border-border bg-background px-2 py-1 text-xs"
                >
                  {HEADING_WEIGHTS.map((w) => (
                    <option key={w} value={w}>
                      {w}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid grid-cols-3 gap-2">
                {(['heading1Size', 'heading2Size', 'heading3Size'] as const).map((k, i) => (
                  <label key={k} className="text-xs text-muted-foreground">
                    H{i + 1} size
                    <input
                      type="text"
                      value={typo[k] ?? ['1.25rem', '1.125rem', '1rem'][i]}
                      onChange={(e) => setTypo({ [k]: e.target.value })}
                      className="mt-0.5 w-full rounded border border-border bg-background px-2 py-1 font-mono text-xs"
                    />
                  </label>
                ))}
              </div>
              <p className="text-[11px] text-muted-foreground">
                Heading color applies to all headings. Weight and size apply to rendered
                content headings (proposals, pages).
              </p>
            </div>
          )}
        </div>
      ))}

      <details className="rounded-lg border border-border/50 bg-muted/5 px-3 py-2">
        <summary className="cursor-pointer select-none text-sm font-semibold">
          Advanced
        </summary>
        <div className="mt-2 space-y-3">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={gradientsOn}
              onChange={(e) => toggleGradients(e.target.checked)}
            />
            Customize gradients (otherwise derived from accent and card colors)
          </label>
          {gradientsOn && draft.gradients && (
            <div className="rounded-lg border border-border/50 px-3 py-1">
              <ColorField label="Primary gradient from" value={draft.gradients.primaryFrom ?? c.primary} onChange={(v) => setGradient('primaryFrom', v)} />
              <ColorField label="Primary gradient to" value={draft.gradients.primaryTo ?? c.secondary} onChange={(v) => setGradient('primaryTo', v)} />
              <ColorField label="Card gradient from" value={draft.gradients.cardFrom ?? c.card} onChange={(v) => setGradient('cardFrom', v)} />
              <ColorField label="Card gradient to" value={draft.gradients.cardTo ?? c.card} onChange={(v) => setGradient('cardTo', v)} />
            </div>
          )}
          <div className="flex items-center gap-3">
            <span className="flex-1 text-sm">Corner radius (reserved)</span>
            <input
              type="text"
              value={draft.radius ?? '0.75rem'}
              onChange={(e) => setDraft((d) => ({ ...d, radius: e.target.value }))}
              className="w-24 rounded border border-border bg-background px-2 py-1 font-mono text-xs"
            />
          </div>
        </div>
      </details>

      {error && (
        <div className="flex items-center gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4" /> {error}
        </div>
      )}

      <div className="sticky bottom-0 -mx-6 flex justify-end gap-2 border-t border-border/50 bg-card/80 px-6 py-3 backdrop-blur-sm">
        <button
          type="button"
          onClick={onCancel}
          disabled={busy}
          className="inline-flex items-center gap-2 rounded-lg bg-muted/20 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted/30 disabled:opacity-50"
        >
          <RotateCcw className="h-4 w-4" /> Discard
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={busy}
          className="inline-flex items-center gap-2 rounded-lg bg-gradient-primary px-4 py-2 text-sm font-semibold text-white transition-all disabled:opacity-50"
        >
          <Check className="h-4 w-4" /> {busy ? 'Saving…' : 'Save theme'}
        </button>
      </div>
    </div>
  );
};
