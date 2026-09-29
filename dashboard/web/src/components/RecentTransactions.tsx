import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, Clock } from 'lucide-react';
import { getWalletTransactions, WalletTransaction } from '../services/api';
import { calculateTicketMaturity } from '../services/ticketService';
import { MaturityBar } from './MaturityBar';
import { TxCategoryIcon, formatTxAmount, txAmountColor, txCategoryLabel, txWhen } from './walletTxFormat';

const RECENT_LIMIT = 5;

export const RecentTransactions = ({ hideViewAll = false }: { hideViewAll?: boolean } = {}) => {
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
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
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
        {!hideViewAll && (
          <Link
            to="/wallet/transactions/history"
            className="flex items-center gap-1 text-sm text-primary hover:underline"
          >
            View all
            <ArrowRight className="h-4 w-4" />
          </Link>
        )}
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
                  <div className="flex-shrink-0"><TxCategoryIcon tx={tx} className="h-4 w-4" /></div>
                  <div className="min-w-0">
                    <div className="font-medium text-sm truncate">{txCategoryLabel(tx)}</div>
                    <div className="text-xs text-muted-foreground">{txWhen(tx)}</div>
                  </div>
                </div>
                <div className={`text-sm font-semibold ml-3 whitespace-nowrap ${txAmountColor(tx)}`}>
                  {formatTxAmount(tx.amount, true)}
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
