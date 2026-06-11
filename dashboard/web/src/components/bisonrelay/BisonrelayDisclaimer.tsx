// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle, MessageSquare } from 'lucide-react';

interface Props {
  onAcknowledge: () => void;
}

export const BisonrelayDisclaimer = ({ onAcknowledge }: Props) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-6 max-w-2xl">
      <div className="flex items-center gap-3">
        <MessageSquare className="h-6 w-6 text-warning" />
        <h2 className="text-xl font-semibold">Enable Bison Relay</h2>
      </div>
      <p className="text-sm text-muted-foreground">
        Bison Relay is an end-to-end encrypted, Lightning-paid messaging network
        built on Decred. Setting it up here will run the BR client headlessly
        inside dcrpulse and bridge it to your existing dcrlnd for payments.
      </p>
      <div className="rounded-lg bg-warning/10 border border-warning/30 p-4 space-y-2">
        <div className="flex items-center gap-2 text-warning font-semibold text-sm">
          <AlertTriangle className="h-4 w-4" />
          <span>Before you continue</span>
        </div>
        <ul className="list-disc list-inside space-y-1 text-xs text-foreground/80">
          <li>Bison Relay is pre-1.0 and unaudited. Treat it as experimental.</li>
          <li>
            dcrpulse holds your BR identity key on disk in the brclientd data
            volume. Anyone with access to that volume can impersonate you.
          </li>
          <li>
            Sending and receiving messages costs DCR Lightning micropayments;
            you need a funded dcrlnd channel before BR will operate.
          </li>
          <li>
            The setup wizard will guide you through opening a channel to the
            recommended hub if one does not already exist.
          </li>
        </ul>
      </div>
      <div className="flex justify-end">
        <button
          onClick={onAcknowledge}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm"
        >
          I Understand, Continue
        </button>
      </div>
    </div>
  );
};
