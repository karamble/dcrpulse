// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Download, Mic, Pause, Play } from 'lucide-react';
import { formatBytes } from '../../../utils/bytes';
import { base64ToBytes } from './packetFraming';
import { oggDurationSeconds } from './oggDuration';

export const formatClock = (seconds: number): string => {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
};

// AudioNoteEmbed plays a received voice note. The bytes go into an object
// URL rather than a data: URL so the megabyte-scale base64 never lands in
// the DOM, and the URL is revoked when the chip goes away.
export const AudioNoteEmbed = ({ dataB64, filename }: { dataB64: string; filename: string }) => {
  const bytes = useMemo(() => base64ToBytes(dataB64), [dataB64]);
  const total = useMemo(() => oggDurationSeconds(bytes), [bytes]);
  const [url, setUrl] = useState('');
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    const u = URL.createObjectURL(new Blob([bytes], { type: 'audio/ogg' }));
    setUrl(u);
    return () => URL.revokeObjectURL(u);
  }, [bytes]);

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

  const duration = total ?? 0;
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
        disabled={!url}
        aria-label={playing ? 'Pause voice note' : 'Play voice note'}
        title={playing ? 'Pause' : 'Play'}
        className="shrink-0 h-8 w-8 flex items-center justify-center rounded-full bg-primary/15 text-primary hover:bg-primary/25 transition-colors disabled:opacity-50"
      >
        {playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4 ml-0.5" />}
      </button>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
          <Mic className="h-3 w-3" />
          <span>Voice note</span>
          <span>· {formatClock(position)} / {formatClock(duration)}</span>
          <span>· {formatBytes(bytes.length)}</span>
        </div>
        <input
          type="range"
          min={0}
          max={Math.max(duration, 0.01)}
          step={0.05}
          value={Math.min(position, duration)}
          onChange={(e) => seek(Number(e.target.value))}
          aria-label="Seek voice note"
          className="w-full h-1 mt-1 accent-primary cursor-pointer"
        />
      </div>
      <a
        href={url || undefined}
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
