import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ArrowDown, Check, Send } from 'lucide-react';
import {
  AccountInfo,
  ConstructTransactionResponse,
  constructTransaction,
  getAccounts,
  getNextAddress,
} from '../../services/api';
import { nextAddressCache } from '../../services/nextAddressCache';
import { SendPassphraseModal } from '../wallet/SendPassphraseModal';

const MAX_DCR = 21_000_000;
const CONSTRUCT_DEBOUNCE_MS = 500;

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

interface Props {
  changeAccount: number;
}

export const SendToUnmixedCard = ({ changeAccount }: Props) => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [sourceAccount, setSourceAccount] = useState<number | null>(null);
  const [destAddress, setDestAddress] = useState<string | null>(null);
  const [deriveError, setDeriveError] = useState<string | null>(null);
  const [amount, setAmount] = useState('');
  const [sendAll, setSendAll] = useState(false);

  const [construct, setConstruct] = useState<ConstructTransactionResponse | null>(null);
  const [constructError, setConstructError] = useState<string | null>(null);
  const [constructing, setConstructing] = useState(false);

  const [modalOpen, setModalOpen] = useState(false);
  const [successTxHash, setSuccessTxHash] = useState<string | null>(null);
  const [topLevelError, setTopLevelError] = useState<string | null>(null);

  const constructTimerRef = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await getAccounts();
        if (cancelled) return;
        const visible = data
          .filter((a) =>
            a.accountName !== 'imported' &&
            a.accountNumber !== 2147483647 &&
            a.accountName !== 'mixed' &&
            a.accountName !== 'unmixed',
          )
          .sort((a, b) => a.accountNumber - b.accountNumber);
        setAccounts(visible);
        if (visible.length > 0) setSourceAccount(visible[0].accountNumber);
      } catch {
        // surface via constructError later if user tries to send
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const cached = nextAddressCache.get(changeAccount);
    if (cached) {
      setDestAddress(cached);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const r = await getNextAddress(changeAccount);
        if (cancelled) return;
        nextAddressCache.set(changeAccount, 0, r.address);
        setDestAddress(r.address);
      } catch (err: any) {
        if (cancelled) return;
        const body = err?.response?.data;
        setDeriveError(typeof body === 'string' ? body : err?.message || 'Failed to derive unmixed address');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [changeAccount]);

  const amountError = sendAll ? null : validateAmount(amount);
  const amountAtoms = sendAll ? 0 : Math.round(parseFloat(amount || '0') * 1e8);

  const formReady = useMemo(() => {
    return (
      sourceAccount !== null &&
      destAddress !== null &&
      (sendAll || (!amountError && amountAtoms > 0))
    );
  }, [sourceAccount, destAddress, amountError, amountAtoms, sendAll]);

  useEffect(() => {
    if (constructTimerRef.current) window.clearTimeout(constructTimerRef.current);
    setConstructError(null);
    if (!formReady || sourceAccount === null || destAddress === null) {
      setConstruct(null);
      return;
    }
    constructTimerRef.current = window.setTimeout(async () => {
      setConstructing(true);
      try {
        const resp = await constructTransaction({
          sourceAccount,
          address: destAddress,
          amountAtoms,
          sendAll,
        });
        setConstruct(resp);
      } catch (err: any) {
        const body = err?.response?.data;
        setConstructError(typeof body === 'string' ? body : err?.message || 'Failed to construct transaction');
        setConstruct(null);
      } finally {
        setConstructing(false);
      }
    }, CONSTRUCT_DEBOUNCE_MS);
    return () => {
      if (constructTimerRef.current) window.clearTimeout(constructTimerRef.current);
    };
  }, [formReady, sourceAccount, destAddress, amountAtoms, sendAll]);

  const handleSuccess = (txHash: string) => {
    // Cached address has just been consumed on-chain — drop it so the next
    // mount derives a fresh index.
    nextAddressCache.invalidate(changeAccount);
    setSuccessTxHash(txHash);
    setModalOpen(false);
    setAmount('');
    setSendAll(false);
    setConstruct(null);
  };

  if (successTxHash) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 text-center space-y-3">
        <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-success/10 border border-success/30">
          <Check className="h-6 w-6 text-success" />
        </div>
        <p className="font-semibold">Funds sent to unmixed account</p>
        <p className="font-mono text-xs break-all text-muted-foreground">{successTxHash}</p>
        <button
          onClick={() => setSuccessTxHash(null)}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm"
        >
          Send another
        </button>
      </div>
    );
  }

  return (
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <ArrowDown className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">Send to unmixed</h3>
      </div>
      <p className="text-xs text-muted-foreground">
        Move funds from any account into the unmixed account so the mixer has something to work on.
      </p>

      {deriveError && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{deriveError}</span>
        </div>
      )}

      {accounts.length === 0 ? (
        <p className="text-sm text-muted-foreground">No source accounts available.</p>
      ) : (
        <div className="space-y-3">
          <div>
            <label className="block text-xs text-muted-foreground mb-1">From account</label>
            <select
              value={sourceAccount ?? ''}
              onChange={(e) => setSourceAccount(Number(e.target.value))}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
            >
              {accounts.map((a) => (
                <option key={a.accountNumber} value={a.accountNumber}>
                  {a.accountName} ({a.spendableBalance.toFixed(4)} DCR)
                </option>
              ))}
            </select>
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-xs text-muted-foreground">Amount (DCR)</label>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={sendAll}
                  onChange={(e) => setSendAll(e.target.checked)}
                  className="accent-primary"
                />
                Send all
              </label>
            </div>
            <input
              type="text"
              inputMode="decimal"
              value={sendAll ? '' : amount}
              onChange={(e) => setAmount(e.target.value)}
              disabled={sendAll}
              placeholder={sendAll ? 'Entire spendable balance' : '0.00000000'}
              className={`w-full px-3 py-2 rounded-lg bg-background border text-foreground text-sm focus:outline-none disabled:opacity-50 ${
                !sendAll && amountError && amount ? 'border-destructive' : 'border-border focus:border-primary'
              }`}
            />
            {!sendAll && amountError && amount && (
              <p className="mt-1 text-xs text-destructive">{amountError}</p>
            )}
          </div>
        </div>
      )}

      {(constructing || construct || constructError) && (
        <div className="p-3 rounded-lg bg-background border border-border text-sm space-y-1">
          {constructing ? (
            <p className="text-muted-foreground text-xs">Building transaction…</p>
          ) : constructError ? (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span>{constructError}</span>
            </div>
          ) : construct ? (
            <>
              <div className="flex justify-between text-xs">
                <span className="text-muted-foreground">Network fee</span>
                <span>{formatDcr(construct.feeAtoms)} DCR</span>
              </div>
              <div className="flex justify-between text-xs">
                <span className="text-muted-foreground">Total debited</span>
                <span className="font-semibold">{formatDcr(construct.inputsTotalAtoms)} DCR</span>
              </div>
            </>
          ) : null}
        </div>
      )}

      {topLevelError && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{topLevelError}</span>
        </div>
      )}

      <div className="flex justify-end">
        <button
          onClick={() => {
            setTopLevelError(null);
            setModalOpen(true);
          }}
          disabled={!construct || constructing || sourceAccount === null}
          className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Send className="h-4 w-4" />
          Send to unmixed
        </button>
      </div>

      {construct && sourceAccount !== null && destAddress && (
        <SendPassphraseModal
          isOpen={modalOpen}
          sourceAccount={sourceAccount}
          recipient={destAddress}
          amountAtoms={sendAll ? construct.outputsTotalAtoms : amountAtoms}
          feeAtoms={construct.feeAtoms}
          unsignedTxHex={construct.unsignedTxHex}
          onClose={() => setModalOpen(false)}
          onSuccess={handleSuccess}
          onWatchOnly={(msg) => {
            setModalOpen(false);
            setTopLevelError(msg);
          }}
        />
      )}
    </div>
  );
};
