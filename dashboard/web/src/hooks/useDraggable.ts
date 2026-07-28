// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** useDraggable moves and resizes a panel with pointer events.
 *
 *  Pointer capture is what makes dragging over the panel's iframe work: without
 *  it the frame keeps every pointermove once the cursor crosses into it. */
export function useDraggable(storageKey: string, initial: Rect) {
  const [rect, setRect] = useState<Rect>(() => clamp(restore(storageKey) ?? initial));
  const [dragging, setDragging] = useState(false);
  const from = useRef<{ x: number; y: number; rect: Rect; mode: 'move' | 'resize' } | null>(null);

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(rect));
    } catch {
      // Private mode or a full quota; the panel just forgets its place.
    }
  }, [storageKey, rect]);

  useEffect(() => {
    const onResize = () => setRect((r) => clamp(r));
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  const onPointerDown = useCallback(
    (mode: 'move' | 'resize') => (e: React.PointerEvent) => {
      if (e.button !== 0) return;
      // A control inside the drag surface keeps its own click: capturing the
      // pointer here would swallow it before it ever reaches the button.
      if ((e.target as HTMLElement).closest('button, a, input, select, textarea')) return;
      e.preventDefault();
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
      from.current = { x: e.clientX, y: e.clientY, rect, mode };
      setDragging(true);
    },
    [rect],
  );

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    const start = from.current;
    if (!start) return;
    const dx = e.clientX - start.x;
    const dy = e.clientY - start.y;
    setRect(
      clamp(
        start.mode === 'move'
          ? { ...start.rect, x: start.rect.x + dx, y: start.rect.y + dy }
          : {
              ...start.rect,
              w: Math.max(320, start.rect.w + dx),
              h: Math.max(220, start.rect.h + dy),
            },
      ),
    );
  }, []);

  const onPointerUp = useCallback((e: React.PointerEvent) => {
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture?.(e.pointerId)) el.releasePointerCapture(e.pointerId);
    from.current = null;
    setDragging(false);
  }, []);

  return {
    rect,
    dragging,
    dragHandlers: {
      onPointerDown: onPointerDown('move'),
      onPointerMove,
      onPointerUp,
      onPointerCancel: onPointerUp,
    },
    resizeHandlers: {
      onPointerDown: onPointerDown('resize'),
      onPointerMove,
      onPointerUp,
      onPointerCancel: onPointerUp,
    },
  };
}

/** clamp keeps the panel on screen after a resize. */
function clamp(r: Rect): Rect {
  const maxW = Math.max(320, window.innerWidth - 16);
  const maxH = Math.max(240, window.innerHeight - 16);
  const w = Math.min(r.w, maxW);
  const h = Math.min(r.h, maxH);
  return {
    w,
    h,
    x: Math.min(Math.max(r.x, 8), Math.max(8, window.innerWidth - w - 8)),
    y: Math.min(Math.max(r.y, 8), Math.max(8, window.innerHeight - h - 8)),
  };
}

function restore(key: string): Rect | null {
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (
      typeof parsed?.x === 'number' &&
      typeof parsed?.y === 'number' &&
      typeof parsed?.w === 'number' &&
      typeof parsed?.h === 'number'
    ) {
      return parsed as Rect;
    }
  } catch {
    // Nothing usable stored.
  }
  return null;
}
