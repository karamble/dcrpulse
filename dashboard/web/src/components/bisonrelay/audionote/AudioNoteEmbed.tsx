// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import { Download, Loader2, Mic, Pause, Play } from 'lucide-react';
import { formatBytes } from '../../../utils/bytes';
import { base64ToBytes } from './packetFraming';
import { parseOggOpus } from './oggDuration';

export const formatClock = (seconds: number): string => {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
};

// Bound on what is fetched for a logged note; a voice note is under 400 KB
// and an image past this size also stays a chip.
const MAX_NOTE_BYTES = 4 * 1024 * 1024;

// AudioNoteEmbed plays a voice note. Our own echo carries the bytes inline as
// data=; a received note has been written to disk by Bison Relay and its tag
// points at localfilename= instead, so the bytes are fetched from the embed
// route. Either way the bytes are parsed before anything plays: the tag's
// type= and filename= are the peer's claim, and a stream that is not
// Ogg/Opus renders the caller's fallback instead. Playback goes through an
// object URL rather than a data: URL, revoked when the chip goes away.
export const AudioNoteEmbed = ({
  dataB64,
  fileUrl,
  filename,
  fallback,
}: {
  dataB64?: string;
  fileUrl?: string;
  filename: string;
  fallback: ReactNode;
}) => {
  const inline = useMemo(() => (dataB64 ? base64ToBytes(dataB64) : null), [dataB64]);
  const [fetched, setFetched] = useState<Uint8Array<ArrayBuffer> | null>(null);
  const [fetchErr, setFetchErr] = useState<string | null>(null);
  const bytes = inline ?? fetched;
  const info = useMemo(() => (bytes ? parseOggOpus(bytes) : null), [bytes]);
  const playable = info ? bytes : null;
  const [url, setUrl] = useState('');
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    if (inline || !fileUrl) return undefined;
    let cancelled = false;
    setFetched(null);
    setFetchErr(null);
    fetch(fileUrl, { credentials: 'same-origin' })
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const declared = Number(res.headers.get('content-length') ?? 0);
        if (declared > MAX_NOTE_BYTES) throw new Error('too large');
        const buf = await res.arrayBuffer();
        if (buf.byteLength > MAX_NOTE_BYTES) throw new Error('too large');
        if (!cancelled) setFetched(new Uint8Array(buf));
      })
      .catch((e: any) => { if (!cancelled) setFetchErr(e?.message ?? String(e)); });
    return () => { cancelled = true; };
  }, [inline, fileUrl]);

  useEffect(() => {
    if (!playable) return undefined;
    const u = URL.createObjectURL(new Blob([playable], { type: 'audio/ogg' }));
    setUrl(u);
    return () => URL.revokeObjectURL(u);
  }, [playable]);

  if (bytes && !info) return <>{fallback}</>;

  const toggle = () => {
    const el = audioRef.current;
    if (!el) return;
    if (el.paused) void el.play().catch(() => setPlaying(false));
    else el.pause();
  };

  const seek = (seconds: number) => {
    const el = audioRef.current;
    if (!el) return;
    el.currentTime = seconds;
    setPosition(seconds);
  };

  const duration = info?.seconds ?? 0;
  const ready = !!url;
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded border border-border/40 bg-background/40 min-w-[14rem] max-w-full">
      <audio
        ref={audioRef}
        src={url || undefined}
        preload="metadata"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => { setPlaying(false); setPosition(0); }}
        onTimeUpdate={(e) => setPosition(e.currentTarget.currentTime)}
      />
      <button
        type="button"
        onClick={toggle}
        disabled={!ready}
        aria-label={playing ? 'Pause voice note' : 'Play voice note'}
        title={playing ? 'Pause' : 'Play'}
        className="shrink-0 h-8 w-8 flex items-center justify-center rounded-full bg-primary/15 text-primary hover:bg-primary/25 transition-colors disabled:opacity-50"
      >
        {!ready && !fetchErr ? <Loader2 className="h-4 w-4 animate-spin" /> : playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4 ml-0.5" />}
      </button>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
          <Mic className="h-3 w-3" />
          <span>Voice note</span>
          {playable ? (
            <>
              <span>· {formatClock(position)} / {formatClock(duration)}</span>
              <span>· {formatBytes(playable.length)}</span>
            </>
          ) : (
            <span>· {fetchErr ? `not available (${fetchErr})` : 'loading…'}</span>
          )}
        </div>
        <input
          type="range"
          min={0}
          max={Math.max(duration, 0.01)}
          step={0.05}
          value={Math.min(position, duration)}
          onChange={(e) => seek(Number(e.target.value))}
          disabled={!ready}
          aria-label="Seek voice note"
          className="w-full h-1 mt-1 accent-primary cursor-pointer disabled:opacity-40"
        />
      </div>
      <a
        href={url || fileUrl || undefined}
        download={filename || 'audionote.opus'}
        className="shrink-0 p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
        title="Save"
        aria-label="Save voice note"
      >
        <Download className="h-4 w-4" />
      </a>
    </div>
  );
};
