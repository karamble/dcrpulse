import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Coins, ExternalLink, Loader2, Send, ShieldCheck, Wallet } from 'lucide-react';
import {
  AccountInfo,
  PrivacyStatus,
  PurchaseEvent,
  StakingInfo,
  VSPInfo,
  getAccounts,
  getPrivacyStatus,
  getPurchaseStatus,
  getWalletDashboard,
  isAsyncPurchase,
  purchaseTickets,
  subscribePurchaseEvents,
} from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';
import { VSPSelect } from './VSPSelect';

const formatDcr = (v: number): string => v.toFixed(8);

export const PurchaseTicketForm = () => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [staking, setStaking] = useState<StakingInfo | null>(null);
  const [privacy, setPrivacy] = useState<PrivacyStatus | null>(null);
  const [account, setAccount] = useState<number | null>(null);
  const [vsp, setVsp] = useState<VSPInfo | null>(null);
  const [numTickets, setNumTickets] = useState<number>(1);
  const [modalOpen, setModalOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string[] | null>(null);
  // A privacy/mixed purchase runs in the background (it must CSPP-mix first,
  // which can take up to ~10 min). inProgress drives the progress panel and
  // events holds the streamed log. inProgressRef lets the WS callback read the
  // current value and ignore replayed events from past purchases.
  const [inProgress, setInProgress] = useState(false);
  const [events, setEvents] = useState<PurchaseEvent[]>([]);
  const inProgressRef = useRef(false);
  const network: 'mainnet' | 'testnet' = 'mainnet';

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [accs, dash, priv] = await Promise.all([
          getAccounts(),
          getWalletDashboard(),
          getPrivacyStatus().catch(() => null),
        ]);
        if (cancelled) return;
        const usable = accs.filter((a) => a.accountName !== 'imported');
        setAccounts(usable);
        setPrivacy(priv);
        // When privacy is configured the purchase is always mixed and the
        // backend funds it from the mixed account, so lock the source to it.
        if (priv?.configured && priv.mixedAccount !== undefined) {
          setAccount(priv.mixedAccount);
        } else {
          setAccount((current) => {
            if (current !== null) return current;
            const mixed = usable.find((a) => a.accountName === 'mixed');
            return mixed ? mixed.accountNumber : null;
          });
        }
        setStaking(dash.stakingInfo ?? null);
      } catch (err: any) {
        if (cancelled) return;
        setError(err?.message || 'Failed to load wallet state');
      }
    };
    load();
    const id = window.setInterval(load, 10000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  // Bootstrap in-progress state on mount so a page reload during a long mixed
  // purchase re-attaches to it (status is the source of truth, like the
  // autobuyer). A completed past purchase reports inProgress=false, so its old
  // result is not shown as if it were fresh.
  useEffect(() => {
    let cancelled = false;
    getPurchaseStatus()
      .then((st) => {
        if (cancelled) return;
        if (st.inProgress) {
          inProgressRef.current = true;
          setInProgress(true);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  // Stream purchase progress/result events. The log is always appended; the
  // terminal done/error transitions are applied only while a purchase we know
  // about is active (inProgressRef), so replayed events from a prior purchase
  // never trigger a false success/error.
  useEffect(() => {
    return subscribePurchaseEvents(
      (ev) => {
        setEvents((prev) => {
          const next = [...prev, ev];
          if (next.length > 50) next.splice(0, next.length - 50);
          return next;
        });
        if (!inProgressRef.current) return;
        if (ev.kind === 'done') {
          inProgressRef.current = false;
          setInProgress(false);
          setSuccess(ev.ticketHashes ?? []);
        } else if (ev.kind === 'error') {
          inProgressRef.current = false;
          setInProgress(false);
          setError(ev.message);
        }
      },
      (err) => console.error('Purchase events WebSocket error:', err),
    );
  }, []);

  const ticketPrice = staking?.currentDifficulty ?? 0;
  const stakeCost = ticketPrice * numTickets;
  // VSP fee is a % of the expected stake reward per vote, not of the stake.
  // Use the current per-vote subsidy as the estimate; actual reward may shift
  // between purchase and vote, so the fee is approximate.
  const perVoteReward = (staking?.blockSubsidyPos ?? 0) / 5;
  const vspFee = perVoteReward * numTickets * ((vsp?.feePercentage ?? 0) / 100);
  const totalCost = stakeCost + vspFee;

  const selectedAccount = useMemo(
    () => (account === null ? null : accounts.find((a) => a.accountNumber === account) ?? null),
    [accounts, account],
  );
  const spendable = selectedAccount?.spendableBalance ?? 0;
  const remaining = spendable - totalCost;
  const canAfford = spendable >= totalCost;

  const privacyOn = !!privacy?.configured;

  const canSubmit =
    account !== null && vsp !== null && numTickets > 0 && ticketPrice > 0 && canAfford;

  const handleConfirm = async (passphrase: string) => {
    if (account === null || vsp === null) return;
    setError(null);
    try {
      // For mixed purchases the backend overrides the source/change to the
      // mixed/unmixed accounts; send the unmixed account as change so the
      // request reflects the same intent.
      const resp = await purchaseTickets({
        account,
        numTickets,
        vspHost: vsp.host,
        vspPubkey: vsp.pubkey,
        changeAccount: privacyOn && privacy?.changeAccount !== undefined ? privacy.changeAccount : account,
        passphrase,
      });
      setModalOpen(false);
      if (isAsyncPurchase(resp)) {
        // Mixed purchase: the backend accepted it and runs it in the
        // background. Switch to the progress panel; the WS delivers the result.
        setEvents([]);
        inProgressRef.current = true;
        setInProgress(true);
        return;
      }
      setSuccess(resp.ticketHashes);
    } catch (err: any) {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Purchase failed';
      throw new Error(msg);
    }
  };

  if (inProgress) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/30 space-y-3 animate-fade-in">
        <div className="flex items-center gap-2">
          <Loader2 className="h-5 w-5 text-primary animate-spin shrink-0" />
          <h3 className="text-lg font-semibold">Purchasing mixed tickets...</h3>
        </div>
        <p className="text-sm text-muted-foreground">
          Your funds are CoinShuffle++ mixed before the ticket is bought, which can take up to
          ~10 minutes. You can leave this page; the purchase keeps running in the background.
        </p>
        <div className="rounded-lg bg-background/50 border border-border/30 p-3 max-h-48 overflow-y-auto space-y-1 font-mono text-xs">
          {events.length === 0 ? (
            <p className="text-muted-foreground">Waiting for progress...</p>
          ) : (
            events.map((ev, i) => (
              <div
                key={i}
                className={ev.level === 'error' ? 'text-destructive' : 'text-muted-foreground'}
              >
                {ev.message}
              </div>
            ))
          )}
        </div>
      </div>
    );
  }

  if (success) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-success/30 space-y-3 animate-fade-in">
        <h3 className="text-lg font-semibold text-success">
          Purchased {success.length} ticket{success.length === 1 ? '' : 's'}
        </h3>
        <ul className="space-y-1 font-mono text-xs">
          {success.map((h) => (
            <li key={h} className="flex items-center gap-2">
              <Link
                to={`/explorer/tx/${h}`}
                className="text-primary hover:underline truncate flex items-center gap-1"
              >
                {h}
                <ExternalLink className="h-3 w-3 shrink-0" />
              </Link>
            </li>
          ))}
        </ul>
        <button
          onClick={() => {
            setSuccess(null);
            setNumTickets(1);
          }}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors"
        >
          Purchase more
        </button>
      </div>
    );
  }

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <Send className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Purchase tickets</h3>
      </div>

      {privacyOn ? (
        <div className="space-y-2">
          <div className="flex items-center gap-2 p-3 rounded-lg bg-primary/10 border border-primary/30 text-sm">
            <ShieldCheck className="h-4 w-4 text-primary shrink-0" />
            <span>
              Purchasing <span className="font-semibold">mixed (private)</span> tickets from your
              mixed account
              {selectedAccount ? ` (${selectedAccount.spendableBalance.toFixed(2)} DCR available)` : ''}.
            </span>
          </div>
          {privacy?.mixerRunning && (
            <p className="text-xs text-muted-foreground">
              The mixer will pause briefly during the purchase and restart afterwards.
            </p>
          )}
        </div>
      ) : (
        <div className="space-y-1">
          <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
            <Wallet className="h-4 w-4" />
            Source account
          </label>
          <select
            value={account ?? ''}
            onChange={(e) => setAccount(e.target.value === '' ? null : Number(e.target.value))}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm"
          >
            <option value="" disabled>
              Select account
            </option>
            {accounts.map((a) => (
              <option key={a.accountNumber} value={a.accountNumber}>
                {a.accountName} ({a.spendableBalance.toFixed(2)} DCR)
              </option>
            ))}
          </select>
        </div>
      )}

      <VSPSelect network={network} value={vsp} onChange={setVsp} />

      <div className="space-y-1">
        <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
          <Coins className="h-4 w-4" />
          Number of tickets
        </label>
        <input
          type="number"
          min={1}
          step={1}
          value={numTickets}
          onChange={(e) => setNumTickets(Math.max(1, Math.floor(Number(e.target.value) || 1)))}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm font-mono"
        />
      </div>

      {ticketPrice > 0 && (
        <div className="p-3 rounded-lg bg-background/50 border border-border/30 space-y-1 text-sm">
          <div className="flex justify-between text-muted-foreground">
            <span>Ticket price</span>
            <span className="font-mono whitespace-nowrap">{formatDcr(ticketPrice)} DCR</span>
          </div>
          <div className="flex justify-between text-muted-foreground">
            <span>Stake ({numTickets} × {formatDcr(ticketPrice)})</span>
            <span className="font-mono whitespace-nowrap">{formatDcr(stakeCost)} DCR</span>
          </div>
          {vsp && (
            <div className="flex justify-between text-muted-foreground">
              <span>VSP fee ({vsp.feePercentage.toFixed(2)}%)</span>
              <span className="font-mono whitespace-nowrap">~{formatDcr(vspFee)} DCR</span>
            </div>
          )}
          <div className="flex justify-between text-foreground font-semibold pt-1 border-t border-border/30">
            <span>Total</span>
            <span className="font-mono whitespace-nowrap">{vsp ? '~' : ''}{formatDcr(totalCost)} DCR</span>
          </div>
          {selectedAccount && (
            <div className={`flex justify-between pt-1 ${canAfford ? 'text-muted-foreground' : 'text-destructive'}`}>
              <span>Remaining after purchase</span>
              <span className="font-mono whitespace-nowrap">{formatDcr(remaining)} DCR</span>
            </div>
          )}
        </div>
      )}

      {error && <p className="text-sm text-destructive">{error}</p>}

      <button
        onClick={() => setModalOpen(true)}
        disabled={!canSubmit}
        className="w-full px-4 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {!canAfford && selectedAccount ? 'Insufficient balance' : 'Purchase'}
      </button>

      <PassphraseModal
        isOpen={modalOpen}
        title={`Purchase ${numTickets} ticket${numTickets === 1 ? '' : 's'}`}
        description="Enter your private passphrase to sign the ticket transaction."
        submitLabel="Purchase"
        busyLabel="Purchasing..."
        onSubmit={handleConfirm}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
};
