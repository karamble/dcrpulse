// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ShieldAlert, X } from 'lucide-react';

// Shown every time one of the agent listeners is switched on. The bearer token
// travels in a plain HTTP header, and encrypting that hop is the operator's to
// arrange; the listener itself cannot.
export const PlaintextTokenWarning = ({
  surface,
  onCancel,
  onConfirm,
}: {
  surface: 'agents' | 'bridge';
  onCancel: () => void;
  onConfirm: () => void;
}) => {
  const bridge = surface === 'bridge';
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <ShieldAlert className="h-5 w-5 text-warning" />
            Before you turn this on
          </h3>
          <button onClick={onCancel} className="text-muted-foreground hover:text-foreground" aria-label="Close">
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="p-6 space-y-3 text-sm">
          <p>
            {bridge
              ? 'Agents reach the Bison Relay bridge with a bearer token sent in a plain HTTP header.'
              : 'Agents authenticate to this listener with a bearer token sent in a plain HTTP header.'}{' '}
            dcrpulse does not encrypt that connection. Anyone who can read the traffic between an agent and
            this machine can read the token and use it as that agent.
          </p>
          <p>
            By default the listener answers only on this machine. If you open it to a network, the encryption
            is yours to provide: put it behind an HTTPS reverse proxy, reach it over a VPN or an overlay network
            such as Tailscale or WireGuard, or keep it on a network you trust end to end. Never expose it
            directly to the internet.
          </p>
          {bridge && (
            <p>
              The hop from this machine to the bots runs over Bison Relay, which is encrypted end to end. The
              plain leg is only between the agent and this machine.
            </p>
          )}
          <p>
            {bridge
              ? 'You can recycle the token here at any time, and calls stay limited to the bots you allow and the spend caps you set. Until you recycle it, a leaked token is that access.'
              : 'You can revoke or block a token here at any time, and each agent stays limited to the domains and spend grants you give it. Until you revoke it, a leaked token is that agent\'s access.'}
          </p>
        </div>
        <div className="flex gap-2 p-6 pt-0">
          <button
            type="button"
            onClick={onCancel}
            className="flex-1 px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90"
          >
            I have secured the connection, turn it on
          </button>
        </div>
      </div>
    </div>
  );
};
