// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Maps a Theme to the CSS custom properties on :root. This is the single
// source of truth for theme application, shared by the runtime provider and
// the editor's live preview. The var names MUST match src/index.css and the
// inline first-paint script in index.html.

import { Theme } from './types';

export function themeToCssVars(theme: Theme): Record<string, string> {
  const c = theme.colors;
  const g = theme.gradients ?? {};
  const t = theme.typography ?? {};
  return {
    '--background': c.background,
    '--foreground': c.foreground,
    '--card': c.card,
    '--card-foreground': c.cardForeground,
    '--primary': c.primary,
    '--primary-foreground': c.primaryForeground,
    '--secondary': c.secondary,
    '--secondary-foreground': c.secondaryForeground,
    '--success': c.success,
    '--success-foreground': c.successForeground,
    '--destructive': c.destructive,
    '--destructive-foreground': c.destructiveForeground,
    '--warning': c.warning,
    '--warning-foreground': c.warningForeground,
    '--muted': c.muted,
    '--muted-foreground': c.mutedForeground,
    '--border': c.border,
    // Gradients derive from the accent / card colors unless overridden.
    '--gradient-primary-from': g.primaryFrom ?? c.primary,
    '--gradient-primary-to': g.primaryTo ?? c.secondary,
    '--gradient-card-from': g.cardFrom ?? c.card,
    '--gradient-card-to': g.cardTo ?? c.card,
    '--heading-color': t.headingColor ?? c.foreground,
    '--heading-weight': String(t.headingWeight ?? 700),
    '--heading-1-size': t.heading1Size ?? '1.25rem',
    '--heading-2-size': t.heading2Size ?? '1.125rem',
    '--heading-3-size': t.heading3Size ?? '1rem',
    '--radius': theme.radius ?? '0.75rem',
  };
}

export function applyTheme(
  theme: Theme,
  el: HTMLElement = document.documentElement,
): void {
  const vars = themeToCssVars(theme);
  for (const [name, value] of Object.entries(vars)) {
    el.style.setProperty(name, value);
  }
  el.style.colorScheme = theme.appearance;
  el.setAttribute('data-theme', theme.id);
}
