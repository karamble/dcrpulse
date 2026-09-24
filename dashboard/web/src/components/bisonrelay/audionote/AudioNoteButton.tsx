// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Loader2, Mic, Pause, Play, Send, Square, X } from 'lucide-react';
import { inSecureContext, supportsWebCodecsAudio } from '../realtime/AudioPipeline';
import { formatBytes } from '../../../utils/bytes';
import { formatClock } from './AudioNoteEmbed';
import { bytesToBase64, framePackets } from './packetFraming';
import {
  MAX_NOTE_SECONDS,
  OpusNoteRecorder,
  RecordedNote,
  SAMPLE_RATE,
  decodeNote,
} from './opusRecorder';

type Phase = 'idle' | 'starting' | 'recording' | 'preview' | 'sending';

// Ogg framing costs about 28 bytes per 20 ms page on top of the packets.
const estimateOggBytes = (note: RecordedNote) => note.bytes + note.packets.length * 28 + 100;

const micError = (err: any): string => {
  const name = err?.name ?? '';
  if (name === 'NotAllowedError' || name === 'SecurityError') return 'Microphone access was denied.';
  if (name === 'NotFoundError' || name === 'OverconstrainedError') return 'No microphone was found.';
  return err?.message ?? String(err);
};

// AudioNoteButton records a voice note for the open PM. It owns the whole
// record / preview flow in a popover above the input bar, so a misfire never
// reaches the peer: nothing is sent until Send is pressed on the preview.
export const AudioNoteButton = ({
  disabled,
  onSend,
}: {
  disabled?: boolean;
  onSend: (packetsB64: string) => Promise<boolean>;
}) => {
  const [phase, setPhase] = useState<Phase>('idle');
  const [seconds, setSeconds] = useState(0);
  const [note, setNote] = useState<RecordedNote | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const recorder = useRef<OpusNoteRecorder | null>(null);
  const playback = useRef<{ ctx: AudioContext; source: AudioBufferSourceNode } | null>(null);
  const buffer = useRef<AudioBuffer | null>(null);

  const stopPlayback = () => {
    const p = playback.current;
    playback.current = null;
    if (!p) return;
    try { p.source.stop(); } catch { /* ignore */ }
    void p.ctx.close().catch(() => undefined);
    setPlaying(false);
  };

  const reset = () => {
    stopPlayback();
    recorder.current?.dispose();
    recorder.current = null;
    buffer.current = null;
    setNote(null);
    setSeconds(0);
    setPhase('idle');
  };

  useEffect(() => () => {
    stopPlayback();
    recorder.current?.dispose();
    recorder.current = null;
  }, []);

  const stop = async () => {
    const rec = recorder.current;
    if (!rec) return;
    recorder.current = null;
    setPhase('preview');
    const recorded = await rec.stop();
    if (recorded.packets.length === 0) {
      setErr('Nothing was recorded.');
      reset();
      return;
    }
    setNote(recorded);
    setSeconds(recorded.seconds);
  };

  const start = async () => {
    setErr(null);
    if (!inSecureContext()) {
      setErr('Voice notes need an HTTPS page or localhost.');
      return;
    }
    if (!supportsWebCodecsAudio()) {
      setErr('Voice notes require Chrome 130+ or Firefox 130+.');
      return;
    }
    setPhase('starting');
    const rec = new OpusNoteRecorder({
      onPackets: (n) => setSeconds(n / 50),
      onLimit: () => { void stop(); },
      onError: (message) => setErr(message),
    });
    recorder.current = rec;
    try {
      await rec.start();
      if (recorder.current !== rec) return;
      setPhase('recording');
    } catch (e) {
      recorder.current = null;
      setErr(micError(e));
      setPhase('idle');
    }
  };

  const play = async () => {
    if (!note) return;
    if (playing) {
      stopPlayback();
      return;
    }
    try {
      if (!buffer.current) buffer.current = await decodeNote(note.packets);
      const ctx = new AudioContext({ sampleRate: SAMPLE_RATE });
      const source = ctx.createBufferSource();
      source.buffer = buffer.current;
      source.connect(ctx.destination);
      source.onended = () => {
        if (playback.current?.source === source) stopPlayback();
      };
      playback.current = { ctx, source };
      setPlaying(true);
      source.start();
    } catch (e: any) {
      setErr(`Playback failed: ${e?.message ?? e}`);
    }
  };

  const send = async () => {
    if (!note) return;
    stopPlayback();
    setPhase('sending');
    let ok = false;
    try {
      ok = await onSend(bytesToBase64(framePackets(note.packets)));
    } catch (e: any) {
      setErr(e?.message ?? String(e));
    }
    if (ok) reset();
    else setPhase('preview');
  };

  const open = phase !== 'idle' || err;
  const busy = phase === 'starting' || phase === 'sending';

  return (
    <div className="relative shrink-0">
      <button
        type="button"
        onClick={() => {
          if (phase === 'idle') void start();
          else if (phase === 'recording') void stop();
        }}
        disabled={disabled || busy || phase === 'preview'}
        title={phase === 'recording' ? 'Stop recording' : 'Record a voice note'}
        aria-label={phase === 'recording' ? 'Stop recording' : 'Record a voice note'}
        className={`shrink-0 p-2 rounded-lg transition-colors disabled:opacity-50 ${
          phase === 'recording'
            ? 'bg-destructive/15 text-destructive'
            : open
              ? 'bg-muted/40 text-foreground'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
        }`}
      >
        {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : phase === 'recording' ? <Square className="h-4 w-4" /> : <Mic className="h-4 w-4" />}
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 right-0 z-40 w-72 rounded-xl bg-card border border-border/50 shadow-xl p-2 text-sm">
          {phase === 'recording' && (
            <div className="flex items-center gap-2">
              <span className="h-2 w-2 rounded-full bg-destructive animate-pulse" />
              <span className="font-mono">{formatClock(seconds)} / {formatClock(MAX_NOTE_SECONDS)}</span>
              <span className="text-muted-foreground text-xs">Recording…</span>
              <button
                type="button"
                onClick={() => { void stop(); }}
                className="ml-auto px-2 py-1 rounded-md bg-destructive/15 text-destructive hover:bg-destructive/25 text-xs"
              >
                Stop
              </button>
            </div>
          )}
          {(phase === 'preview' || phase === 'sending') && note && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => { void play(); }}
                disabled={phase === 'sending'}
                aria-label={playing ? 'Pause preview' : 'Play preview'}
                className="shrink-0 h-8 w-8 flex items-center justify-center rounded-full bg-primary/15 text-primary hover:bg-primary/25 disabled:opacity-50"
              >
                {playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4 ml-0.5" />}
              </button>
              <div className="min-w-0 text-xs text-muted-foreground">
                <div className="text-foreground font-mono">{formatClock(seconds)}</div>
                <div>~{formatBytes(estimateOggBytes(note))}</div>
              </div>
              <button
                type="button"
                onClick={() => { void send(); }}
                disabled={phase === 'sending'}
                className="ml-auto flex items-center gap-1 px-2 py-1 rounded-md bg-primary text-primary-foreground hover:bg-primary/90 text-xs disabled:opacity-50"
              >
                {phase === 'sending' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Send className="h-3 w-3" />} Send
              </button>
              <button
                type="button"
                onClick={reset}
                disabled={phase === 'sending'}
                aria-label="Discard voice note"
                title="Discard"
                className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-50"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          )}
          {phase === 'preview' && !note && (
            <div className="text-xs text-muted-foreground">Finishing…</div>
          )}
          {err && (
            <div className="flex items-start gap-2 mt-1 text-xs text-destructive">
              <span className="flex-1">{err}</span>
              <button type="button" onClick={() => setErr(null)} aria-label="Dismiss" className="p-0.5 rounded hover:bg-muted/30">
                <X className="h-3 w-3" />
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
