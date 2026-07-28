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

// A game's own interface, framed.
//
// Three decisions here are not cosmetic.
//
// **`sandbox="allow-scripts"` and nothing else.** Adding `allow-same-origin`
// would give the framed document this dashboard's origin: the session cookie,
// same-origin access to every /api route, and localStorage. That single
// attribute would collapse the container isolation, the internal network and
// the bearer tokens all at once. It is the one thing in this file that must
// never change.
//
// **Not a modal.** No backdrop, no focus trap, and z-index below the dialogs.
// A game asks this host for money by calling /table/fund, which blocks for
// minutes while a person approves the payment *in the dashboard behind this
// panel*. A modal would deadlock the user against their own approval prompt -
// which is also why the approvals card is rendered inside the panel.
//
// **A MessageChannel rather than window.postMessage.** The frame's origin is
// opaque, so `event.origin` on a message from it is the literal string "null" -
// and any sandboxed frame on the page can produce that, so it authenticates
// nothing. A port is bound to the document that received it: if the frame
// navigates itself somewhere else, the port is neutered and nothing further
// reaches it.

export interface GamePanelProps {
  game: string;
  session: GamePanelSession;
  onClose: () => void;
}

/** refreshBefore is how long before expiry the parent rotates the token. The
 *  parent holds the dashboard session and the frame does not, so this is the
 *  only side that can - and a hand should never end because a token did. */
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

  // Hand over one end of a private channel, once, on the frame's first load.
  // The message that carries the port has to use targetOrigin '*' because an
  // opaque origin cannot be named - so it deliberately carries nothing else.
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
          // The panel will stop working when the current token lapses.
          // Saying so beats a table that silently goes quiet.
          setError('This panel lost its connection to the host. Close and reopen it.');
        });
    }, refreshBefore);
    return () => window.clearInterval(id);
  }, [send]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Escape closes the panel only when the focus is outside the frame,
      // because inside it the game may want the key.
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

  // Through a portal, because feed and post cards use backdrop-blur, and a
  // blurred ancestor creates a containing block that traps position: fixed.
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
              // allow-same-origin is never added here. See the note at the
              // top of this file: it is the one attribute that would give
              // the game this dashboard's origin.
              sandbox="allow-scripts"
              referrerPolicy="no-referrer"
              allow=""
              className={`h-full w-full border-0 ${dragging ? 'pointer-events-none' : ''}`}
            />
          </div>
          <div className="max-h-40 overflow-y-auto border-t border-border">
            {/* The one thing the frame cannot do is the one it blocks on: a
                stake or a bond needs a person to approve the payment, and
                that happens out here. */}
            <GamingSpendApprovals />
          </div>
        </>
      )}

      {!minimised && (
        <div
          className="absolute bottom-0 right-0 h-4 w-4 cursor-nwse-resize"
          aria-label="resize"
          {...resizeHandlers}
        />
      )}
    </div>,
    document.body,
  );
}
