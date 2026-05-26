// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, AlertTriangle } from 'lucide-react';
import { sendDexWallet, type DexWalletState } from '../../services/dcrdexApi';
import { fmtAmt } from './dexFormat';

interface Props {
  wallet: DexWalletState;
  onSent: () => void;
}

// DexWalletSend is a send form for a wallet. The amount is conventional; the
// backend converts to atoms. Spending is behind a two-step confirmation.
export const DexWalletSend = ({ wallet, onSent }: Props) => {
  const [address, setAddress] = useState('');
  const [amount, setAmount] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [sent, setSent] = useState<string | null>(null);

  const value = parseFloat(amount);
  const valid = address.trim() !== '' && value > 0 && value <= wallet.available;

  const send = async () => {
    setBusy(true);
    setErr(null);
    try {
      const coin = await sendDexWallet(wallet.assetID, value, address.trim());
      setSent(coin);
      setAddress('');
      setAmount('');
      setConfirming(false);
      onSent();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Send failed');
      setConfirming(false);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div>
        <label className="block text-xs text-muted-foreground mb-1">Recipient address</label>
        <input
          value={address}
          onChange={(e) => {
            setAddress(e.target.value);
            setSent(null);
          }}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary"
        />
      </div>
      <div>
        <label className="flex items-center justify-between text-xs text-muted-foreground mb-1">
          <span>Amount ({wallet.symbol})</span>
          <button type="button" className="hover:text-foreground" onClick={() => setAmount(String(wallet.available))}>
            max {fmtAmt(wallet.available, 8)}
          </button>
        </label>
        <input
          type="number"
          min={0}
          value={amount}
          onChange={(e) => {
            setAmount(e.target.value);
            setSent(null);
          }}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary"
        />
      </div>

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {sent && (
        <div className="p-2.5 rounded-lg bg-success/10 border border-success/30 text-xs text-success break-all">
          Sent. Coin: {sent}
        </div>
      )}

      {!confirming ? (
        <button
          type="button"
          disabled={!valid}
          onClick={() => setConfirming(true)}
          className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Send
        </button>
      ) : (
        <>
          <div className="p-2.5 rounded-lg bg-warning/10 border border-warning/30 text-xs text-warning flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>
              Send {fmtAmt(value, 8)} {wallet.symbol} to {address.slice(0, 12)}... This spends real funds and cannot be undone.
            </span>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => setConfirming(false)}
              className="flex-1 px-4 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={send}
              className="flex-1 bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {busy ? 'Sending...' : 'Confirm'}
            </button>
          </div>
        </>
      )}
    </div>
  );
};
