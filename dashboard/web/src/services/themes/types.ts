// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Bumped only on a breaking change to the Theme shape. Import rejects a
// mismatched schema so a future format never loads silently wrong.
export const THEME_SCHEMA_VERSION = 1;

// Canonical storage form for a color: bare HSL channels "H S% L%"
// (e.g. "217 91% 60%"), matching the CSS variables Tailwind consumes via
// hsl(var(--token) / <alpha-value>).
export type HslChannels = string;

export interface ThemeColors {
  background: HslChannels;
  foreground: HslChannels;
  card: HslChannels;
  cardForeground: HslChannels;
  primary: HslChannels;
  primaryForeground: HslChannels;
  secondary: HslChannels;
  secondaryForeground: HslChannels;
  success: HslChannels;
  successForeground: HslChannels;
  destructive: HslChannels;
  destructiveForeground: HslChannels;
  warning: HslChannels;
  warningForeground: HslChannels;
  muted: HslChannels;
  mutedForeground: HslChannels;
  border: HslChannels;
}

// Optional gradient endpoint overrides. When omitted the provider derives
// them from the accent (primary/secondary) and card colors.
export interface ThemeGradients {
  primaryFrom?: HslChannels;
  primaryTo?: HslChannels;
  cardFrom?: HslChannels;
  cardTo?: HslChannels;
}

export interface ThemeTypography {
  headingColor?: HslChannels; // default = colors.foreground
  headingWeight?: number; // 400..800, default 700
  heading1Size?: string; // e.g. "1.25rem"
  heading2Size?: string;
  heading3Size?: string;
}

export interface Theme {
  schema: number;
  id: string;
  name: string;
  builtin: boolean;
  appearance: 'dark' | 'light';
  colors: ThemeColors;
  gradients?: ThemeGradients;
  typography?: ThemeTypography;
  radius?: string;
}

// Persisted document (server global config). Shipped themes live in the
// bundle, so only the active selection and custom themes are stored.
export interface ThemeStore {
  schema: number;
  activeThemeId: string;
  customThemes: Theme[];
}

// The color token keys in render order, used by both the editor and the
// apply function. Keyof ThemeColors keeps this in sync with the type.
export const COLOR_KEYS: (keyof ThemeColors)[] = [
  'background',
  'foreground',
  'card',
  'cardForeground',
  'primary',
  'primaryForeground',
  'secondary',
  'secondaryForeground',
  'success',
  'successForeground',
  'destructive',
  'destructiveForeground',
  'warning',
  'warningForeground',
  'muted',
  'mutedForeground',
  'border',
];
