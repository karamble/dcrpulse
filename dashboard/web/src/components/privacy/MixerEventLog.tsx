import { useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, ScrollText } from 'lucide-react';
import { MixerEvent, subscribeMixerEvents } from '../../services/api';

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
    if (expanded && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [events, expanded]);

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
        <span className="text-xs text-muted-foreground">
          {events.length} event{events.length === 1 ? '' : 's'}
        </span>
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
