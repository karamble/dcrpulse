// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, Wallet } from 'lucide-react';
import { createDexWallet } from '../../services/dcrdexApi';

interface DexWalletSetupProps {
  onReady: () => void;
}

// DexWalletSetup configures DCRDEX's Decred wallet against the dashboard's
// dcrwallet. It creates a dedicated "dex" account (using the wallet passphrase)
// and registers it with the DEX backend. The passphrase is used for the request
// only and is not stored.
export const DexWalletSetup = ({ onReady }: DexWalletSetupProps) => {
  const [pass, setPass] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pass || busy) return;
    setBusy(true);
    setErr(null);
    try {
      await createDexWallet(pass);
      onReady();
    } catch (e2: any) {
      const body = e2?.response?.data;
      setErr((typeof body === 'string' && body.trim()) || e2?.message || 'Request failed');
      setBusy(false);
    }
  };

  return (
    <div className="min-h-[60vh] flex items-center justify-center p-6">
      <div className="w-full max-w-md p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-5">
        <div className="flex items-center gap-2">
          <Wallet className="h-5 w-5 text-primary" />
          <h2 className="text-lg font-semibold">Connect your Decred wallet</h2>
        </div>

        <p className="text-sm text-muted-foreground">
          DCRDEX trades from a dedicated <span className="font-mono">dex</span> account in your
          wallet. Enter your wallet passphrase to create that account (if needed) and connect it.
          The passphrase is used only for this request and is not stored.
        </p>

        <form onSubmit={onSubmit} className="space-y-4">
          <div>
            <label className="block text-xs text-muted-foreground mb-1" htmlFor="dex-wallet-pass">
              Wallet passphrase
            </label>
            <input
              id="dex-wallet-pass"
              type="password"
              autoFocus
              value={pass}
              onChange={(e) => setPass(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
            />
          </div>

          {err && (
            <div className="p-3 rounded-lg bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-start gap-2">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}

          <button
            type="submit"
            disabled={!pass || busy}
            className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2.5 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {busy ? 'Connecting…' : 'Connect wallet'}
          </button>
        </form>
      </div>
    </div>
  );
};
