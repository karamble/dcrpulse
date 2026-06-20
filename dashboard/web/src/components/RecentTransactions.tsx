import { useEffect, useState } from 'react';
import { toYMD } from '../utils/date';
import { Link } from 'react-router-dom';
import {
  ArrowDownCircle,
  ArrowUpCircle,
  ArrowRight,
  BadgeDollarSign,
  Check,
  Clock,
  Coins,
  Shuffle,
  Ticket,
  X,
} from 'lucide-react';
import { getWalletTransactions, WalletTransaction } from '../services/api';
import { calculateTicketMaturity } from '../services/ticketService';
import { MaturityBar } from './MaturityBar';

const RECENT_LIMIT = 5;

const categoryIcon = (tx: WalletTransaction) => {
  const { category, txType, isMixed } = tx;
  if (txType === 'ticket') return <Ticket className="h-4 w-4 text-warning" />;
  if (txType === 'vote') return <Check className="h-4 w-4 text-success" />;
  if (txType === 'revocation') return <X className="h-4 w-4 text-destructive" />;
  if (category === 'vspfee') return <BadgeDollarSign className="h-4 w-4 text-orange-500" />;
  if (category === 'coinjoin' || isMixed) return <Shuffle className="h-4 w-4 text-purple-500" />;
  if (category === 'send') return <ArrowUpCircle className="h-4 w-4 text-red-500" />;
  if (category === 'receive') return <ArrowDownCircle className="h-4 w-4 text-success" />;
  if (category === 'generate') return <Coins className="h-4 w-4 text-primary" />;
  return <Coins className="h-4 w-4 text-muted-foreground" />;
};

const categoryLabel = (tx: WalletTransaction) => {
  const { category, txType, isMixed } = tx;
  if (txType === 'ticket') return 'Ticket Purchase';
  if (txType === 'vote') return 'Vote';
  if (txType === 'revocation') return 'Revocation';
  if (category === 'vspfee') return 'VSP Fee';
  if (category === 'coinjoin') return 'CoinJoin';
  if (category === 'send') return isMixed ? 'Sent (CoinJoin)' : 'Sent';
  if (category === 'receive') return isMixed ? 'Received (CoinJoin)' : 'Received';
  if (category === 'generate') return 'Mined';
  return 'Transaction';
};

const amountColor = (tx: WalletTransaction) => {
  if (tx.txType === 'ticket') return 'text-warning';
  if (tx.txType === 'vote') return 'text-success';
  if (tx.txType === 'revocation') return 'text-destructive';
  if (tx.category === 'vspfee') return 'text-orange-500';
  if (tx.category === 'coinjoin') return 'text-purple-500';
  if (tx.category === 'send') return 'text-red-500';
  if (tx.category === 'receive') return 'text-success';
  return 'text-muted-foreground';
};

const formatAmount = (amount: number) => {
  const abs = Math.abs(amount);
  const sign = amount < 0 ? '-' : '+';
  // Small amounts (e.g. CoinJoin fees ~0.00002530 DCR) need more decimals
  // to be visible; larger transfers stay readable at 4.
  const decimals = abs > 0 && abs < 0.0001 ? 8 : 4;
  return `${sign}${abs.toFixed(decimals)} DCR`;
};

const formatWhen = (tx: WalletTransaction) => {
  const ts = tx.blockTime ? tx.blockTime * 1000 : new Date(tx.time).getTime();
  const diff = Math.floor((Date.now() - ts) / 1000);
  if (diff < 60) return 'Just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return toYMD(new Date(ts));
};

export const RecentTransactions = () => {
  const [transactions, setTransactions] = useState<WalletTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await getWalletTransactions(RECENT_LIMIT);
        if (!cancelled) {
          setTransactions(data.transactions);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          console.error('Error fetching recent transactions:', err);
          setError('Failed to load recent transactions');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <Clock className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Recent Transactions</h2>
            <p className="text-sm text-muted-foreground">Latest wallet activity</p>
          </div>
        </div>
        <Link
          to="/wallet/transactions/history"
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          View all
          <ArrowRight className="h-4 w-4" />
        </Link>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary"></div>
        </div>
      ) : error ? (
        <p className="text-center py-8 text-muted-foreground">{error}</p>
      ) : transactions.length === 0 ? (
        <p className="text-center py-8 text-muted-foreground">No transactions yet</p>
      ) : (
        <div className="space-y-2">
          {transactions.map((tx, index) => (
            <Link
              key={`${tx.txid}-${tx.vout}-${index}`}
              to={`/explorer/tx/${tx.txid}`}
              className="flex flex-col gap-2 p-3 rounded-lg bg-background/50 hover:bg-background transition-colors border border-border/30 hover:border-primary/30"
            >
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <div className="flex-shrink-0">{categoryIcon(tx)}</div>
                  <div className="min-w-0">
                    <div className="font-medium text-sm truncate">{categoryLabel(tx)}</div>
                    <div className="text-xs text-muted-foreground">{formatWhen(tx)}</div>
                  </div>
                </div>
                <div className={`text-sm font-semibold ml-3 whitespace-nowrap ${amountColor(tx)}`}>
                  {formatAmount(tx.amount)}
                </div>
              </div>
              {tx.txType === 'vote' && (
                <MaturityBar
                  blocksRemaining={tx.blocksUntilSpendable}
                  className="ml-7 max-w-[180px]"
                />
              )}
              {tx.txType === 'ticket' && tx.confirmations > 0 && calculateTicketMaturity(tx).isImmature && (
                <MaturityBar
                  blocksRemaining={calculateTicketMaturity(tx).blocksUntilMature}
                  pendingSuffix="to live"
                  className="ml-7 max-w-[180px]"
                />
              )}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
};
