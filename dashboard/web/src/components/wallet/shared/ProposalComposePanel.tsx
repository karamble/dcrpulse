// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { AlertCircle, Loader2, Send } from 'lucide-react';
import { PassphraseModal } from '../PassphraseModal';
import { MsigWallet, proposeMsigSpend } from '../../../services/msigApi';

const MAX_ATOMS = 2_100_000_000_000_000;

const parseAmount = (raw: string): { atoms: number; error: string | null } => {
  const trimmed = raw.trim();
  if (!trimmed) return { atoms: 0, error: 'Enter an amount' };
  if (!/^\d*\.?\d{0,8}$/.test(trimmed)) return { atoms: 0, error: 'Up to 8 decimal places' };
  const atoms = Math.round(Number(trimmed) * 1e8);
  if (!Number.isFinite(atoms) || atoms <= 0) return { atoms: 0, error: 'Enter an amount above zero' };
  if (atoms > MAX_ATOMS) return { atoms: 0, error: 'Amount is too large' };
  return { atoms, error: null };
};

const TTL_CHOICES = [
  { label: '1 hour', secs: 3600 },
  { label: '24 hours', secs: 86400 },
  { label: '3 days', secs: 259200 },
  { label: '7 days', secs: 604800 },
];

// ProposalComposePanel builds a payment from a shared wallet and sends it
// to the chosen cosigners for approval.
export const ProposalComposePanel = ({
  wallet,
  onProposed,
}: {
  wallet: MsigWallet;
  onProposed: () => void;
}) => {
  const [address, setAddress] = useState('');
  const [amount, setAmount] = useState('');
  const [note, setNote] = useState('');
  const [ttl, setTtl] = useState(86400);
  const [queue, setQueue] = useState<string[]>(() => wallet.peers.map((p) => p.uid));
  const [askPass, setAskPass] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const parsed = useMemo(() => parseAmount(amount), [amount]);
  const needed = wallet.m - 1;
  const enough = queue.length >= needed;
  const valid = address.trim().length > 0 && parsed.error === null && enough;

  const toggle = (uid: string) => {
    setQueue((prev) => (prev.includes(uid) ? prev.filter((u) => u !== uid) : [...prev, uid]));
  };

  const submit = async (passphrase: string) => {
    setBusy(true);
    setErr(null);
    try {
      await proposeMsigSpend(
        wallet.address!,
        [{ address: address.trim(), amountAtoms: parsed.atoms }],
        queue,
        note.trim(),
        ttl,
        wallet.own?.account ?? 0,
        passphrase,
      );
      setAddress('');
      setAmount('');
      setNote('');
      setAskPass(false);
      onProposed();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not create the payment');
      throw e;
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <Send className="h-4 w-4 text-primary" />
        <p className="text-sm font-medium">Send from this wallet</p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium mb-1">To address</label>
          <input
            type="text"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="Dc..."
            className="w-full px-3 py-2 rounded-lg bg-background border border-border font-mono text-sm focus:outline-none focus:border-primary"
          />
        </div>
        <div>
          <label className="block text-sm font-medium mb-1">Amount (DCR)</label>
          <input
            type="text"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="0.00000000"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
          />
          {amount.trim() !== '' && parsed.error && (
            <p className="text-xs text-destructive mt-1">{parsed.error}</p>
          )}
        </div>
        <div>
          <label className="block text-sm font-medium mb-1">Give cosigners</label>
          <select
            value={ttl}
            onChange={(e) => setTtl(Number(e.target.value))}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
          >
            {TTL_CHOICES.map((c) => (
              <option key={c.secs} value={c.secs}>{c.label}</option>
            ))}
          </select>
          <p className="text-xs text-muted-foreground mt-1">
            After this, the request moves to the next cosigner.
          </p>
        </div>
        <div className="sm:col-span-2">
          <label className="block text-sm font-medium mb-1">Note for cosigners (optional)</label>
          <input
            type="text"
            value={note}
            maxLength={200}
            onChange={(e) => setNote(e.target.value)}
            placeholder="What is this payment for?"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
          />
        </div>
      </div>

      <div>
        <p className="text-sm font-medium mb-2">
          Ask these cosigners, in order (need {needed})
        </p>
        <div className="space-y-1">
          {wallet.peers.map((p) => {
            const pos = queue.indexOf(p.uid);
            return (
              <button
                key={p.uid}
                type="button"
                onClick={() => toggle(p.uid)}
                className="w-full flex items-center gap-3 px-3 py-2 rounded-lg border border-border/50 hover:bg-muted/30 text-left"
              >
                <span
                  className={`h-5 w-5 shrink-0 rounded-full border text-[10px] font-semibold flex items-center justify-center ${
                    pos >= 0 ? 'bg-primary border-primary text-primary-foreground' : 'border-border'
                  }`}
                >
                  {pos >= 0 ? pos + 1 : ''}
                </span>
                <span className="flex-1 min-w-0 truncate text-sm">{p.nick || p.uid.slice(0, 12)}</span>
              </button>
            );
          })}
        </div>
        {!enough && (
          <p className="text-xs text-destructive mt-2">
            Pick at least {needed} cosigner{needed === 1 ? '' : 's'} so the payment can reach{' '}
            {wallet.m} signatures.
          </p>
        )}
      </div>

      {err && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      <button
        type="button"
        onClick={() => setAskPass(true)}
        disabled={!valid || busy}
        className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
      >
        {busy && <Loader2 className="h-4 w-4 animate-spin" />}
        Create payment
      </button>

      <PassphraseModal
        isOpen={askPass}
        title="Sign this payment"
        description="Your signature is added first, then the payment goes to your cosigners for approval."
        submitLabel="Sign and send"
        busyLabel="Signing..."
        onSubmit={submit}
        onClose={() => setAskPass(false)}
      />
    </div>
  );
};

export default ProposalComposePanel;
