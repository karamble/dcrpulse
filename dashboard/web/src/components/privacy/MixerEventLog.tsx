import { useEffect, useRef, useState } from 'react';
import { Bug, ChevronDown, ChevronRight, ScrollText } from 'lucide-react';
import {
  MixerEvent,
  getMixerDebug,
  setMixerDebug,
  subscribeMixerEvents,
} from '../../services/api';

const MAX_EVENTS = 200;

const levelClass = (level: MixerEvent['level']) => {
  switch (level) {
    case 'error':
      return 'text-destructive';
    case 'warn':
      return 'text-warning';
    default:
      return 'text-muted-foreground';
  }
};

export const MixerEventLog = () => {
  const [expanded, setExpanded] = useState(false);
  const [events, setEvents] = useState<MixerEvent[]>([]);
  const [debugOn, setDebugOn] = useState(false);
  const [debugBusy, setDebugBusy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const cleanup = subscribeMixerEvents(
      (ev) => {
        setEvents((prev) => {
          const next = [...prev, ev];
          if (next.length > MAX_EVENTS) next.splice(0, next.length - MAX_EVENTS);
          return next;
        });
      },
      (err) => console.error('Mixer events WebSocket error:', err),
    );
    return cleanup;
  }, []);

  useEffect(() => {
    getMixerDebug()
      .then((r) => setDebugOn(r.enabled))
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (expanded && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [events, expanded]);

  const toggleDebug = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (debugBusy) return;
    setDebugBusy(true);
    try {
      const next = await setMixerDebug(!debugOn);
      setDebugOn(next.enabled);
    } catch (err) {
      console.error('Failed to toggle mixer debug:', err);
    } finally {
      setDebugBusy(false);
    }
  };

  return (
    <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden">
      <div
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-4 hover:bg-muted/10 transition-colors cursor-pointer"
      >
        <div className="flex items-center gap-2">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
          <ScrollText className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">Mixer events</h3>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={toggleDebug}
            disabled={debugBusy}
            title="Toggle MIXC + TKBY debug logging on dcrwallet"
            className={`flex items-center gap-1 px-2 py-1 rounded text-xs transition-colors ${
              debugOn
                ? 'bg-warning/20 text-warning hover:bg-warning/30'
                : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
            } disabled:opacity-50 disabled:cursor-wait`}
          >
            <Bug className="h-3 w-3" />
            Debug
          </button>
          <span className="text-xs text-muted-foreground">
            {events.length} event{events.length === 1 ? '' : 's'}
          </span>
        </div>
      </div>
      {expanded && (
        <div
          ref={scrollRef}
          className="max-h-64 overflow-y-auto px-4 pb-4 border-t border-border/30 space-y-1 font-mono text-xs"
        >
          {events.length === 0 ? (
            <p className="py-4 text-center text-muted-foreground">No events yet.</p>
          ) : (
            events.map((ev, i) => (
              <div key={i} className="flex gap-2">
                <span className="text-muted-foreground shrink-0">
                  {new Date(ev.timestamp).toLocaleTimeString()}
                </span>
                <span className={`shrink-0 uppercase ${levelClass(ev.level)}`}>{ev.level}</span>
                <span className="text-foreground">{ev.message}</span>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
};
