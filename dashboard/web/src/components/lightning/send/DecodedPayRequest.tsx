import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, ChevronDown, Copy } from 'lucide-react';
import type { LightningDecodedPayReq } from '../../../services/lightningApi';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';

const trunc = (s: string, head = 10, tail = 8) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;

const fmtExpiry = (endSec: number, nowMs: number): { text: string; expired: boolean } => {
  const endMs = endSec * 1000;
  const diff = endMs - nowMs;
  const abs = Math.abs(diff);
  const m = Math.floor(abs / 60000);
  const s = Math.floor((abs % 60000) / 1000);
  const h = Math.floor(m / 60);
  let label: string;
  if (h > 0) label = `${h}h ${m % 60}m`;
  else if (m > 0) label = `${m}m ${s}s`;
  else label = `${s}s`;
  return diff >= 0
    ? { text: `Expires in ${label}`, expired: false }
    : { text: `Expired ${label} ago`, expired: true };
};

interface Props {
  decoded: LightningDecodedPayReq;
  sendValue: number;
  onSendValueChange: (atoms: number) => void;
  onExpiredChange: (expired: boolean) => void;
}

export const DecodedPayRequest = ({
  decoded,
  sendValue,
  onSendValueChange,
  onExpiredChange,
}: Props) => {
  const [now, setNow] = useState<number>(Date.now());
  const [showDetails, setShowDetails] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const expiryAt = decoded.timestamp + decoded.expiry;
  const expiryInfo = fmtExpiry(expiryAt, now);
  useEffect(() => {
    onExpiredChange(expiryInfo.expired);
  }, [expiryInfo.expired, onExpiredChange]);

  const copy = async (value: string, key: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
      window.setTimeout(() => setCopied((c) => (c === key ? null : c)), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  const Row = ({
    label,
    value,
    copyKey,
    copyValue,
    mono,
  }: {
    label: string;
    value: React.ReactNode;
    copyKey?: string;
    copyValue?: string;
    mono?: boolean;
  }) => (
    <div className="flex items-start justify-between gap-3 py-2 border-b border-border/40 last:border-b-0">
      <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">{label}</span>
      <span className={`text-sm text-right break-all ${mono ? 'font-mono' : ''}`}>
        {value}
        {copyKey && copyValue !== undefined && (
          <button
            onClick={() => copy(copyValue, copyKey)}
            className="ml-2 inline-flex items-center text-muted-foreground hover:text-foreground"
            title="Copy"
            type="button"
          >
            {copied === copyKey ? (
              <CheckCircle2 className="h-3.5 w-3.5 text-success" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </button>
        )}
      </span>
    </div>
  );

  return (
    <div className="p-4 rounded-xl bg-background/40 border border-border/60 space-y-1">
      <div className="flex items-start justify-between gap-3 py-2 border-b border-border/40">
        <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">Amount</span>
        {decoded.numAtoms > 0 ? (
          <span className="text-base font-semibold text-right">{fmtDcr(decoded.numAtoms)}</span>
        ) : (
          <span className="flex items-center gap-2">
            <input
              type="text"
              inputMode="decimal"
              autoFocus
              value={sendValue > 0 ? (sendValue / atomsPerDcr).toString() : ''}
              onChange={(e) => {
                const v = e.target.value.trim();
                if (v === '') {
                  onSendValueChange(0);
                  return;
                }
                if (!/^\d*\.?\d{0,8}$/.test(v)) return;
                const dcr = parseFloat(v);
                if (Number.isFinite(dcr)) {
                  onSendValueChange(Math.round(dcr * atomsPerDcr));
                }
              }}
              placeholder="0.00000000"
              className="w-40 px-3 py-1.5 rounded-lg bg-background border border-border text-foreground text-right focus:outline-none focus:border-primary"
            />
            <span className="text-sm text-muted-foreground">DCR</span>
          </span>
        )}
      </div>
      <Row label="Destination" value={trunc(decoded.destination)} copyKey="dest" copyValue={decoded.destination} mono />
      <Row
        label="Expiration"
        value={
          <span className={expiryInfo.expired ? 'text-destructive' : 'text-foreground'}>
            {expiryInfo.expired ? (
              <span className="inline-flex items-center gap-1">
                <AlertCircle className="h-3.5 w-3.5" /> {expiryInfo.text}
              </span>
            ) : (
              expiryInfo.text
            )}
          </span>
        }
      />
      {decoded.description && <Row label="Description" value={decoded.description} />}
      <Row
        label="Payment hash"
        value={trunc(decoded.paymentHash)}
        copyKey="hash"
        copyValue={decoded.paymentHash}
        mono
      />

      <div className="pt-2">
        <button
          type="button"
          onClick={() => setShowDetails((v) => !v)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ChevronDown
            className={`h-3.5 w-3.5 transition-transform ${showDetails ? 'rotate-180' : ''}`}
          />
          {showDetails ? 'Hide' : 'Show'} invoice details
        </button>
      </div>
      {showDetails && (
        <div className="pt-1">
          <Row label="CLTV expiry" value={String(decoded.cltvExpiry)} />
          {decoded.fallbackAddr && (
            <Row label="Fallback addr" value={decoded.fallbackAddr} mono />
          )}
          {decoded.paymentAddr && (
            <Row label="Payment addr" value={trunc(decoded.paymentAddr)} mono />
          )}
        </div>
      )}
    </div>
  );
};
