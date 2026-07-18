import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ClipboardPaste, Loader2, Search, Send, Trash2 } from 'lucide-react';
import {
  LightningDecodedPayReq,
  LightningPayment,
  decodeLnPayReq,
  listLnPayments,
  lnFeeLimitAtoms,
  streamLnPayment,
} from '../../../services/lightningApi';
import { DecodedPayRequest } from './DecodedPayRequest';
import { PaymentRow } from './PaymentRow';
import { PaymentDetailsModal } from './PaymentDetailsModal';

type Filter = 'all' | 'confirmed' | 'pending' | 'failed';

export const SendTab = () => {
  const [payReq, setPayReq] = useState('');
  const [decoded, setDecoded] = useState<LightningDecodedPayReq | null>(null);
  const [decodeError, setDecodeError] = useState<string | null>(null);
  const [decoding, setDecoding] = useState(false);
  const [expired, setExpired] = useState(false);
  const [sendValue, setSendValue] = useState(0);

  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const [currentSnap, setCurrentSnap] = useState<LightningPayment | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  const [payments, setPayments] = useState<LightningPayment[]>([]);
  const [filter, setFilter] = useState<Filter>('all');
  const [search, setSearch] = useState('');
  const [sortDir, setSortDir] = useState<'desc' | 'asc'>('desc');
  const [openDetail, setOpenDetail] = useState<LightningPayment | null>(null);

  // ---- Decode-on-change (Decrediton: useEffect on payRequest change) -------
  useEffect(() => {
    const trimmed = payReq.trim();
    if (!trimmed) {
      setDecoded(null);
      setDecodeError(null);
      setDecoding(false);
      setExpired(false);
      return;
    }
    let cancelled = false;
    setDecoding(true);
    const timer = window.setTimeout(() => {
      decodeLnPayReq(trimmed)
        .then((r) => {
          if (cancelled) return;
          setDecoded(r);
          setDecodeError(null);
        })
        .catch((err: any) => {
          if (cancelled) return;
          setDecoded(null);
          const msg =
            typeof err?.response?.data === 'string'
              ? err.response.data
              : err?.message || 'Invalid invoice';
          setDecodeError(msg);
        })
        .finally(() => {
          if (!cancelled) setDecoding(false);
        });
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [payReq]);

  // ---- Payment history (poll every 10s while mounted) ----------------------
  const reloadPayments = useCallback(async () => {
    try {
      const { payments } = await listLnPayments();
      setPayments(payments || []);
    } catch {
      /* keep prior list on transient error */
    }
  }, []);

  useEffect(() => {
    reloadPayments();
    const id = window.setInterval(reloadPayments, 10000);
    return () => window.clearInterval(id);
  }, [reloadPayments]);

  // Clean up the WebSocket on unmount.
  useEffect(() => () => cleanupRef.current?.(), []);

  // ---- Send handler --------------------------------------------------------
  const canSend = useMemo(() => {
    if (!decoded || expired || sending) return false;
    if (decoded.numAtoms === 0 && sendValue <= 0) return false;
    return true;
  }, [decoded, expired, sending, sendValue]);

  const onSend = () => {
    if (!decoded || !canSend) return;
    setSendError(null);
    setSending(true);
    setCurrentSnap({
      paymentHash: decoded.paymentHash,
      destination: decoded.destination,
      valueAtoms: decoded.numAtoms > 0 ? decoded.numAtoms : sendValue,
      feeAtoms: 0,
      creationDate: Math.floor(Date.now() / 1000),
      status: 'pending',
      paymentRequest: payReq.trim(),
      description: decoded.description,
    });
    cleanupRef.current = streamLnPayment(
      {
        payReq: payReq.trim(),
        amt: decoded.numAtoms === 0 ? sendValue : undefined,
        feeLimitAtoms: lnFeeLimitAtoms(decoded.numAtoms > 0 ? decoded.numAtoms : sendValue),
      },
      (snap) => {
        setCurrentSnap(snap);
        if (snap.status !== 'pending') {
          setSending(false);
        }
      },
      (msg) => {
        setSendError(msg);
        setSending(false);
      },
      () => {
        setSending(false);
        cleanupRef.current = null;
        reloadPayments();
      },
    );
  };

  const clearForm = () => {
    cleanupRef.current?.();
    cleanupRef.current = null;
    setPayReq('');
    setDecoded(null);
    setDecodeError(null);
    setSendValue(0);
    setCurrentSnap(null);
    setSendError(null);
    setSending(false);
  };

  // ---- Filtered/sorted history view ----------------------------------------
  const mergedList = useMemo(() => {
    // Surface the in-flight current send (if not yet in the polled list)
    // at the top, so the user sees a row appear immediately on submit.
    const items = [...payments];
    if (currentSnap && !items.some((p) => p.paymentHash === currentSnap.paymentHash)) {
      items.unshift(currentSnap);
    } else if (currentSnap) {
      // Already in the list — let the snapshot's status override the
      // backend snapshot until the next poll lands.
      const i = items.findIndex((p) => p.paymentHash === currentSnap.paymentHash);
      if (i >= 0) items[i] = currentSnap;
    }
    const q = search.trim().toLowerCase();
    const out = items.filter((p) => {
      if (filter !== 'all' && p.status !== filter) return false;
      if (q && !p.paymentHash.toLowerCase().includes(q) &&
        !(p.description || '').toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
    out.sort((a, b) =>
      sortDir === 'desc' ? b.creationDate - a.creationDate : a.creationDate - b.creationDate,
    );
    return out;
  }, [payments, currentSnap, filter, search, sortDir]);

  // ---- Render --------------------------------------------------------------
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-xl font-semibold">Send Lightning Payment</h2>
        <p className="text-sm text-muted-foreground">
          Paste a BOLT-11 invoice to pay it. The invoice is decoded on the fly and the
          payment streams its progress back from dcrlnd.
        </p>
      </div>

      {/* Invoice input */}
      <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-4">
        <label className="block text-sm font-medium" htmlFor="ln-payreq">
          Lightning payment request
        </label>
        <div className="relative">
          <textarea
            id="ln-payreq"
            value={payReq}
            onChange={(e) => setPayReq(e.target.value)}
            placeholder="lnbc..."
            rows={3}
            spellCheck={false}
            className="w-full px-3 py-2 pr-10 rounded-lg bg-background border border-border text-foreground font-mono text-xs focus:outline-none focus:border-primary resize-y"
          />
          <div className="absolute right-2 top-2 flex gap-1">
            {payReq ? (
              <button
                type="button"
                onClick={clearForm}
                className="text-muted-foreground hover:text-foreground"
                title="Clear"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            ) : (
              <button
                type="button"
                onClick={async () => {
                  try {
                    const v = await navigator.clipboard.readText();
                    if (v) setPayReq(v.trim());
                  } catch {
                    /* clipboard unavailable */
                  }
                }}
                className="text-muted-foreground hover:text-foreground"
                title="Paste"
              >
                <ClipboardPaste className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        {decoding && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" /> Decoding…
          </div>
        )}
        {decodeError && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{decodeError}</span>
          </div>
        )}

        {decoded && (
          <DecodedPayRequest
            decoded={decoded}
            sendValue={sendValue}
            onSendValueChange={setSendValue}
            onExpiredChange={setExpired}
          />
        )}

        {sendError && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{sendError}</span>
          </div>
        )}

        <div className="flex justify-end">
          <button
            type="button"
            onClick={onSend}
            disabled={!canSend}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {sending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Send className="h-4 w-4" />
            )}
            {sending ? 'Sending…' : 'Send Payment'}
          </button>
        </div>
      </div>

      {/* History */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-lg font-semibold">Lightning payments</h3>
          <div className="flex items-center gap-2">
            {(['all', 'confirmed', 'pending', 'failed'] as Filter[]).map((f) => (
              <button
                key={f}
                type="button"
                onClick={() => setFilter(f)}
                className={`px-2.5 py-1 rounded-md text-xs capitalize transition-colors ${
                  filter === f
                    ? 'bg-primary/20 text-primary font-semibold'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {f}
              </button>
            ))}
            <button
              type="button"
              onClick={() => setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))}
              className="px-2.5 py-1 rounded-md text-xs text-muted-foreground hover:text-foreground"
              title="Toggle sort direction"
            >
              {sortDir === 'desc' ? 'Newest first' : 'Oldest first'}
            </button>
          </div>
        </div>
        <div className="relative">
          <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search hash or description"
            className="w-full pl-8 pr-3 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
          />
        </div>
        {mergedList.length === 0 ? (
          <div className="text-sm text-muted-foreground py-6 text-center">
            No payments match the current filter.
          </div>
        ) : (
          <div className="space-y-1.5">
            {mergedList.map((p) => (
              <PaymentRow
                key={p.paymentHash + p.creationDate}
                payment={p}
                onClick={() => setOpenDetail(p)}
              />
            ))}
          </div>
        )}
      </div>

      {openDetail && <PaymentDetailsModal payment={openDetail} onClose={() => setOpenDetail(null)} />}
    </div>
  );
};
