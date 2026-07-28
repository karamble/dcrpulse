// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { GripHorizontal, Minus, X } from 'lucide-react';
import { useDraggable } from '../../hooks/useDraggable';
import { refreshGamePanel, type GamePanelSession } from '../../services/gameUiApi';
import { parseFromFrame, PROTOCOL_VERSION, type ToFrame } from './gamePanelProtocol';
import { GamingSpendApprovals } from './GamingSpendApprovals';

// A game's interface, framed. Not a modal: a game blocks on /table/fund while
// the user approves the payment in the dashboard behind this panel.

export interface GamePanelProps {
  game: string;
  session: GamePanelSession;
  onClose: () => void;
}

/** The parent rotates the token; only it holds the dashboard session. */
const refreshBefore = 4 * 60 * 1000;

export default function GamePanel({ game, session, onClose }: GamePanelProps) {
  const frame = useRef<HTMLIFrameElement>(null);
  const port = useRef<MessagePort | null>(null);
  const handed = useRef(false);
  const token = useRef(session.token);

  const [title, setTitle] = useState('');
  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');
  const [minimised, setMinimised] = useState(false);

  const { rect, dragging, dragHandlers, resizeHandlers } = useDraggable('dcrpulse.gamePanel.rect', {
    x: Math.max(8, window.innerWidth - 1000),
    y: 72,
    w: Math.min(960, window.innerWidth - 32),
    h: Math.min(640, window.innerHeight - 120),
  });

  const send = useCallback((msg: ToFrame) => {
    port.current?.postMessage(msg);
  }, []);

  // A MessageChannel rather than window.postMessage: the frame's origin is
  // opaque, so event.origin on a message from it is the string "null", which
  // any sandboxed frame can produce. The port hand-off carries no secret,
  // because its targetOrigin has to be '*'.
  const onLoad = useCallback(() => {
    if (handed.current) return;
    const win = frame.current?.contentWindow;
    if (!win) return;
    handed.current = true;

    const channel = new MessageChannel();
    port.current = channel.port1;
    channel.port1.onmessage = (ev) => {
      const msg = parseFromFrame(ev.data);
      if (!msg) return;
      switch (msg.type) {
        case 'game.ready':
          setReady(true);
          send({
            type: 'pulse.init',
            v: PROTOCOL_VERSION,
            apiBase: session.apiBase,
            token: token.current,
            expiresAt: session.expiresAt,
            tableId: session.tableId ?? null,
            payoutAddress: session.payout,
          });
          break;
        case 'game.close':
          onClose();
          break;
        case 'game.title':
          setTitle(msg.text);
          break;
        case 'game.error':
          setError(msg.message);
          break;
      }
    };
    channel.port1.start();
    win.postMessage({ type: 'pulse.port', v: PROTOCOL_VERSION }, '*', [channel.port2]);
  }, [onClose, send, session]);

  useEffect(() => {
    const id = window.setInterval(() => {
      refreshGamePanel(token.current)
        .then((next) => {
          token.current = next.token;
          send({ type: 'pulse.token', v: PROTOCOL_VERSION, token: next.token, expiresAt: next.expiresAt });
        })
        .catch(() => {
          setError('This panel lost its connection to the host. Close and reopen it.');
        });
    }, refreshBefore);
    return () => window.clearInterval(id);
  }, [send]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Inside the frame the game may want Escape.
      if (e.key === 'Escape' && document.activeElement !== frame.current) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const style: React.CSSProperties = {
    left: rect.x,
    top: rect.y,
    width: rect.w,
    height: minimised ? undefined : rect.h,
  };

  // A portal, because a backdrop-blur ancestor traps position: fixed.
  return createPortal(
    <div
      className="fixed z-40 flex flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
      style={style}
      role="dialog"
      aria-label={`${game} panel`}
    >
      <div
        className="flex cursor-grab select-none items-center gap-2 border-b border-border bg-muted/40 px-3 py-2 active:cursor-grabbing"
        {...dragHandlers}
      >
        <GripHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="text-sm font-medium capitalize">{game}</span>
        {title && <span className="truncate text-xs text-muted-foreground">{title}</span>}
        <span className="flex-1" />
        <button
          type="button"
          onClick={() => setMinimised((m) => !m)}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={minimised ? 'expand' : 'minimise'}
        >
          <Minus className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onClose}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {!minimised && (
        <>
          {error && (
            <div className="border-b border-border bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {error}
            </div>
          )}
          <div className="relative flex-1">
            {!ready && (
              <div className="absolute inset-0 flex items-center justify-center text-sm text-muted-foreground">
                Starting {game}…
              </div>
            )}
            <iframe
              ref={frame}
              src={session.uiUrl}
              onLoad={onLoad}
              title={`${game} interface`}
              // allow-same-origin would give the game this dashboard's
              // origin, its cookie and every /api route. Never add it.
              sandbox="allow-scripts"
              referrerPolicy="no-referrer"
              allow=""
              className={`h-full w-full border-0 ${dragging ? 'pointer-events-none' : ''}`}
            />
          </div>
          <div className="max-h-40 overflow-y-auto border-t border-border">
            <GamingSpendApprovals />
          </div>
        </>
      )}

      {!minimised && (
        <div
          className="absolute bottom-0 right-0 h-5 w-5 cursor-nwse-resize text-muted-foreground/70 hover:text-foreground"
          aria-label="Resize"
          title="Drag to resize"
          {...resizeHandlers}
        >
          <svg viewBox="0 0 20 20" className="h-5 w-5" aria-hidden="true">
            <path d="M19 7 L7 19 M19 12 L12 19 M19 17 L17 19" stroke="currentColor" strokeWidth="1.5" fill="none" />
          </svg>
        </div>
      )}
    </div>,
    document.body,
  );
}
