// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from 'react';
import {
  COLOR_KEYS,
  THEME_SCHEMA_VERSION,
  Theme,
  ThemeColors,
  ThemeGradients,
  ThemeTypography,
} from './types';
import { BUILTIN_THEMES, PULSE_THEME_ID, findBuiltin } from './builtins';
import { applyTheme } from './apply';
import { cacheActiveTheme, readCachedActiveTheme } from './storage';
import { getThemeStore, saveThemeStore } from './client';
import { isHslChannels } from './themeMath';

interface ThemeContextValue {
  active: Theme;
  activeThemeId: string;
  builtins: Theme[];
  customs: Theme[];
  allThemes: Theme[];
  loading: boolean;
  setActiveTheme: (id: string) => Promise<void>;
  // Live-apply a draft without persisting; pass null to restore the active theme.
  preview: (theme: Theme | null) => void;
  saveCustomTheme: (theme: Theme) => Promise<Theme>;
  deleteCustomTheme: (id: string) => Promise<void>;
  importTheme: (json: string) => Promise<Theme>;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}

function newCustomId(): string {
  const rnd =
    typeof crypto !== 'undefined' && crypto.randomUUID
      ? crypto.randomUUID()
      : `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`;
  return `custom-${rnd}`;
}

// Validate + normalize a parsed object into a Theme. Throws on anything that
// is not a usable theme. Always assigns a fresh custom id and builtin:false,
// so an import can never overwrite a shipped theme.
export function normalizeImportedTheme(raw: unknown): Theme {
  if (!raw || typeof raw !== 'object') throw new Error('Not a theme object');
  const o = raw as Record<string, unknown>;
  if (o.schema !== THEME_SCHEMA_VERSION) {
    throw new Error(`Unsupported theme schema (expected ${THEME_SCHEMA_VERSION})`);
  }
  const colorsIn = o.colors as Record<string, unknown> | undefined;
  if (!colorsIn || typeof colorsIn !== 'object') throw new Error('Missing colors');
  const colors = {} as ThemeColors;
  for (const key of COLOR_KEYS) {
    const v = colorsIn[key];
    if (!isHslChannels(v)) throw new Error(`Invalid or missing color: ${key}`);
    colors[key] = v;
  }
  const theme: Theme = {
    schema: THEME_SCHEMA_VERSION,
    id: newCustomId(),
    name:
      typeof o.name === 'string' && o.name.trim()
        ? o.name.trim().slice(0, 60)
        : 'Imported theme',
    builtin: false,
    appearance: o.appearance === 'light' ? 'light' : 'dark',
    colors,
  };
  if (o.gradients && typeof o.gradients === 'object') {
    const g = o.gradients as Record<string, unknown>;
    const grad: ThemeGradients = {};
    (['primaryFrom', 'primaryTo', 'cardFrom', 'cardTo'] as const).forEach((k) => {
      if (isHslChannels(g[k])) grad[k] = g[k];
    });
    if (Object.keys(grad).length) theme.gradients = grad;
  }
  if (o.typography && typeof o.typography === 'object') {
    const t = o.typography as Record<string, unknown>;
    const typ: ThemeTypography = {};
    if (isHslChannels(t.headingColor)) typ.headingColor = t.headingColor;
    if (typeof t.headingWeight === 'number') typ.headingWeight = t.headingWeight;
    (['heading1Size', 'heading2Size', 'heading3Size'] as const).forEach((k) => {
      if (typeof t[k] === 'string') typ[k] = t[k] as string;
    });
    if (Object.keys(typ).length) theme.typography = typ;
  }
  if (typeof o.radius === 'string') theme.radius = o.radius;
  return theme;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  // The inline script in index.html has already applied the cached colors to
  // avoid a flash; we reconcile with the server below.
  const [cachedActive] = useState<Theme | null>(() => readCachedActiveTheme());
  const [customs, setCustoms] = useState<Theme[]>([]);
  const [activeThemeId, setActiveThemeId] = useState<string>(
    cachedActive?.id ?? PULSE_THEME_ID,
  );
  const [loading, setLoading] = useState(true);

  const allThemes = useMemo(() => [...BUILTIN_THEMES, ...customs], [customs]);

  const resolveTheme = useCallback(
    (id: string): Theme =>
      customs.find((t) => t.id === id) ??
      findBuiltin(id) ??
      (cachedActive && cachedActive.id === id ? cachedActive : undefined) ??
      BUILTIN_THEMES[0],
    [customs, cachedActive],
  );

  const active = useMemo(() => resolveTheme(activeThemeId), [resolveTheme, activeThemeId]);

  // Apply the resolved active theme before paint whenever it changes (initial
  // cache hydrate, server reconcile, or a switch).
  useLayoutEffect(() => {
    applyTheme(active);
    cacheActiveTheme(active);
  }, [active]);

  // Load the persisted store from the server once.
  useEffect(() => {
    let cancelled = false;
    getThemeStore()
      .then((store) => {
        if (cancelled) return;
        setCustoms(Array.isArray(store.customThemes) ? store.customThemes : []);
        if (store.activeThemeId) setActiveThemeId(store.activeThemeId);
      })
      .catch(() => {
        /* keep the cached/default theme when the server is unreachable */
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const persist = useCallback(async (nextCustoms: Theme[], nextActiveId: string) => {
    await saveThemeStore({
      schema: THEME_SCHEMA_VERSION,
      activeThemeId: nextActiveId,
      customThemes: nextCustoms,
    });
  }, []);

  const setActiveTheme = useCallback(
    async (id: string) => {
      setActiveThemeId(id);
      await persist(customs, id);
    },
    [customs, persist],
  );

  const preview = useCallback(
    (theme: Theme | null) => {
      applyTheme(theme ?? active);
    },
    [active],
  );

  const saveCustomTheme = useCallback(
    async (theme: Theme): Promise<Theme> => {
      const toSave: Theme = {
        ...theme,
        schema: THEME_SCHEMA_VERSION,
        builtin: false,
        id: theme.id && theme.id.startsWith('custom-') ? theme.id : newCustomId(),
      };
      const idx = customs.findIndex((t) => t.id === toSave.id);
      const next =
        idx >= 0
          ? customs.map((t, i) => (i === idx ? toSave : t))
          : [...customs, toSave];
      setCustoms(next);
      await persist(next, activeThemeId === toSave.id ? toSave.id : activeThemeId);
      return toSave;
    },
    [customs, activeThemeId, persist],
  );

  const deleteCustomTheme = useCallback(
    async (id: string) => {
      const next = customs.filter((t) => t.id !== id);
      const nextActive = activeThemeId === id ? PULSE_THEME_ID : activeThemeId;
      setCustoms(next);
      if (nextActive !== activeThemeId) setActiveThemeId(nextActive);
      await persist(next, nextActive);
    },
    [customs, activeThemeId, persist],
  );

  const importTheme = useCallback(
    async (json: string): Promise<Theme> => {
      const theme = normalizeImportedTheme(JSON.parse(json));
      return saveCustomTheme(theme);
    },
    [saveCustomTheme],
  );

  const value: ThemeContextValue = {
    active,
    activeThemeId,
    builtins: BUILTIN_THEMES,
    customs,
    allThemes,
    loading,
    setActiveTheme,
    preview,
    saveCustomTheme,
    deleteCustomTheme,
    importTheme,
  };

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
