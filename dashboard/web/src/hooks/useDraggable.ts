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
 *  Hand-written rather than a dependency, for one reason that is not
 *  minimalism: the panel contains an iframe from another origin, and pointer
 *  capture is what makes dragging over it work at all. Without capture the
 *  frame swallows every pointermove the instant the cursor crosses into it and
 *  the panel sticks halfway. Most drag libraries handle this; relying on one to
 *  is a thing to have checked rather than assumed.
 *
 *  The caller is expected to also set `pointer-events: none` on the frame while
 *  `dragging` is true, which is belt and braces for the same problem. */
export function useDraggable(storageKey: string, initial: Rect) {
  const [rect, setRect] = useState<Rect>(() => clamp(restore(storageKey) ?? initial));
  const [dragging, setDragging] = useState(false);
  const from = useRef<{ x: number; y: number; rect: Rect; mode: 'move' | 'resize' } | null>(null);

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(rect));
    } catch {
      // Private mode, or a full quota. The panel still works; it just does
      // not remember where it was.
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
      e.preventDefault();
      // Capture is the load-bearing line. It routes every subsequent
      // pointermove to this element even while the cursor is over the
      // iframe, which is otherwise a different document that keeps them.
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
              w: Math.max(360, start.rect.w + dx),
              h: Math.max(280, start.rect.h + dy),
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

/** clamp keeps the panel on screen, including after the window is resized to
 *  something smaller than where the panel was left. */
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
    // Nothing usable was stored. Fall back to the default position.
  }
  return null;
}
