// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { AlertCircle, ArrowDownToLine, ArrowUpFromLine, Check, Loader2, QrCode } from 'lucide-react';
import { CopyButton } from '../../explorer/CopyButton';
import { useBisonrelayLive } from '../../bisonrelay/BisonrelayLiveProvider';
import {
  MsigImportResult,
  MsigManualFrame,
  MsigWallet,
  getMsigOutbox,
  importMsigFrame,
  markMsigDelivered,
  msigFrameTypeLabel,
} from '../../../services/msigApi';

// A frame small enough for one scannable QR; larger ones use the file.
const qrLimit = 1800;

const downloadText = (text: string, filename: string) => {
  const blob = new Blob([text], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
};

// CoordinationCard is the whole manual transport in one place: outbound
// frames wait here as per-cosigner hand-over cards, and inbound frames
// are pasted or opened here, attributed to whoever handed them over.
export const CoordinationCard = ({
  wallet,
  onChanged,
}: {
  wallet: MsigWallet;
  onChanged: () => void;
}) => {
  const [frames, setFrames] = useState<MsigManualFrame[]>([]);
  const [qrFor, setQrFor] = useState<string | null>(null);
  const [importText, setImportText] = useState('');
  // Attribution is deliberately unsticky: it IS the sender identity, so
  // every import must name its courier anew.
  const [fromUid, setFromUid] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);
  const { addListener } = useBisonrelayLive();

  const load = useCallback(async () => {
    try {
      setFrames(await getMsigOutbox(wallet.tempId));
    } catch {
      // The card lives on a page that already surfaces wallet errors.
    }
  }, [wallet.tempId]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    const remove = addListener((evt) => {
      if (evt.type !== 'msig' && evt.type !== 'msig-state') return;
      if (timer) clearTimeout(timer);
      timer = setTimeout(load, 300);
    });
    return () => {
      if (timer) clearTimeout(timer);
      remove();
    };
  }, [addListener, load]);

  const done = async (f: MsigManualFrame) => {
    try {
      await markMsigDelivered(wallet.tempId, f.mid, f.toUid);
      await load();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not update the message');
    }
  };

  const describeOutcome = (res: MsigImportResult, from: string): string => {
    const what = msigFrameTypeLabel(res.type).toLowerCase();
    switch (res.outcome) {
      case 'processed':
        return `Imported the ${what} from ${from}.`;
      case 'duplicate':
        return `This ${what} was already imported.`;
      default:
        return `The ${what} did not apply; it may be stale.`;
    }
  };

  const runImport = async () => {
    if (busy || !importText.trim() || !fromUid) return;
    setBusy(true);
    setErr(null);
    setResult(null);
    try {
      const res = await importMsigFrame({
        body: importText,
        id: wallet.tempId,
        fromUid,
      });
      const from = wallet.peers.find((p) => p.uid === fromUid);
      setResult(describeOutcome(res, from?.nick || fromUid.slice(0, 12)));
      setImportText('');
      setFromUid('');
      await load();
      onChanged();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not import the message');
    } finally {
      setBusy(false);
    }
  };

  const onFile = async (file: File) => {
    setImportText((await file.text()).trim());
  };

  const byPeer = new Map<string, MsigManualFrame[]>();
  for (const f of frames) {
    byPeer.set(f.toUid, [...(byPeer.get(f.toUid) ?? []), f]);
  }

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-5">
      <div>
        <p className="text-sm font-medium">Coordination</p>
        <p className="text-xs text-muted-foreground mt-1">
          This wallet exchanges its messages by hand. Deliver each message below to its
          cosigner over any channel you trust, and import whatever they hand back.
        </p>
      </div>

      {byPeer.size > 0 && (
        <div className="space-y-3">
          {[...byPeer.entries()].map(([uid, items]) => (
            <div key={uid} className="p-4 rounded-lg bg-background border border-border/50 space-y-3">
              <p className="text-sm font-medium flex items-center gap-2">
                <ArrowUpFromLine className="h-4 w-4 text-primary" />
                Hand to {items[0].toNick || uid.slice(0, 12)}
              </p>
              {items.map((f) => (
                <div key={`${f.mid}:${f.toUid}`} className="space-y-2">
                  <p className="text-xs text-muted-foreground">{msigFrameTypeLabel(f.type)}</p>
                  <div className="flex items-center gap-2 flex-wrap">
                    <CopyButton text={f.body} label="Copy message" />
                    <button
                      type="button"
                      onClick={() => downloadText(f.body, `${wallet.label}-${f.type}.msig.txt`)}
                      className="px-3 py-1.5 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2"
                    >
                      <ArrowDownToLine className="h-4 w-4" />
                      Save as file
                    </button>
                    {f.body.length <= qrLimit && (
                      <button
                        type="button"
                        onClick={() => setQrFor(qrFor === `${f.mid}:${f.toUid}` ? null : `${f.mid}:${f.toUid}`)}
                        className="px-3 py-1.5 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2"
                      >
                        <QrCode className="h-4 w-4" />
                        QR
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => done(f)}
                      className="px-3 py-1.5 rounded-lg border border-border hover:bg-muted/30 text-sm inline-flex items-center gap-2 text-muted-foreground"
                    >
                      <Check className="h-4 w-4" />
                      Handed over
                    </button>
                  </div>
                  {qrFor === `${f.mid}:${f.toUid}` && (
                    <div className="p-3 bg-white rounded-lg w-fit">
                      <QRCodeSVG value={f.body} size={256} level="M" />
                    </div>
                  )}
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
      {byPeer.size === 0 && (
        <p className="text-sm text-muted-foreground">Nothing waiting to be handed over.</p>
      )}

      <div className="space-y-2">
        <p className="text-sm font-medium flex items-center gap-2">
          <ArrowDownToLine className="h-4 w-4 text-primary" />
          Import a message
        </p>
        <textarea
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
          placeholder="Paste a --msig[...]-- coordination message..."
          className="w-full h-24 px-3 py-2 rounded-lg bg-background border border-border font-mono text-xs focus:outline-none focus:border-primary"
        />
        <div className="flex items-center gap-2 flex-wrap">
          <select
            value={fromUid}
            onChange={(e) => setFromUid(e.target.value)}
            className="px-2 py-2 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
          >
            <option value="" disabled>
              Who handed you this?
            </option>
            {wallet.peers.map((p) => (
              <option key={p.uid} value={p.uid}>
                From {p.nick || p.uid.slice(0, 12)}
              </option>
            ))}
          </select>
          <input
            ref={fileRef}
            type="file"
            accept=".txt,.msig,text/plain"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              e.target.value = '';
              if (file) onFile(file);
            }}
          />
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            className="px-3 py-2 rounded-lg border border-border hover:bg-muted/30 text-sm"
          >
            Open file...
          </button>
          <button
            type="button"
            onClick={runImport}
            disabled={busy || !importText.trim() || !fromUid}
            className="px-3 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            Import
          </button>
        </div>
        {result && <p className="text-xs text-success">{result}</p>}
        {err && (
          <p className="text-xs text-destructive flex items-start gap-1">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span>{err}</span>
          </p>
        )}
      </div>
    </div>
  );
};

export default CoordinationCard;
