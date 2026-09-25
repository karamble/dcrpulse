// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, Shield } from 'lucide-react';
import {
  TorControl,
  TorStatus,
  getTorControl,
  getTorStatus,
  torDaemonLabels,
} from '../../services/tor/client';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';

export type TorStripKind = 'off' | 'connected' | 'bootstrapping' | 'unreachable';

export interface TorStripState {
  kind: TorStripKind;
  pct: number;
  phase: string;
  circuits: number;
  // Services routed through Tor right now, by daemon name.
  routed: string[];
}

// Tor's bootstrap tags, from its control-spec, in the words the strip shows.
const PHASES: Record<string, string> = {
  starting: 'starting',
  conn_pt: 'connecting to a bridge',
  conn_done_pt: 'connected to a bridge',
  conn_proxy: 'connecting to a proxy',
  conn_done_proxy: 'connected to a proxy',
  conn: 'connecting to a relay',
  conn_done: 'connected to a relay',
  handshake: 'handshaking with a relay',
  handshake_done: 'handshake done',
  onehop_create: 'opening a directory connection',
  requesting_status: 'asking for network status',
  loading_status: 'loading network status',
  loading_keys: 'loading authority keys',
  requesting_descriptors: 'asking for relay descriptors',
  loading_descriptors: 'loading relay descriptors',
  enough_dirinfo: 'building circuits',
  ap_conn: 'connecting to a relay',
  ap_conn_done: 'connected to a relay',
  ap_handshake: 'handshaking with a relay',
  ap_handshake_done: 'handshake done',
  circuit_create: 'establishing a circuit',
  done: 'done',
};

// torStripState reduces the two Tor endpoints to what the strip shows.
export const torStripState = (status: TorStatus | null, control: TorControl | null): TorStripState => {
  const routed = (status?.daemons ?? []).filter((d) => d.tor && d.running).map((d) => d.name);
  const base = { pct: 0, phase: '', circuits: 0, routed };
  if (!status?.settings.enabled) return { ...base, kind: 'off' };
  if (!control?.reachable) return { ...base, kind: 'unreachable' };
  const pct = control.bootstrapPct;
  const phase = PHASES[control.bootstrapTag] ?? control.bootstrapTag;
  if (pct >= 100) return { ...base, kind: 'connected', pct, phase, circuits: control.circuits };
  return { ...base, kind: 'bootstrapping', pct, phase };
};

// nodeWaitMessage names Tor as the wait when dcrd has no peers because Tor,
// which it dials through, is not connected yet.
export const nodeWaitMessage = (nodeStatus: string, message: string, tor: TorStripState | null): string => {
  if (nodeStatus !== 'connecting' || !tor || tor.kind === 'off' || tor.kind === 'connected') return message;
  return tor.routed.includes('dcrd') ? 'Waiting for Tor' : message;
};

const DOT: Record<Exclude<TorStripKind, 'off'>, string> = {
  connected: 'bg-success',
  bootstrapping: 'bg-warning animate-pulse',
  unreachable: 'bg-destructive',
};

// TorStrip is a one-line Tor status shown on the node dashboard while Tor is
// enabled: connection state, bootstrap progress and the services routed through it.
export const TorStrip = ({ onState }: { onState?: (s: TorStripState) => void }) => {
  const [state, setState] = useState<TorStripState | null>(null);

  const load = async () => {
    let status: TorStatus | null = null;
    try {
      status = await getTorStatus();
    } catch {
      return;
    }
    let control: TorControl | null = null;
    if (status.settings.enabled) {
      try {
        control = await getTorControl();
      } catch {
        control = null;
      }
    }
    setState(torStripState(status, control));
  };

  const settled = !state || state.kind === 'off' || state.kind === 'connected';
  useVisiblePoll(load, settled ? 30_000 : 4_000);

  useEffect(() => {
    if (state) onState?.(state);
  }, [state, onState]);

  if (!state || state.kind === 'off') return null;

  const text =
    state.kind === 'connected'
      ? `Connected · ${state.circuits} circuit${state.circuits === 1 ? '' : 's'}`
      : state.kind === 'bootstrapping'
        ? `Connecting ${state.pct}% · ${state.phase}`
        : 'Not reachable';

  return (
    <div className="px-4 py-2.5 rounded-xl bg-gradient-card border border-border/50 space-y-1.5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-sm">
        <span className="flex items-center gap-2 font-medium">
          <span aria-hidden className={`h-2 w-2 rounded-full ${DOT[state.kind]}`} />
          <Shield className="h-4 w-4 text-muted-foreground" />
          Tor
        </span>
        <span className="text-muted-foreground">{text}</span>
        {state.routed.length > 0 && (
          <span className="flex flex-wrap gap-1.5" aria-label="Routed through Tor">
            {state.routed.map((name) => (
              <span key={name} className="px-2 py-0.5 rounded-full text-[11px] bg-muted/40 border border-border/50">
                {torDaemonLabels[name]?.short ?? name}
              </span>
            ))}
          </span>
        )}
        <Link
          to="/wallet/settings/tor"
          className="ml-auto flex items-center text-xs text-muted-foreground hover:text-foreground"
        >
          Settings <ChevronRight className="h-3.5 w-3.5" />
        </Link>
      </div>
      {state.kind === 'bootstrapping' && (
        <div className="h-1 rounded-full bg-muted/40 overflow-hidden" role="progressbar" aria-valuenow={state.pct} aria-valuemin={0} aria-valuemax={100}>
          <div className="h-full bg-warning transition-all" style={{ width: `${state.pct}%` }} />
        </div>
      )}
    </div>
  );
};
