// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { KeyEnds } from '../../AddressGroups';
import { PassphraseModal } from '../PassphraseModal';
import {
  MsigWallet,
  activateMsigRound,
  confirmMsigRoster,
} from '../../../services/msigApi';

// ConfirmRosterCard is the checkpoint between "every key is in" and "this
// wallet can hold money": no receive address is handed out until the user
// signs the assembled key set. The fingerprint hashes that key set, so every
// cosigner's card shows the same value.
export const ConfirmRosterCard = ({
  rec,
  onDone,
}: {
  rec: MsigWallet;
  onDone: () => void;
}) => {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reviewing is the initiator's turn, confirming the cosigner's.
  const isInitiator = rec.status === 'reviewing';
  const keys = [
    ...(rec.ownHd?.xpub ? [{ nick: 'You', xpub: rec.ownHd.xpub, self: true }] : []),
    ...(rec.peers ?? []).map((p) => ({
      nick: p.nick || p.uid.slice(0, 12),
      xpub: p.xpub || '',
      self: false,
    })),
  ];

  const submit = async (passphrase: string) => {
    setError(null);
    if (isInitiator) {
      await activateMsigRound(rec.tempId, passphrase);
    } else {
      await confirmMsigRoster(rec.tempId, passphrase);
    }
    setOpen(false);
    onDone();
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/40 space-y-4">
      <div className="flex items-start gap-2">
        <ShieldCheck className="h-5 w-5 text-primary mt-0.5 shrink-0" />
        <div className="space-y-1">
          <h3 className="font-medium">Confirm this wallet's key set</h3>
          <p className="text-sm text-muted-foreground">
            These {rec.n} keys will control <span className="font-medium">{rec.label}</span>;
            any {rec.m} of them can spend. Every cosigner signs this exact list before
            the wallet activates, and confirming adds your signature.
          </p>
        </div>
      </div>

      <ul className="space-y-2">
        {keys.map((k, i) => (
          <li
            key={`${k.nick}-${i}`}
            className="flex items-center justify-between gap-3 p-2 rounded-lg bg-background/40 border border-border/40"
          >
            <span className="text-sm truncate">{k.nick}</span>
            <span className="text-xs text-muted-foreground shrink-0">
              {k.xpub ? <KeyEnds value={k.xpub} /> : 'no key'}
            </span>
          </li>
        ))}
      </ul>

      {rec.keySetFingerprint && (
        <p className="text-xs text-muted-foreground">
          Key set fingerprint{' '}
          <span className="font-mono text-foreground">{rec.keySetFingerprint}</span>.
          Every cosigner's screen shows the same value.
        </p>
      )}

      {error && <p className="text-xs text-destructive">{error}</p>}

      <button
        type="button"
        onClick={() => setOpen(true)}
        className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:opacity-90 transition-opacity"
      >
        Confirm and sign
      </button>

      <PassphraseModal
        isOpen={open}
        title="Confirm the key set"
        description="Your wallet passphrase unlocks the account key that signs this key set."
        submitLabel="Confirm and sign"
        busyLabel="Signing"
        onSubmit={submit}
        onClose={() => setOpen(false)}
      />
    </div>
  );
};
