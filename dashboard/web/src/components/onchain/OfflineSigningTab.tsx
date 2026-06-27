// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { QRCodeSVG } from 'qrcode.react';
import {
  AlertCircle,
  Check,
  Download,
  ExternalLink,
  FileUp,
  Loader2,
  RotateCcw,
  Send,
  ShieldCheck,
  UploadCloud,
} from 'lucide-react';
import {
  AccountInfo,
  SignRequestExport,
  SignedTxPreview,
  broadcastSignedTransaction,
  buildSignRequest,
  decodeSignedTransaction,
  getAccounts,
} from '../../services/api';

const MAX_DCR = 21_000_000;
// Above 1 DCR a fee is almost certainly a mistake; we soft-warn (non-blocking).
const HIGH_FEE_ATOMS = 100_000_000;

const formatDcr = (atoms: number): string => (atoms / 1e8).toFixed(8);

const validateAmount = (raw: string): string | null => {
  const trimmed = raw.trim();
  if (!trimmed) return 'Amount required';
  if (!/^\d*\.?\d{0,8}$/.test(trimmed)) return 'Use a positive number with up to 8 decimals';
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n <= 0) return 'Amount must be positive';
  if (n > MAX_DCR) return `Amount must be at most ${MAX_DCR.toLocaleString()} DCR`;
  return null;
};

const base64ToBytes = (b64: string) => {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < out.length; i++) out[i] = bin.charCodeAt(i);
  return out;
};

// fileToBase64 reads a file (binary-safe) and returns its base64 contents, so a
// raw .dcrtx export survives the trip to the backend intact.
const fileToBase64 = (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const s = String(reader.result);
      const comma = s.indexOf(',');
      resolve(comma >= 0 ? s.slice(comma + 1) : s);
    };
    reader.onerror = () => reject(new Error('could not read file'));
    reader.readAsDataURL(file);
  });

const downloadBlob = (data: BlobPart, filename: string, type: string) => {
  const url = URL.createObjectURL(new Blob([data], { type }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
};

const scriptClassLabel = (sc: string): string => (sc === 'NULL_DATA' ? 'OP_RETURN' : sc);

// ExportUnsignedPanel builds an unsigned transaction from the active wallet and
// downloads it for an air-gapped device (e.g. a Foundation Passport) to sign. It
// uses no private keys, so it works for watch-only wallets.
const ExportUnsignedPanel = () => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [accountsError, setAccountsError] = useState<string | null>(null);
  const [sourceAccount, setSourceAccount] = useState<number | null>(null);
  const [recipient, setRecipient] = useState('');
  const [amount, setAmount] = useState('');
  const [sendAll, setSendAll] = useState(false);
  const [building, setBuilding] = useState(false);
  const [buildError, setBuildError] = useState<string | null>(null);
  const [construct, setConstruct] = useState<SignRequestExport | null>(null);

  useEffect(() => {
    getAccounts()
      .then((data) => {
        const visible = data
          .filter((a) => a.accountName !== 'imported')
          .sort((a, b) => a.accountNumber - b.accountNumber);
        setAccounts(visible);
        setAccountsError(null);
        setSourceAccount((prev) => prev ?? (visible.length > 0 ? visible[0].accountNumber : null));
      })
      .catch((err) => {
        console.error('Failed to load accounts:', err);
        setAccountsError('Failed to load accounts');
      });
  }, []);

  const amountError = sendAll || !amount ? null : validateAmount(amount);
  const amountAtoms = sendAll ? 0 : Math.round(parseFloat(amount || '0') * 1e8);
  const formReady =
    sourceAccount !== null && recipient.trim() !== '' && (sendAll || (!validateAmount(amount) && amountAtoms > 0));

  // Any input change invalidates a previously built transaction so a stale
  // unsigned tx is never downloaded.
  const invalidate = () => {
    setConstruct(null);
    setBuildError(null);
  };

  const onBuild = async () => {
    if (sourceAccount === null) return;
    setBuilding(true);
    setBuildError(null);
    setConstruct(null);
    try {
      const resp = await buildSignRequest({
        sourceAccount,
        address: recipient.trim(),
        amountAtoms,
        sendAll,
      });
      setConstruct(resp);
    } catch (err: any) {
      const body = err?.response?.data;
      setBuildError(typeof body === 'string' ? body : err?.message || 'Failed to build transaction');
    } finally {
      setBuilding(false);
    }
  };

  const onDownload = () => {
    if (!construct) return;
    downloadBlob(base64ToBytes(construct.signRequestB64), 'unsigned.dcrtx', 'application/octet-stream');
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
      <div className="flex items-center gap-3 mb-4">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <Download className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h2 className="text-xl font-semibold">1. Export an unsigned transaction</h2>
          <p className="text-sm text-muted-foreground">
            Build a transaction here, then move the downloaded file to your hardware wallet to sign it.
          </p>
        </div>
      </div>

      {accountsError ? (
        <div className="flex items-center gap-2 text-destructive text-sm">
          <AlertCircle className="h-4 w-4" />
          {accountsError}
        </div>
      ) : accounts.length === 0 ? (
        <div className="text-muted-foreground text-sm">No accounts available.</div>
      ) : (
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-muted-foreground mb-1">From account</label>
            <select
              value={sourceAccount ?? ''}
              onChange={(e) => {
                setSourceAccount(Number(e.target.value));
                invalidate();
              }}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
            >
              {accounts.map((a) => (
                <option key={a.accountNumber} value={a.accountNumber}>
                  {a.accountName} ({a.spendableBalance.toFixed(4)} DCR spendable)
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm text-muted-foreground mb-1">Recipient address</label>
            <input
              type="text"
              value={recipient}
              onChange={(e) => {
                setRecipient(e.target.value);
                invalidate();
              }}
              placeholder="DsXXXX… or TsXXXX…"
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary font-mono text-sm"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-sm text-muted-foreground">Amount (DCR)</label>
              <label className="flex items-center gap-2 text-sm text-muted-foreground">
                <input
                  type="checkbox"
                  checked={sendAll}
                  onChange={(e) => {
                    setSendAll(e.target.checked);
                    invalidate();
                  }}
                  className="accent-primary"
                />
                Send all
              </label>
            </div>
            <input
              type="text"
              inputMode="decimal"
              value={sendAll ? '' : amount}
              onChange={(e) => {
                setAmount(e.target.value);
                invalidate();
              }}
              disabled={sendAll}
              placeholder={sendAll ? 'Entire spendable balance' : '0.00000000'}
              className={`w-full px-3 py-2 rounded-lg bg-background border text-foreground focus:outline-none disabled:opacity-50 ${
                amountError && amount ? 'border-destructive' : 'border-border focus:border-primary'
              }`}
            />
            {amountError && amount && <p className="mt-1 text-xs text-destructive">{amountError}</p>}
          </div>

          <button
            type="button"
            onClick={onBuild}
            disabled={!formReady || building}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {building ? <Loader2 className="h-4 w-4 animate-spin" /> : <FileUp className="h-4 w-4" />}
            {building ? 'Building…' : 'Build unsigned transaction'}
          </button>

          {buildError && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{buildError}</span>
            </div>
          )}

          {construct && (
            <div className="space-y-3 pt-2 border-t border-border/50">
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Amount to recipient</span>
                  <span>{formatDcr(sendAll ? construct.outputsTotalAtoms : amountAtoms)} DCR</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Network fee</span>
                  <span>{formatDcr(construct.feeAtoms)} DCR</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Change returned</span>
                  <span>{formatDcr(construct.changeAtoms)} DCR</span>
                </div>
                <div className="flex justify-between pt-2 border-t border-border/50">
                  <span className="text-muted-foreground">Total debited</span>
                  <span className="font-semibold">{formatDcr(construct.totalDebitedAtoms)} DCR</span>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={onDownload}
                  className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all inline-flex items-center gap-2"
                >
                  <Download className="h-4 w-4" />
                  Download unsigned.dcrtx
                </button>
              </div>
              {construct.signRequestUR && construct.signRequestUR.length <= 1800 ? (
                <div className="flex flex-col items-center gap-2 pt-2">
                  <div className="rounded-lg bg-white p-3">
                    <QRCodeSVG value={construct.signRequestUR.toUpperCase()} size={256} level="M" />
                  </div>
                  <p className="text-xs text-muted-foreground">…or scan this with your Passport</p>
                </div>
              ) : (
                <p className="text-xs text-warning">
                  This transaction is too large for a single QR - use the .dcrtx file.
                </p>
              )}
              <p className="text-xs text-muted-foreground">
                Sign on the device (file or QR), then import the signed file below. Verify the amount and
                recipient on the device before approving.
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

// ImportSignedPanel decodes a signed transaction (a .dcrtx file or pasted hex)
// into a verification preview, then broadcasts it. Broadcasting uses no private
// keys, so it works for watch-only wallets.
const ImportSignedPanel = () => {
  const [file, setFile] = useState<File | null>(null);
  const [pasteText, setPasteText] = useState('');
  const [decoding, setDecoding] = useState(false);
  const [decodeError, setDecodeError] = useState<string | null>(null);
  const [preview, setPreview] = useState<SignedTxPreview | null>(null);
  const [broadcasting, setBroadcasting] = useState(false);
  const [broadcastError, setBroadcastError] = useState<string | null>(null);
  const [result, setResult] = useState<{ txHash: string; alreadyBroadcast: boolean } | null>(null);
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const resetAll = () => {
    setFile(null);
    setPasteText('');
    setDecoding(false);
    setDecodeError(null);
    setPreview(null);
    setBroadcasting(false);
    setBroadcastError(null);
    setResult(null);
  };

  const takeFile = (f: File) => {
    setFile(f);
    setPasteText('');
    setPreview(null);
    setDecodeError(null);
    setResult(null);
  };

  const onPick = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (f) takeFile(f);
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const f = e.dataTransfer.files?.[0];
    if (f) takeFile(f);
  };

  const onDecode = async () => {
    setDecoding(true);
    setDecodeError(null);
    setPreview(null);
    setResult(null);
    try {
      const input = file
        ? { signedTxB64: await fileToBase64(file) }
        : pasteText.trim()
          ? { signedTx: pasteText }
          : null;
      if (!input) {
        setDecodeError('Choose a file or paste a signed transaction.');
        return;
      }
      setPreview(await decodeSignedTransaction(input));
    } catch (err: any) {
      const body = err?.response?.data;
      setDecodeError(typeof body === 'string' ? body : err?.message || 'Could not decode transaction');
    } finally {
      setDecoding(false);
    }
  };

  const onBroadcast = async () => {
    if (!preview) return;
    setBroadcasting(true);
    setBroadcastError(null);
    try {
      const resp = await broadcastSignedTransaction({ signedTx: preview.txHex });
      setResult({ txHash: resp.txHash, alreadyBroadcast: !!resp.alreadyBroadcast });
    } catch (err: any) {
      const body = err?.response?.data;
      setBroadcastError(typeof body === 'string' ? body : err?.message || 'Broadcast failed');
    } finally {
      setBroadcasting(false);
    }
  };

  if (result) {
    return (
      <div className="p-8 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 text-center space-y-4">
        <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-success/10 border border-success/30">
          <Check className="h-8 w-8 text-success" />
        </div>
        <h2 className="text-2xl font-semibold">
          {result.alreadyBroadcast ? 'Already broadcast' : 'Transaction broadcast'}
        </h2>
        <p className="text-muted-foreground">
          {result.alreadyBroadcast
            ? 'This transaction was already known to the network.'
            : 'The signed transaction has been sent to the Decred network.'}
        </p>
        <div className="p-3 rounded-lg bg-background border border-border break-all">
          <p className="text-xs text-muted-foreground mb-1">Transaction ID</p>
          <Link
            to={`/explorer/tx/${result.txHash}`}
            className="font-mono text-sm text-primary hover:underline inline-flex items-center gap-1"
          >
            {result.txHash}
            <ExternalLink className="h-3 w-3" />
          </Link>
        </div>
        <button
          onClick={resetAll}
          className="px-6 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all"
        >
          Broadcast another
        </button>
      </div>
    );
  }

  const externalTotal = preview
    ? preview.outputs.filter((o) => !o.isMine).reduce((s, o) => s + o.amountAtoms, 0)
    : 0;
  const highFee = !!preview?.feeKnown && preview.feeAtoms > HIGH_FEE_ATOMS;

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
      <div className="flex items-center gap-3 mb-4">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <UploadCloud className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h2 className="text-xl font-semibold">2. Import a signed transaction and broadcast</h2>
          <p className="text-sm text-muted-foreground">
            Load the signed file from your hardware wallet, verify the details, then broadcast.
          </p>
        </div>
      </div>

      <input ref={inputRef} type="file" onChange={onPick} className="hidden" />

      {file ? (
        <div className="p-4 rounded-xl bg-background border border-border flex items-start gap-3">
          <FileUp className="h-5 w-5 text-primary mt-0.5 shrink-0" />
          <div className="min-w-0 flex-1">
            <div className="font-medium truncate">{file.name}</div>
            <div className="text-xs text-muted-foreground">{file.size} bytes</div>
          </div>
          <button
            onClick={() => inputRef.current?.click()}
            className="text-xs text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            Replace
          </button>
        </div>
      ) : (
        <div
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={`cursor-pointer rounded-xl border-2 border-dashed p-8 text-center transition-colors ${
            dragging ? 'border-primary bg-primary/5' : 'border-border/60 hover:border-primary/50 hover:bg-muted/10'
          }`}
        >
          <UploadCloud className="mx-auto h-9 w-9 text-muted-foreground" />
          <p className="mt-2 font-medium">Drop the signed file here, or click to choose</p>
          <p className="mt-1 text-xs text-muted-foreground">Accepts the device's .dcrtx file.</p>
        </div>
      )}

      <div className="mt-3">
        <label className="block text-sm text-muted-foreground mb-1">…or paste signed transaction hex</label>
        <textarea
          value={pasteText}
          onChange={(e) => {
            setPasteText(e.target.value);
            setFile(null);
            setPreview(null);
            setDecodeError(null);
            setResult(null);
          }}
          rows={3}
          placeholder="0100000001…"
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary font-mono text-xs resize-y"
        />
      </div>

      <div className="mt-3">
        <button
          type="button"
          onClick={onDecode}
          disabled={decoding || (!file && !pasteText.trim())}
          className="px-4 py-2 rounded-lg border border-border text-sm transition-all inline-flex items-center gap-2 hover:bg-muted/20 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {decoding ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
          {decoding ? 'Decoding…' : 'Decode & verify'}
        </button>
      </div>

      {decodeError && (
        <div className="mt-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{decodeError}</span>
        </div>
      )}

      {preview && (
        <div className="mt-4 space-y-3">
          <div className="p-4 rounded-xl bg-background border border-border space-y-3">
            <div>
              <p className="text-xs text-muted-foreground mb-1">Transaction ID</p>
              <p className="font-mono text-xs break-all">{preview.txid}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground mb-1">Outputs</p>
              <div className="space-y-1">
                {preview.outputs.map((o) => (
                  <div key={o.index} className="flex items-start justify-between gap-3 text-sm">
                    <span className="font-mono text-xs break-all min-w-0">
                      {o.address || scriptClassLabel(o.scriptClass)}
                      {o.isMine && <span className="ml-1 text-success">(your wallet)</span>}
                    </span>
                    <span className="shrink-0">{formatDcr(o.amountAtoms)} DCR</span>
                  </div>
                ))}
              </div>
            </div>
            <div className="space-y-2 text-sm pt-2 border-t border-border/50">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Leaving wallet (external)</span>
                <span className="font-semibold">{formatDcr(externalTotal)} DCR</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Network fee</span>
                <span>{preview.feeKnown ? `${formatDcr(preview.feeAtoms)} DCR` : 'unknown'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Size</span>
                <span>{preview.sizeBytes} bytes</span>
              </div>
            </div>
          </div>

          {highFee && (
            <div className="flex items-start gap-2 p-3 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>This fee is unusually high ({formatDcr(preview.feeAtoms)} DCR). Double-check before broadcasting.</span>
            </div>
          )}
          {!preview.feeKnown && (
            <div className="flex items-start gap-2 p-3 rounded-lg bg-muted/20 border border-border/50 text-sm text-muted-foreground">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>Fee could not be determined (input amounts missing from the transaction).</span>
            </div>
          )}

          {broadcastError && (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{broadcastError}</span>
            </div>
          )}

          <div className="flex justify-end">
            <button
              type="button"
              onClick={onBroadcast}
              disabled={broadcasting}
              className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {broadcasting ? <Loader2 className="h-5 w-5 animate-spin" /> : <Send className="h-5 w-5" />}
              {broadcasting ? 'Broadcasting…' : 'Broadcast'}
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export const OfflineSigningTab = () => {
  return (
    <div className="space-y-6">
      <div className="p-4 rounded-xl bg-muted/10 border border-border/50 flex items-start gap-3">
        <ShieldCheck className="h-5 w-5 text-primary mt-0.5 shrink-0" />
        <p className="text-sm text-muted-foreground">
          Spend from an air-gapped hardware wallet (e.g. Foundation Passport): export an unsigned
          transaction, sign it on the device, then import the signed file here to broadcast. Your keys
          never leave the device.
        </p>
      </div>
      <ExportUnsignedPanel />
      <ImportSignedPanel />
    </div>
  );
};
