// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, MessageSquare, Send } from 'lucide-react';
import {
  getBisonrelayGCDetail,
  getBisonrelayGCHistory,
  sendBisonrelayGCMessage,
  type BisonrelayMessage,
} from '../../services/bisonrelayApi';
import { getGamingGameTables } from '../../services/gamingApi';
import { linkifyChatText } from './chatLinkify';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

// The table's own conversation, beside the game.
//
// A table plays in the group chat its invitation arrived in, and the people at
// it should be able to talk. This is drawn by the dashboard, outside the frame,
// for the same reason the wallet strip is and a stronger one: the framed game
// reaches an allowlist of its own routes and has never been able to read Bison
// Relay messages. Chat inside the frame would hand a game the user's
// conversations with other people to solve a layout problem. The host already
// has the whole messaging stack; the game never sees a word of this.

const COLLAPSE_KEY = 'dcrpulse.gamePanel.chat';

export const GamingChatSidebar = ({ game, tableId }: { game: string; tableId?: string }) => {
  const [open, setOpen] = useState<boolean>(() => {
    try {
      return window.localStorage.getItem(COLLAPSE_KEY) !== 'closed';
    } catch {
      return true;
    }
  });
  const [gcid, setGcid] = useState('');
  const [gcName, setGcName] = useState('');
  const [messages, setMessages] = useState<BisonrelayMessage[]>([]);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const { addListener, clearGCUnread } = useBisonrelayLive();

  const toggle = () => {
    setOpen((was) => {
      try {
        window.localStorage.setItem(COLLAPSE_KEY, was ? 'closed' : 'open');
      } catch {
        // Remembering the preference is a nicety, not a requirement.
      }
      return !was;
    });
  };

  // Which conversation. The game reports which GC each table plays in and the
  // host relays that list; the panel's table decides, falling back to the
  // first live table so a panel opened without one still has its chat.
  useEffect(() => {
    let stopped = false;
    getGamingGameTables(game)
      .then((tables) => {
        if (stopped) return;
        const byId = tableId ? tables.find((t) => t.sid === tableId) : undefined;
        const live = tables.find((t) => !t.finished && t.gcid);
        const chosen = byId ?? live ?? tables[0];
        if (chosen?.gcid) setGcid(chosen.gcid);
      })
      .catch(() => {});
    return () => {
      stopped = true;
    };
  }, [game, tableId]);

  useEffect(() => {
    if (!gcid) return;
    let stopped = false;
    getBisonrelayGCDetail(gcid)
      .then((gc) => {
        if (!stopped && gc?.name) setGcName(gc.name);
      })
      .catch(() => {});
    getBisonrelayGCHistory(gcid)
      .then((h) => {
        if (stopped) return;
        const entries = [...(h.entries ?? [])].sort((a, b) => a.timestamp - b.timestamp);
        setMessages(entries);
      })
      .catch(() => {});
    return () => {
      stopped = true;
    };
  }, [gcid]);

  // Live messages, from the same feed the messaging page reads. Gaming frames
  // never appear here: brclientd publishes them as their own gaming-frame
  // events rather than as chat, which is the property that makes this sidebar
  // possible at all.
  useEffect(() => {
    if (!gcid) return;
    return addListener((evt) => {
      if (evt.type !== 'gc-message') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (String(payload.gcid ?? '') !== gcid) return;
      const text = String(payload.message ?? '');
      if (!text) return;
      setMessages((prev) => [
        ...prev,
        {
          message: text,
          from: String(payload.fromNick ?? ''),
          timestamp: Math.floor(Date.now() / 1000),
          internal: false,
        },
      ]);
      clearGCUnread(gcid);
    });
  }, [gcid, addListener, clearGCUnread]);

  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages, open]);

  const send = useCallback(() => {
    const text = draft.trim();
    if (!text || !gcid || sending) return;
    setSending(true);
    sendBisonrelayGCMessage(gcid, text)
      .then(() => {
        setDraft('');
        setMessages((prev) => [
          ...prev,
          {
            message: text,
            from: 'you',
            timestamp: Math.floor(Date.now() / 1000),
            internal: false,
          },
        ]);
      })
      .catch(() => {})
      .finally(() => setSending(false));
  }, [draft, gcid, sending]);

  if (!open) {
    return (
      <button
        type="button"
        onClick={toggle}
        className="flex w-9 shrink-0 flex-col items-center gap-2 border-l border-border bg-muted/30 py-3 text-muted-foreground hover:text-foreground"
        aria-label="open chat"
        title={gcName || 'table chat'}
      >
        <ChevronLeft className="h-4 w-4" />
        <MessageSquare className="h-4 w-4" />
      </button>
    );
  }

  return (
    <div className="flex w-72 shrink-0 flex-col border-l border-border bg-muted/20">
      <div className="flex items-center gap-2 border-b border-border px-2 py-1.5">
        <MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate text-sm font-medium">{gcName || 'table chat'}</span>
        <span className="flex-1" />
        <button
          type="button"
          onClick={toggle}
          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="collapse chat"
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>

      <div ref={scroller} className="flex-1 space-y-2 overflow-y-auto px-2 py-2">
        {!gcid && (
          <p className="text-xs text-muted-foreground">
            No conversation yet. The chat belongs to the group the table was made in.
          </p>
        )}
        {messages.map((m, i) => (
          <div key={i} className="text-sm leading-snug">
            {m.internal ? (
              <span className="text-xs text-muted-foreground">{m.message}</span>
            ) : (
              <>
                <span className="mr-1 font-medium text-primary/90">{m.from || '?'}</span>
                <span className="break-words text-foreground/90">
                  {linkifyChatText(m.message)}
                </span>
              </>
            )}
          </div>
        ))}
      </div>

      <div className="flex items-center gap-1 border-t border-border p-2">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              send();
            }
          }}
          placeholder={gcid ? 'Say something…' : 'No conversation'}
          disabled={!gcid}
          className="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-sm"
        />
        <button
          type="button"
          onClick={send}
          disabled={!gcid || sending || !draft.trim()}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          aria-label="send"
        >
          <Send className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
};
