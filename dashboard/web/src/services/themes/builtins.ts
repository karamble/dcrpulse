// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Shipped themes. These live in the bundle (not the server) and are
// read-only in the editor; editing one duplicates it into a custom theme.
// Pulse is a 1:1 capture of the original hardcoded palette.

import { Theme } from './types';

export const PULSE_THEME_ID = 'pulse';

const pulse: Theme = {
  schema: 1,
  id: PULSE_THEME_ID,
  name: 'Pulse',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '222 47% 5%',
    foreground: '210 40% 98%',
    card: '222 47% 8%',
    cardForeground: '210 40% 98%',
    primary: '217 91% 60%',
    primaryForeground: '222 47% 5%',
    secondary: '173 80% 40%',
    secondaryForeground: '210 40% 98%',
    success: '142 76% 36%',
    successForeground: '210 40% 98%',
    destructive: '0 72% 51%',
    destructiveForeground: '210 40% 98%',
    warning: '38 92% 50%',
    warningForeground: '222 47% 5%',
    muted: '217 32% 17%',
    mutedForeground: '215 20% 65%',
    border: '217 32% 17%',
  },
  gradients: {
    primaryFrom: '217 91% 60%',
    primaryTo: '173 80% 40%',
    cardFrom: '222 47% 8%',
    cardTo: '222 47% 10%',
  },
};

const daybreak: Theme = {
  schema: 1,
  id: 'daybreak',
  name: 'Daybreak',
  builtin: true,
  appearance: 'light',
  colors: {
    background: '210 40% 98%',
    foreground: '222 47% 11%',
    card: '0 0% 100%',
    cardForeground: '222 47% 11%',
    primary: '217 84% 48%',
    primaryForeground: '0 0% 100%',
    secondary: '173 70% 34%',
    secondaryForeground: '0 0% 100%',
    success: '142 64% 32%',
    successForeground: '0 0% 100%',
    destructive: '0 72% 47%',
    destructiveForeground: '0 0% 100%',
    warning: '30 90% 40%',
    warningForeground: '0 0% 100%',
    muted: '214 32% 91%',
    mutedForeground: '215 16% 38%',
    border: '214 25% 84%',
  },
  gradients: {
    cardFrom: '0 0% 100%',
    cardTo: '210 40% 97%',
  },
};

const aurora: Theme = {
  schema: 1,
  id: 'aurora',
  name: 'Aurora',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '250 30% 7%',
    foreground: '250 25% 96%',
    card: '250 30% 10%',
    cardForeground: '250 25% 96%',
    primary: '262 83% 64%',
    primaryForeground: '250 40% 8%',
    secondary: '295 72% 58%',
    secondaryForeground: '250 25% 96%',
    success: '142 70% 42%',
    successForeground: '250 25% 96%',
    destructive: '0 72% 56%',
    destructiveForeground: '250 25% 96%',
    warning: '38 92% 55%',
    warningForeground: '250 40% 8%',
    muted: '255 22% 18%',
    mutedForeground: '255 15% 68%',
    border: '255 22% 20%',
  },
};

const ember: Theme = {
  schema: 1,
  id: 'ember',
  name: 'Ember',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '20 16% 7%',
    foreground: '30 25% 96%',
    card: '20 16% 10%',
    cardForeground: '30 25% 96%',
    primary: '18 90% 56%',
    primaryForeground: '20 30% 8%',
    secondary: '40 95% 55%',
    secondaryForeground: '20 30% 8%',
    success: '142 65% 42%',
    successForeground: '30 25% 96%',
    destructive: '0 75% 56%',
    destructiveForeground: '30 25% 96%',
    warning: '38 95% 55%',
    warningForeground: '20 30% 8%',
    muted: '22 16% 18%',
    mutedForeground: '28 14% 68%',
    border: '22 16% 20%',
  },
};

// Terminal / "Matrix" look: green-tinted black with a bright phosphor-green
// accent. The accent (--primary) stays bright for glowing text/links/borders,
// while the gradient endpoints are darker greens so the white button text on
// bg-gradient-primary stays legible.
const matrix: Theme = {
  schema: 1,
  id: 'matrix',
  name: 'Matrix',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '135 30% 4%',
    foreground: '130 90% 64%',
    card: '135 26% 7%',
    cardForeground: '130 90% 64%',
    primary: '135 100% 50%',
    primaryForeground: '135 60% 5%',
    secondary: '95 85% 55%',
    secondaryForeground: '135 60% 5%',
    success: '140 90% 45%',
    successForeground: '135 60% 5%',
    destructive: '0 90% 62%',
    destructiveForeground: '0 0% 100%',
    warning: '50 95% 55%',
    warningForeground: '50 60% 8%',
    muted: '135 18% 13%',
    mutedForeground: '130 40% 58%',
    border: '135 45% 20%',
  },
  gradients: {
    primaryFrom: '140 85% 28%',
    primaryTo: '160 85% 26%',
    cardFrom: '135 26% 7%',
    cardTo: '135 30% 10%',
  },
};

// Deep midnight blue: a darker, cooler, indigo-accented dark theme. Gradient
// endpoints are a touch darker than the accent so white button text reads.
const midnight: Theme = {
  schema: 1,
  id: 'midnight',
  name: 'Midnight',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '225 45% 6%',
    foreground: '220 30% 92%',
    card: '226 42% 9%',
    cardForeground: '220 30% 92%',
    primary: '234 84% 68%',
    primaryForeground: '225 45% 6%',
    secondary: '205 80% 62%',
    secondaryForeground: '225 45% 6%',
    success: '150 60% 45%',
    successForeground: '220 30% 95%',
    destructive: '350 78% 62%',
    destructiveForeground: '0 0% 100%',
    warning: '40 92% 58%',
    warningForeground: '225 45% 8%',
    muted: '226 30% 16%',
    mutedForeground: '220 22% 64%',
    border: '226 30% 19%',
  },
  gradients: {
    primaryFrom: '234 70% 52%',
    primaryTo: '210 78% 50%',
    cardFrom: '226 42% 9%',
    cardTo: '230 44% 12%',
  },
};

// Jet black: a near-pure-black, monochrome look modeled on the Decred website.
// The primary token stays near-white for links/text/borders, while the gradient
// endpoints are mid-grey so the hardcoded white text on bg-gradient-primary
// buttons stays legible.
const jetBlack: Theme = {
  schema: 1,
  id: 'jet-black',
  name: 'Jet Black',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '0 0% 3%',
    foreground: '0 0% 96%',
    card: '0 0% 6%',
    cardForeground: '0 0% 96%',
    primary: '0 0% 90%',
    primaryForeground: '0 0% 8%',
    secondary: '0 0% 72%',
    secondaryForeground: '0 0% 8%',
    success: '142 60% 45%',
    successForeground: '0 0% 100%',
    destructive: '0 72% 55%',
    destructiveForeground: '0 0% 100%',
    warning: '40 92% 55%',
    warningForeground: '0 0% 8%',
    muted: '0 0% 13%',
    mutedForeground: '0 0% 60%',
    border: '0 0% 16%',
  },
  gradients: {
    primaryFrom: '0 0% 26%',
    primaryTo: '0 0% 38%',
    cardFrom: '0 0% 6%',
    cardTo: '0 0% 9%',
  },
};

// Redshift: a warm, low-blue "night shift" palette shifted toward deep red and
// amber. Gradient endpoints stay a dark red so white button text reads.
const redshift: Theme = {
  schema: 1,
  id: 'redshift',
  name: 'Redshift',
  builtin: true,
  appearance: 'dark',
  colors: {
    background: '6 40% 5%',
    foreground: '24 50% 92%',
    card: '6 36% 8%',
    cardForeground: '24 50% 92%',
    primary: '14 88% 56%',
    primaryForeground: '10 50% 6%',
    secondary: '34 90% 56%',
    secondaryForeground: '10 50% 6%',
    success: '130 50% 42%',
    successForeground: '0 0% 100%',
    destructive: '358 78% 56%',
    destructiveForeground: '0 0% 100%',
    warning: '44 95% 55%',
    warningForeground: '10 50% 6%',
    muted: '8 26% 15%',
    mutedForeground: '20 28% 66%',
    border: '8 26% 17%',
  },
  gradients: {
    primaryFrom: '6 82% 44%',
    primaryTo: '18 86% 46%',
    cardFrom: '6 36% 8%',
    cardTo: '8 38% 11%',
  },
};

// Pulse first so it is the default.
export const BUILTIN_THEMES: Theme[] = [pulse, daybreak, aurora, ember, matrix, midnight, jetBlack, redshift];

export function findBuiltin(id: string): Theme | undefined {
  return BUILTIN_THEMES.find((t) => t.id === id);
}
