import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, Copy, Inbox, Loader2, Search } from 'lucide-react';
import {
  LightningInvoice,
  addLnInvoice,
  listLnInvoices,
  subscribeLnInvoiceEvents,
} from '../../../services/lightningApi';
import { InvoiceRow } from './InvoiceRow';
import { InvoiceDetailsModal } from './InvoiceDetailsModal';

const atomsPerDcr = 1e8;
const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';
const truncHash = (s: string) => (s.length <= 18 ? s : `${s.slice(0, 10)}…${s.slice(-6)}`);

type Filter = 'all' | 'open' | 'settled' | 'expired' | 'canceled';

const fmtExpiry = (endSec: number, nowMs: number): { text: string; expired: boolean } => {
  const diff = endSec * 1000 - nowMs;
  const abs = Math.abs(diff);
  const h = Math.floor(abs / 3600000);
  const m = Math.floor((abs % 3600000) / 60000);
  const s = Math.floor((abs % 60000) / 1000);
  let label: string;
  if (h > 0) label = `${h}h ${m}m`;
  else if (m > 0) label = `${m}m ${s}s`;
  else label = `${s}s`;
  return diff >= 0
    ? { text: `Expires in ${label}`, expired: false }
    : { text: `Expired ${label} ago`, expired: true };
};

export const ReceiveTab = () => {
  // ---- Form state ---------------------------------------------------------
  const [memo, setMemo] = useState('');
  const [valueAtoms, setValueAtoms] = useState(0);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  // ---- Active invoice card -----------------------------------------------
  const [active, setActive] = useState<LightningInvoice | null>(null);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  const [copied, setCopied] = useState<string | null>(null);

  // ---- History list state -------------------------------------------------
  const [invoices, setInvoices] = useState<LightningInvoice[]>([]);
  const [filter, setFilter] = useState<Filter>('all');
  const [search, setSearch] = useState('');
  const [sortDir, setSortDir] = useState<'desc' | 'asc'>('desc');
  const [openDetail, setOpenDetail] = useState<LightningInvoice | null>(null);

  // ---- Initial load + WebSocket subscription -----------------------------
  const upsert = useCallback((inv: LightningInvoice) => {
    setInvoices((prev) => {
      const i = prev.findIndex((p) => p.addIndex === inv.addIndex);
      if (i >= 0) {
        const next = prev.slice();
        next[i] = inv;
        return next;
      }
      return [inv, ...prev];
    });
    setActive((prev) => (prev && prev.addIndex === inv.addIndex ? inv : prev));
    setOpenDetail((prev) => (prev && prev.addIndex === inv.addIndex ? inv : prev));
  }, []);

  const cleanupRef = useRef<(() => void) | null>(null);
  useEffect(() => {
    let cancelled = false;
    listLnInvoices()
      .then((r) => {
        if (!cancelled) setInvoices(r.invoices || []);
      })
      .catch(() => {
        /* keep prior */
      });
    cleanupRef.current = subscribeLnInvoiceEvents(upsert);
    return () => {
      cancelled = true;
      cleanupRef.current?.();
      cleanupRef.current = null;
    };
  }, [upsert]);

  // ---- Create-invoice submit ---------------------------------------------
  const canCreate = !creating && valueAtoms >= 0;
  const onCreate = async () => {
    if (!canCreate) return;
    setCreating(true);
    setCreateError(null);
    try {
      const inv = await addLnInvoice({ memo: memo.trim(), valueAtoms });
      setActive(inv);
      upsert(inv);
      setMemo('');
      setValueAtoms(0);
    } catch (err: any) {
      const body = err?.response?.data;
      setCreateError(typeof body === 'string' ? body : err?.message || 'Create failed');
    } finally {
      setCreating(false);
    }
  };

  const copy = async (value: string, key: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(key);
      window.setTimeout(() => setCopied((c) => (c === key ? null : c)), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  // ---- Filtered/sorted history --------------------------------------------
  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    const out = invoices.filter((inv) => {
      if (filter !== 'all' && inv.status !== filter) return false;
      if (q && !inv.rHashHex.toLowerCase().includes(q) &&
        !(inv.memo || '').toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
    out.sort((a, b) =>
      sortDir === 'desc' ? b.creationDate - a.creationDate : a.creationDate - b.creationDate,
    );
    return out;
  }, [invoices, filter, search, sortDir]);

  // ---- Render -------------------------------------------------------------
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Receive Lightning Payment</h2>
        <p className="text-sm text-muted-foreground">
          Create a BOLT-11 invoice for someone to pay. Open invoices update live as they
          settle, expire, or are canceled.
        </p>
      </div>

      {/* Form */}
      <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label className="block text-sm font-medium mb-1" htmlFor="ln-rcv-amount">
              Amount
            </label>
            <div className="flex items-center gap-2">
              <input
                id="ln-rcv-amount"
                type="text"
                inputMode="decimal"
                value={valueAtoms > 0 ? (valueAtoms / atomsPerDcr).toString() : ''}
                onChange={(e) => {
                  const v = e.target.value.trim();
                  if (v === '') {
                    setValueAtoms(0);
                    return;
                  }
                  if (!/^\d*\.?\d{0,8}$/.test(v)) return;
                  const dcr = parseFloat(v);
                  if (Number.isFinite(dcr)) setValueAtoms(Math.round(dcr * atomsPerDcr));
                }}
                placeholder="0.00000000 (leave blank for open amount)"
                className="flex-1 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              />
              <span className="text-sm text-muted-foreground">DCR</span>
            </div>
          </div>
          <div>
            <label className="block text-sm font-medium mb-1" htmlFor="ln-rcv-memo">
              Description
            </label>
            <input
              id="ln-rcv-memo"
              type="text"
              value={memo}
              onChange={(e) => setMemo(e.target.value.slice(0, 639))}
              placeholder="Message for payer"
              maxLength={639}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
            />
          </div>
        </div>

        {createError && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{createError}</span>
          </div>
        )}

        <div className="flex justify-end">
          <button
            type="button"
            onClick={onCreate}
            disabled={!canCreate}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Inbox className="h-4 w-4" />}
            {creating ? 'Creating…' : 'Create invoice'}
          </button>
        </div>
      </div>

      {/* Active invoice card */}
      {active && (
        <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="text-xs uppercase tracking-wide text-muted-foreground">
                Current invoice
              </div>
              <div className="text-lg font-semibold">
                {active.valueAtoms > 0 ? fmtDcr(active.valueAtoms) : 'Open amount'}
              </div>
              {active.memo && (
                <div className="text-sm text-muted-foreground">{active.memo}</div>
              )}
            </div>
            <div className="text-right">
              <div className="text-xs text-muted-foreground">Status: {active.status}</div>
              <div className="text-xs text-muted-foreground">
                {active.status === 'settled' && active.settleDate
                  ? `Settled ${new Date(active.settleDate * 1000).toLocaleString()}`
                  : fmtExpiry(active.creationDate + active.expiry, now).text}
              </div>
              <div className="text-xs text-muted-foreground font-mono">
                {truncHash(active.rHashHex)}
              </div>
            </div>
          </div>
          <div>
            <div className="text-xs uppercase tracking-wide text-muted-foreground mb-1">
              Payment request
            </div>
            <div className="p-3 rounded-lg bg-background/40 border border-border/60 break-all font-mono text-xs">
              {active.paymentRequest}
            </div>
            <div className="flex justify-end mt-2">
              <button
                type="button"
                onClick={() => copy(active.paymentRequest, 'active-pr')}
                className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
              >
                {copied === 'active-pr' ? (
                  <>
                    <CheckCircle2 className="h-3.5 w-3.5 text-success" /> Copied
                  </>
                ) : (
                  <>
                    <Copy className="h-3.5 w-3.5" /> Copy payment request
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* History */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-lg font-semibold">Lightning invoices</h3>
          <div className="flex items-center gap-2 flex-wrap">
            {(['all', 'open', 'settled', 'expired', 'canceled'] as Filter[]).map((f) => (
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
            placeholder="Search hash or memo"
            className="w-full pl-8 pr-3 py-1.5 rounded-lg bg-background border border-border text-sm focus:outline-none focus:border-primary"
          />
        </div>
        {visible.length === 0 ? (
          <div className="text-sm text-muted-foreground py-6 text-center">
            No invoices match the current filter.
          </div>
        ) : (
          <div className="space-y-1.5">
            {visible.map((inv) => (
              <InvoiceRow
                key={inv.rHashHex + inv.creationDate}
                invoice={inv}
                onClick={() => setOpenDetail(inv)}
              />
            ))}
          </div>
        )}
      </div>

      {openDetail && (
        <InvoiceDetailsModal
          invoice={openDetail}
          onClose={() => setOpenDetail(null)}
          onCanceled={() => setOpenDetail(null)}
        />
      )}
    </div>
  );
};
