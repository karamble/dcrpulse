import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, Check, ExternalLink, Send, X } from 'lucide-react';
import {
  AccountInfo,
  ConstructTransactionResponse,
  PrivacyStatus,
  constructTransaction,
  getAccounts,
  getAutobuyerStatus,
  getNextAddress,
  getPrivacyStatus,
  validateAddress,
} from '../../services/api';
import { nextAddressCache } from '../../services/nextAddressCache';
import { SendPassphraseModal } from '../wallet/SendPassphraseModal';

const MAX_DCR = 21_000_000;
const VALIDATE_DEBOUNCE_MS = 400;
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

type AddressCheck =
  | { state: 'idle' }
  | { state: 'checking' }
  | { state: 'valid'; isMine: boolean }
  | { state: 'invalid'; message: string };

export const SendTab = () => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [accountsError, setAccountsError] = useState<string | null>(null);
  const [sourceAccount, setSourceAccount] = useState<number | null>(null);

  const [mode, setMode] = useState<'external' | 'internal'>('external');
  const [destAccount, setDestAccount] = useState<number | null>(null);
  const [derivingAddress, setDerivingAddress] = useState(false);
  const [deriveError, setDeriveError] = useState<string | null>(null);

  const [recipient, setRecipient] = useState('');
  const [addrCheck, setAddrCheck] = useState<AddressCheck>({ state: 'idle' });

  const [amount, setAmount] = useState('');
  const [sendAll, setSendAll] = useState(false);

  const [construct, setConstruct] = useState<ConstructTransactionResponse | null>(null);
  const [constructError, setConstructError] = useState<string | null>(null);
  const [constructing, setConstructing] = useState(false);

  const [modalOpen, setModalOpen] = useState(false);
  const [successTxHash, setSuccessTxHash] = useState<string | null>(null);
  const [topLevelError, setTopLevelError] = useState<string | null>(null);
  // A regular send conflicts with the running mixer/autobuyer (both spend the
  // wallet's UTXOs), so block sending while either is active. Mirrors Decrediton.
  const [spendBlocked, setSpendBlocked] = useState(false);
  // Privacy config drives the source-account filter: when mixing is set up the
  // unmixed (change) account is receive-only and must not be spent from.
  const [privacy, setPrivacy] = useState<PrivacyStatus | null>(null);

  const addrTimerRef = useRef<number | null>(null);
  const constructTimerRef = useRef<number | null>(null);

  // loadAccounts refreshes the account list + balances, preserving the user's
  // current source-account selection (only defaulting to the first account when
  // none is selected yet). Used on mount and after a send so balances aren't stale.
  const loadAccounts = async () => {
    try {
      const data = await getAccounts();
      const visible = data
        .filter((a) => a.accountName !== 'imported')
        .sort((a, b) => a.accountNumber - b.accountNumber);
      setAccounts(visible);
      setAccountsError(null);
      setSourceAccount((prev) => (prev ?? (visible.length > 0 ? visible[0].accountNumber : null)));
    } catch (err) {
      console.error('Failed to load accounts:', err);
      setAccountsError('Failed to load accounts');
    }
  };

  useEffect(() => {
    loadAccounts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Poll mixer/autobuyer state so the send block clears once the user stops them.
  useEffect(() => {
    let cancelled = false;
    const check = async () => {
      const [priv, ab] = await Promise.all([
        getPrivacyStatus().catch(() => null),
        getAutobuyerStatus().catch(() => null),
      ]);
      if (cancelled) return;
      setPrivacy(priv);
      setSpendBlocked(!!priv?.mixerRunning || !!ab?.running);
    };
    check();
    const id = window.setInterval(check, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  // When privacy is configured the unmixed (change) account is receive-only:
  // spending it directly would defeat mixing, so drop it from the source list.
  // It stays available as a destination (internal mode) so it can still receive.
  const sourceAccounts = useMemo(() => {
    if (privacy?.configured && privacy.changeAccount !== undefined) {
      return accounts.filter((a) => a.accountNumber !== privacy.changeAccount);
    }
    return accounts;
  }, [accounts, privacy]);

  // Keep the selection valid: pick the first allowed account when none is set,
  // and move off the unmixed account if it became disallowed (privacy was just
  // enabled, or it was the default before the privacy status loaded).
  useEffect(() => {
    if (sourceAccount !== null && sourceAccounts.some((a) => a.accountNumber === sourceAccount)) {
      return;
    }
    const next = sourceAccounts.length > 0 ? sourceAccounts[0].accountNumber : null;
    setSourceAccount(next);
    setConstruct(null);
    setConstructError(null);
  }, [sourceAccounts, sourceAccount]);

  const amountError = sendAll ? null : validateAmount(amount);
  const amountAtoms = sendAll ? 0 : Math.round(parseFloat(amount || '0') * 1e8);

  useEffect(() => {
    if (mode !== 'external') return;
    if (addrTimerRef.current) window.clearTimeout(addrTimerRef.current);
    if (!recipient.trim()) {
      setAddrCheck({ state: 'idle' });
      return;
    }
    setAddrCheck({ state: 'checking' });
    addrTimerRef.current = window.setTimeout(async () => {
      try {
        const r = await validateAddress(recipient.trim());
        if (r.isValid) {
          setAddrCheck({ state: 'valid', isMine: r.isMine });
        } else {
          setAddrCheck({ state: 'invalid', message: 'Invalid address for this network' });
        }
      } catch {
        setAddrCheck({ state: 'invalid', message: 'Could not validate address' });
      }
    }, VALIDATE_DEBOUNCE_MS);
    return () => {
      if (addrTimerRef.current) window.clearTimeout(addrTimerRef.current);
    };
  }, [recipient, mode]);

  useEffect(() => {
    if (mode !== 'internal') return;
    if (destAccount === null) {
      setRecipient('');
      setAddrCheck({ state: 'idle' });
      return;
    }
    const cached = nextAddressCache.get(destAccount);
    if (cached) {
      setRecipient(cached);
      setAddrCheck({ state: 'valid', isMine: true });
      setDerivingAddress(false);
      setDeriveError(null);
      return;
    }
    let cancelled = false;
    setDerivingAddress(true);
    setDeriveError(null);
    setAddrCheck({ state: 'checking' });
    (async () => {
      try {
        const r = await getNextAddress(destAccount);
        if (cancelled) return;
        nextAddressCache.set(destAccount, 0, r.address);
        setRecipient(r.address);
        setAddrCheck({ state: 'valid', isMine: true });
      } catch (err: any) {
        if (cancelled) return;
        const body = err?.response?.data;
        const msg = typeof body === 'string' ? body : err?.message || 'Failed to derive destination address';
        setDeriveError(msg);
        setRecipient('');
        setAddrCheck({ state: 'invalid', message: msg });
      } finally {
        if (!cancelled) setDerivingAddress(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mode, destAccount]);

  const formReady = useMemo(() => {
    return (
      sourceAccount !== null &&
      addrCheck.state === 'valid' &&
      (sendAll || (!amountError && amountAtoms > 0))
    );
  }, [sourceAccount, addrCheck, amountError, amountAtoms, sendAll]);

  useEffect(() => {
    if (constructTimerRef.current) window.clearTimeout(constructTimerRef.current);
    setConstructError(null);
    if (!formReady || sourceAccount === null) {
      setConstruct(null);
      return;
    }
    constructTimerRef.current = window.setTimeout(async () => {
      setConstructing(true);
      try {
        const resp = await constructTransaction({
          sourceAccount,
          address: recipient.trim(),
          amountAtoms,
          sendAll,
        });
        setConstruct(resp);
      } catch (err: any) {
        const body = err?.response?.data;
        const msg = typeof body === 'string' ? body : err?.message || 'Failed to construct transaction';
        setConstructError(msg);
        setConstruct(null);
      } finally {
        setConstructing(false);
      }
    }, CONSTRUCT_DEBOUNCE_MS);
    return () => {
      if (constructTimerRef.current) window.clearTimeout(constructTimerRef.current);
    };
  }, [formReady, sourceAccount, recipient, amountAtoms, sendAll]);

  const handleAccountChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newSource = Number(e.target.value);
    setSourceAccount(newSource);
    setConstruct(null);
    setConstructError(null);
    if (mode === 'internal' && destAccount === newSource) {
      setDestAccount(null);
      setRecipient('');
      setAddrCheck({ state: 'idle' });
    }
  };

  const handleModeChange = (next: 'external' | 'internal') => {
    if (next === mode) return;
    setMode(next);
    setRecipient('');
    setAddrCheck({ state: 'idle' });
    setDestAccount(null);
    setDeriveError(null);
    setConstruct(null);
    setConstructError(null);
  };

  const handleDestChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setDestAccount(Number(e.target.value));
    setConstruct(null);
    setConstructError(null);
  };

  const handleSendAllChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSendAll(e.target.checked);
    setConstruct(null);
  };

  const handleSuccess = (txHash: string) => {
    if (mode === 'internal' && destAccount !== null) {
      nextAddressCache.invalidate(destAccount);
    }
    setSuccessTxHash(txHash);
    setModalOpen(false);
    setRecipient('');
    setAmount('');
    setSendAll(false);
    setConstruct(null);
    setDestAccount(null);
    setAddrCheck({ state: 'idle' });
    // Refresh balances in the background so the form is up to date when the
    // user returns via "Send another".
    loadAccounts();
  };

  const handleWatchOnly = (msg: string) => {
    setModalOpen(false);
    setTopLevelError(msg);
  };

  if (successTxHash) {
    return (
      <div className="p-8 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 text-center space-y-4">
        <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-success/10 border border-success/30">
          <Check className="h-8 w-8 text-success" />
        </div>
        <h2 className="text-2xl font-semibold">Transaction broadcast</h2>
        <p className="text-muted-foreground">
          The transaction has been sent to the Decred network.
        </p>
        <div className="p-3 rounded-lg bg-background border border-border break-all">
          <p className="text-xs text-muted-foreground mb-1">Transaction ID</p>
          <Link
            to={`/explorer/tx/${successTxHash}`}
            className="font-mono text-sm text-primary hover:underline inline-flex items-center gap-1"
          >
            {successTxHash}
            <ExternalLink className="h-3 w-3" />
          </Link>
        </div>
        <button
          onClick={() => {
            loadAccounts();
            setSuccessTxHash(null);
          }}
          className="px-6 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all"
        >
          Send another
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Send className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Send</h2>
            <p className="text-sm text-muted-foreground">
              Construct, sign, and broadcast a Decred transaction.
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
                onChange={handleAccountChange}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              >
                {sourceAccounts.map((a) => (
                  <option key={a.accountNumber} value={a.accountNumber}>
                    {a.accountName} ({a.spendableBalance.toFixed(4)} DCR spendable)
                  </option>
                ))}
              </select>
              {privacy?.configured &&
                privacy.mixedAccount !== undefined &&
                sourceAccount === privacy.mixedAccount && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    Change from a mixed-account send returns to the unmixed account to be
                    re-mixed, so the full input is moved out of the mixed account.
                  </p>
                )}
            </div>

            <div>
              <div className="flex items-center gap-1 mb-2 p-1 rounded-lg bg-background border border-border w-fit">
                <button
                  type="button"
                  onClick={() => handleModeChange('external')}
                  className={`px-3 py-1 rounded-md text-sm transition-colors ${
                    mode === 'external'
                      ? 'bg-primary/20 text-primary font-semibold'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  External address
                </button>
                <button
                  type="button"
                  onClick={() => handleModeChange('internal')}
                  disabled={accounts.length < 2}
                  className={`px-3 py-1 rounded-md text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                    mode === 'internal'
                      ? 'bg-primary/20 text-primary font-semibold'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  Internal account
                </button>
              </div>
              {mode === 'external' ? (
                <>
                  <label className="block text-sm text-muted-foreground mb-1">Recipient address</label>
                  <input
                    type="text"
                    value={recipient}
                    onChange={(e) => setRecipient(e.target.value)}
                    placeholder="DsXXXX… or TsXXXX…"
                    className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary font-mono text-sm"
                  />
                </>
              ) : (
                <>
                  <label className="block text-sm text-muted-foreground mb-1">Destination account</label>
                  <select
                    value={destAccount ?? ''}
                    onChange={handleDestChange}
                    className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
                  >
                    <option value="" disabled>
                      Select destination account…
                    </option>
                    {accounts
                      .filter((a) => a.accountNumber !== sourceAccount && a.accountName !== 'mixed')
                      .map((a) => (
                        <option key={a.accountNumber} value={a.accountNumber}>
                          {a.accountName}
                        </option>
                      ))}
                  </select>
                  {derivingAddress && (
                    <p className="mt-1 text-xs text-muted-foreground">Deriving address…</p>
                  )}
                  {!derivingAddress && recipient && (
                    <div className="mt-2 p-2 rounded-lg bg-background border border-border">
                      <p className="text-xs text-muted-foreground mb-1">Will send to</p>
                      <p className="font-mono text-xs break-all">{recipient}</p>
                    </div>
                  )}
                  {deriveError && (
                    <p className="mt-1 text-xs text-destructive flex items-center gap-1">
                      <X className="h-3 w-3" />
                      {deriveError}
                    </p>
                  )}
                </>
              )}
              {mode === 'external' && addrCheck.state === 'valid' && (
                <p className="mt-1 text-xs text-success flex items-center gap-1">
                  <Check className="h-3 w-3" />
                  Valid address{addrCheck.isMine ? ' - belongs to your wallet' : ''}
                </p>
              )}
              {mode === 'external' && addrCheck.state === 'invalid' && (
                <p className="mt-1 text-xs text-destructive flex items-center gap-1">
                  <X className="h-3 w-3" />
                  {addrCheck.message}
                </p>
              )}
            </div>

            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="block text-sm text-muted-foreground">Amount (DCR)</label>
                <label className="flex items-center gap-2 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={sendAll}
                    onChange={handleSendAllChange}
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
                className={`w-full px-3 py-2 rounded-lg bg-background border text-foreground focus:outline-none disabled:opacity-50 ${
                  !sendAll && amountError && amount ? 'border-destructive' : 'border-border focus:border-primary'
                }`}
              />
              {!sendAll && amountError && amount && (
                <p className="mt-1 text-xs text-destructive">{amountError}</p>
              )}
            </div>
          </div>
        )}
      </div>

      {(constructing || construct || constructError) && (
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <h3 className="text-lg font-semibold mb-3">Preview</h3>
          {constructing ? (
            <p className="text-muted-foreground text-sm">Building transaction…</p>
          ) : constructError ? (
            <div className="flex items-start gap-2 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{constructError}</span>
            </div>
          ) : construct ? (
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Amount to recipient</span>
                <span>
                  {sendAll
                    ? `${formatDcr(construct.outputsTotalAtoms)} DCR`
                    : `${formatDcr(amountAtoms)} DCR`}
                </span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Network fee</span>
                <span>{formatDcr(construct.feeAtoms)} DCR</span>
              </div>
              <div className="flex justify-between pt-2 border-t border-border/50">
                <span className="text-muted-foreground">Total debited</span>
                <span className="font-semibold">
                  {formatDcr(construct.inputsTotalAtoms)} DCR
                </span>
              </div>
              <div className="text-xs text-muted-foreground pt-1">
                Estimated signed size: {construct.estimatedSignedSize} bytes
              </div>
            </div>
          ) : null}
        </div>
      )}

      {topLevelError && (
        <div className="flex items-start gap-2 p-4 rounded-lg bg-destructive/10 border border-destructive/30 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{topLevelError}</span>
        </div>
      )}

      {spendBlocked && (
        <div className="flex items-start gap-2 p-4 rounded-lg bg-warning/10 border border-warning/30 text-sm text-warning">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>
            Stop the privacy mixer or ticket autobuyer (on the Privacy / Staking tabs) before
            sending a transaction.
          </span>
        </div>
      )}

      <div className="flex justify-end">
        <button
          onClick={() => {
            setTopLevelError(null);
            setModalOpen(true);
          }}
          disabled={!construct || constructing || sourceAccount === null || spendBlocked}
          title={spendBlocked ? 'Stop the privacy mixer or ticket autobuyer first' : undefined}
          className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Send className="h-5 w-5" />
          Send
        </button>
      </div>

      {construct && sourceAccount !== null && (
        <SendPassphraseModal
          isOpen={modalOpen}
          sourceAccount={sourceAccount}
          recipient={recipient.trim()}
          amountAtoms={sendAll ? construct.outputsTotalAtoms : amountAtoms}
          feeAtoms={construct.feeAtoms}
          unsignedTxHex={construct.unsignedTxHex}
          onClose={() => setModalOpen(false)}
          onSuccess={handleSuccess}
          onWatchOnly={handleWatchOnly}
        />
      )}
    </div>
  );
};
