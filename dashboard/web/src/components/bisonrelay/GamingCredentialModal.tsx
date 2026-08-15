// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Download, KeyRound, X } from 'lucide-react';
import { GamingBridgeInfo, GamingCredentialMaterial } from '../../services/gamingApi';
import { CopyButton } from '../explorer/CopyButton';

// Whether this browser will let a page write to the clipboard at all.
//
// It will not over plain http on a LAN address, which is how an appliance
// dashboard is usually reached. A Copy button that silently does nothing is
// worse here than no Copy button: somebody believes a private key is on their
// clipboard, closes the panel, and the key is gone for good. So the control is
// only offered where it works, and Download - which works everywhere - is the
// one this screen leads with.
const canCopy = (): boolean =>
  typeof navigator !== 'undefined' && !!navigator.clipboard && window.isSecureContext;

const save = (name: string, text: string) => {
  const url = URL.createObjectURL(new Blob([text], { type: 'application/x-pem-file' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
};

interface Block {
  key: string;
  label: string;
  what: string;
  file: string;
  value: string;
  secret?: boolean;
}

// What an operator carries to the machine running a game, shown once.
export const GamingCredentialModal = ({
  issued,
  bridge,
  bridgeError,
  onClose,
}: {
  issued: GamingCredentialMaterial;
  bridge: GamingBridgeInfo | null;
  bridgeError?: string | null;
  onClose: () => void;
}) => {
  const [saved, setSaved] = useState<Set<string>>(new Set());
  const [confirmingClose, setConfirmingClose] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const opener = useRef<Element | null>(null);

  const blocks: Block[] = [
    {
      key: 'cert',
      label: 'Client certificate',
      what: 'who the game says it is',
      file: `${issued.game}-client.cert.pem`,
      value: issued.certPem,
    },
    {
      key: 'key',
      label: 'Client private key',
      what: 'the secret, and the only part that cannot be shown again',
      file: `${issued.game}-client.key.pem`,
      value: issued.keyPem,
      secret: true,
    },
    {
      key: 'bridge',
      label: 'Bridge certificate to pin',
      what: 'what the game pins, so it knows it is talking to this bridge',
      file: 'dcrpulse-bridge.cert.pem',
      value: issued.bridgeCertPem,
    },
  ];

  const allSaved = blocks.every((b) => saved.has(b.key));
  const note = (k: string) => setSaved((s) => new Set(s).add(k));

  useEffect(() => {
    opener.current = document.activeElement;
    box.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setConfirmingClose(!allSaved);
      if (e.key === 'Escape' && allSaved) onClose();
    };
    window.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      window.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
      (opener.current as HTMLElement | null)?.focus?.();
    };
  }, [allSaved, onClose]);

  return (
    <div className="fixed inset-0 z-50 bg-black/60 flex items-center justify-center p-4">
      <div
        ref={box}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-labelledby="gaming-cred-title"
        className="w-full max-w-2xl max-h-[85vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4 focus:outline-none"
      >
        <div className="flex items-start justify-between gap-4">
          <h3 id="gaming-cred-title" className="text-base font-semibold flex items-center gap-2">
            <KeyRound className="h-4 w-4 text-primary" />
            Credential for {issued.game} - shown once
          </h3>
          <button
            type="button"
            onClick={() => (allSaved ? onClose() : setConfirmingClose(true))}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="text-xs text-muted-foreground">
          The private key below is not stored on this machine and cannot be shown again. If you lose
          it the only remedy is generating another, which cuts {issued.game} off the moment you do.
        </p>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => {
              for (const b of blocks) save(b.file, b.value);
              setSaved(new Set(blocks.map((b) => b.key)));
            }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-xs font-semibold"
          >
            <Download className="h-3.5 w-3.5" />
            Download all three
          </button>
          {!canCopy() && (
            <span className="text-xs text-muted-foreground">
              This page is on plain http, so the browser will not let it write to the clipboard.
              Download, or select a block and copy it yourself.
            </span>
          )}
        </div>

        <div className="text-xs space-y-1">
          {bridge?.port ? (
            <span className="text-muted-foreground block">
              Port {bridge.port}. The address is whatever this machine is reachable at from wherever
              you run the game.
            </span>
          ) : bridgeError ? (
            <span className="text-warning block break-words">
              The bridge did not say which port it listens on: {bridgeError}. Read the port from the
              Bridge section before configuring the game.
            </span>
          ) : (
            <span className="text-warning block">
              The bridge listener has not started, so there is no port to give yet.
            </span>
          )}
          <span className="text-muted-foreground block">
            Fingerprint, to compare against what {issued.game} shows once it connects:
          </span>
          <span className="font-mono break-all block">{issued.fingerprint}</span>
        </div>

        {blocks.map((b) => (
          <label key={b.key} className="text-xs space-y-1 block">
            <span className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-muted-foreground">
                {b.label} - {b.what}
                {saved.has(b.key) && <span className="text-success"> · saved</span>}
              </span>
              <span className="flex items-center gap-1">
                {canCopy() && (
                  <span onClick={() => note(b.key)}>
                    <CopyButton text={b.value} label="Copy" />
                  </span>
                )}
                <button
                  type="button"
                  onClick={() => {
                    save(b.file, b.value);
                    note(b.key);
                  }}
                  className="inline-flex items-center gap-1 px-2 py-1 rounded hover:bg-muted/20 text-xs text-muted-foreground"
                >
                  <Download className="h-3 w-3" />
                  Download
                </button>
              </span>
            </span>
            <textarea
              readOnly
              value={b.value}
              rows={4}
              wrap="off"
              spellCheck={false}
              onFocus={(e) => {
                e.currentTarget.select();
                note(b.key);
              }}
              className={`w-full px-2 py-1.5 rounded-lg bg-background font-mono text-[11px] overflow-x-auto border ${
                b.secret ? 'border-destructive/30 bg-destructive/5' : 'border-border'
              }`}
            />
          </label>
        ))}

        {confirmingClose && !allSaved ? (
          <div className="space-y-2 p-3 rounded-lg bg-destructive/10 border border-destructive/30">
            <p className="text-xs text-destructive">
              Close without saving the private key? It cannot be shown again.
            </p>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setConfirmingClose(false)}
                className="px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-xs font-semibold"
              >
                Go back
              </button>
              <button
                type="button"
                onClick={onClose}
                className="px-3 py-1.5 rounded-lg bg-destructive text-destructive-foreground text-xs font-semibold"
              >
                Close anyway
              </button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={onClose}
            disabled={!allSaved}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-xs font-semibold disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {allSaved ? 'I have saved all three' : 'Save all three to continue'}
          </button>
        )}
      </div>
    </div>
  );
};
