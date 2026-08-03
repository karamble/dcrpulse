// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, ArrowLeft, Check, Loader2, Users } from 'lucide-react';
import {
  BisonrelayContact,
  getBisonrelayContacts,
} from '../../../services/bisonrelayApi';
import { PassphraseModal } from '../PassphraseModal';
import { createMsigWallet, MsigInvitee } from '../../../services/msigApi';

const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || '(unnamed)';

type Step = 'scheme' | 'cosigners' | 'review';

// SharedWalletCreateWizard collects the scheme and the cosigners, then
// sends the invites. The wallet passphrase creates a dedicated account
// whose extended public key is this wallet's contribution.
export const SharedWalletCreateWizard = ({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) => {
  const [step, setStep] = useState<Step>('scheme');
  const [label, setLabel] = useState('');
  const [required, setRequired] = useState(2);
  const [transport, setTransport] = useState<'' | 'manual'>('');
  const [labels, setLabels] = useState<string[]>(['']);
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [askPass, setAskPass] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getBisonrelayContacts()
      .then(setContacts)
      .catch((e) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load contacts');
      });
  }, []);

  const candidates = (contacts ?? [])
    .filter((c) => c.id?.identity)
    .sort((a, b) => displayNick(a).toLowerCase().localeCompare(displayNick(b).toLowerCase()));
  const manual = transport === 'manual';
  const cleanLabels = labels.map((l) => l.trim()).filter((l) => l.length > 0);
  const labelsUnique = new Set(cleanLabels.map((l) => l.toLowerCase())).size === cleanLabels.length;
  const total = (manual ? cleanLabels.length : selected.size) + 1;
  const schemeValid = label.trim().length > 0 && label.length <= 64 && required >= 1;
  const cosignersValid = manual
    ? cleanLabels.length >= 1 && labelsUnique && total <= 8 && required <= total
    : selected.size >= 1 && total <= 8 && required <= total;

  const invitees: MsigInvitee[] = useMemo(
    () =>
      manual
        ? cleanLabels.map((l) => ({ uid: '', nick: l }))
        : candidates
            .filter((c) => selected.has(c.id!.identity!))
            .map((c) => ({ uid: c.id!.identity!, nick: displayNick(c) })),
    [manual, labels, candidates, selected],
  );

  const toggle = (uid: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else next.add(uid);
      return next;
    });
  };

  const submit = async (passphrase: string) => {
    setErr(null);
    try {
      await createMsigWallet(label.trim(), required, invitees, transport, passphrase);
      setAskPass(false);
      onCreated();
      onClose();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not send the invitations');
      throw e;
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="w-full max-w-2xl mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="p-6 border-b border-border/50 flex items-center gap-3">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Users className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-lg font-semibold">New shared wallet</h2>
            <p className="text-sm text-muted-foreground">
              Funds move only when enough cosigners approve.
            </p>
          </div>
        </div>

        <div className="p-6 space-y-5">
          {step === 'scheme' && (
            <>
              <div>
                <label className="block text-sm font-medium mb-1">Name</label>
                <input
                  type="text"
                  value={label}
                  maxLength={64}
                  onChange={(e) => setLabel(e.target.value)}
                  placeholder="Team treasury"
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Signatures required</label>
                <input
                  type="number"
                  min={1}
                  max={8}
                  value={required}
                  onChange={(e) => setRequired(Math.max(1, Math.min(8, Number(e.target.value) || 1)))}
                  className="w-32 px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
                />
                <p className="text-xs text-muted-foreground mt-1">
                  How many cosigners must approve each payment. You pick the participants next.
                </p>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Coordination</label>
                <div className="grid gap-2 sm:grid-cols-2">
                  <button
                    type="button"
                    onClick={() => setTransport('')}
                    className={`px-3 py-2 rounded-lg border text-left ${
                      !manual ? 'border-primary bg-primary/10' : 'border-border hover:bg-muted/30'
                    }`}
                  >
                    <span className="block text-sm font-medium">Bison Relay</span>
                    <span className="block text-xs text-muted-foreground mt-0.5">
                      Messages travel automatically between your contacts.
                    </span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setTransport('manual')}
                    className={`px-3 py-2 rounded-lg border text-left ${
                      manual ? 'border-primary bg-primary/10' : 'border-border hover:bg-muted/30'
                    }`}
                  >
                    <span className="block text-sm font-medium">Manual exchange</span>
                    <span className="block text-xs text-muted-foreground mt-0.5">
                      You hand each message to your cosigners over any channel you trust.
                    </span>
                  </button>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                Creating the wallet adds a dedicated account to your wallet. Only that
                account's extended public key is shared with your cosigners; your other
                accounts stay private.
              </p>
            </>
          )}

          {step === 'cosigners' && manual && (
            <>
              <p className="text-sm text-muted-foreground">
                Name the people who hold the other keys. After the wallet is created you hand
                each of them their invitation from the Coordination card.
              </p>
              <div className="space-y-2">
                {labels.map((l, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={l}
                      maxLength={64}
                      onChange={(e) =>
                        setLabels((prev) => prev.map((v, vi) => (vi === i ? e.target.value : v)))
                      }
                      placeholder={`Cosigner ${i + 1}`}
                      className="flex-1 px-3 py-2 rounded-lg bg-background border border-border focus:outline-none focus:border-primary"
                    />
                    {labels.length > 1 && (
                      <button
                        type="button"
                        onClick={() => setLabels((prev) => prev.filter((_, vi) => vi !== i))}
                        className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm"
                      >
                        Remove
                      </button>
                    )}
                  </div>
                ))}
                {labels.length < 7 && (
                  <button
                    type="button"
                    onClick={() => setLabels((prev) => [...prev, ''])}
                    className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm"
                  >
                    Add cosigner
                  </button>
                )}
              </div>
              {!labelsUnique && (
                <p className="text-xs text-destructive">Every cosigner needs a distinct name.</p>
              )}
              <p className="text-sm">
                Scheme: <span className="font-semibold">{required} of {total}</span> keys
                {required > total && (
                  <span className="text-destructive"> (add more cosigners)</span>
                )}
              </p>
            </>
          )}

          {step === 'cosigners' && !manual && (
            <>
              <p className="text-sm text-muted-foreground">
                Pick the Bison Relay contacts who hold the other keys. They each confirm on their
                own dcrpulse before the wallet becomes usable.
              </p>
              {contacts === null ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" /> Loading contacts...
                </div>
              ) : candidates.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No Bison Relay contacts yet. Add a contact first.
                </p>
              ) : (
                <div className="max-h-72 overflow-y-auto rounded-lg border border-border/50 divide-y divide-border/50">
                  {candidates.map((c) => {
                    const uid = c.id!.identity!;
                    const on = selected.has(uid);
                    return (
                      <button
                        key={uid}
                        type="button"
                        onClick={() => toggle(uid)}
                        className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-muted/30"
                      >
                        <span
                          className={`h-4 w-4 rounded border flex items-center justify-center ${
                            on ? 'bg-primary border-primary' : 'border-border'
                          }`}
                        >
                          {on && <Check className="h-3 w-3 text-primary-foreground" />}
                        </span>
                        <span className="flex-1 min-w-0">
                          <span className="block truncate">{displayNick(c)}</span>
                          <span className="block text-xs text-muted-foreground truncate">
                            {uid.slice(0, 16)}
                          </span>
                        </span>
                      </button>
                    );
                  })}
                </div>
              )}
              <p className="text-sm">
                Scheme: <span className="font-semibold">{required} of {total}</span> keys
                {required > total && (
                  <span className="text-destructive"> (pick more cosigners)</span>
                )}
              </p>
            </>
          )}

          {step === 'review' && (
            <>
              <div className="p-4 rounded-lg bg-background border border-border/50 space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Name</span>
                  <span className="font-medium">{label}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Scheme</span>
                  <span className="font-medium">{required} of {total}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Cosigners</span>
                  <ul className="mt-1 space-y-1">
                    {invitees.map((p) => (
                      <li key={p.uid || p.nick} className="truncate">{p.nick}</li>
                    ))}
                  </ul>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                {manual
                  ? 'Creating the wallet adds the dedicated account and prepares an invitation for each cosigner on the Coordination card. Receive addresses appear once every cosigner has confirmed; do not send funds before then.'
                  : 'Sending the invitations creates the dedicated account in your wallet. Receive addresses appear once every cosigner has confirmed; do not send funds before then.'}
              </p>
            </>
          )}

          {err && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{err}</span>
            </div>
          )}
        </div>

        <div className="p-6 border-t border-border/50 flex items-center justify-between">
          <button
            type="button"
            onClick={step === 'scheme' ? onClose : () => setStep(step === 'review' ? 'cosigners' : 'scheme')}
            className="px-4 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2"
          >
            {step !== 'scheme' && <ArrowLeft className="h-4 w-4" />}
            {step === 'scheme' ? 'Cancel' : 'Back'}
          </button>
          {step === 'review' ? (
            <button
              type="button"
              onClick={() => setAskPass(true)}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm inline-flex items-center gap-2"
            >
              {manual ? 'Create wallet' : 'Send invitations'}
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setStep(step === 'scheme' ? 'cosigners' : 'review')}
              disabled={step === 'scheme' ? !schemeValid : !cosignersValid}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Continue
            </button>
          )}
        </div>
      </div>

      <PassphraseModal
        isOpen={askPass}
        title="Create the shared wallet"
        description="Your wallet passphrase creates this wallet's dedicated account. Only the account's extended public key is shared with cosigners."
        submitLabel="Create and invite"
        busyLabel="Creating..."
        onSubmit={submit}
        onClose={() => setAskPass(false)}
      />
    </div>
  );
};

export default SharedWalletCreateWizard;
