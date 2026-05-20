import { useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  Coins,
  Lock as LockIcon,
  Clock,
  AlertCircle,
  Vote,
  Edit2,
  ShieldAlert,
} from 'lucide-react';
import { AccountInfo } from '../../services/api';
import { ExtendedPubkeyReveal } from './ExtendedPubkeyReveal';

const IMPORTED_ACCOUNT_NUMBER = 2147483647;

export const isImportedAccount = (a: AccountInfo): boolean =>
  a.accountNumber === IMPORTED_ACCOUNT_NUMBER || a.accountName === 'imported';

interface Props {
  account: AccountInfo;
  onRename: (account: AccountInfo) => void;
}

const formatDcr = (v: number) => v.toFixed(8);

export const AccountRow = ({ account, onRename }: Props) => {
  const [expanded, setExpanded] = useState(false);
  const isImported = isImportedAccount(account);

  return (
    <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between p-4 hover:bg-muted/10 transition-colors text-left"
      >
        <div className="flex items-center gap-3">
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
          <div>
            <div className="flex items-center gap-2">
              <h4 className="font-semibold text-foreground">{account.accountName}</h4>
              {isImported && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-amber-500/15 text-amber-600 border border-amber-500/30">
                  <ShieldAlert className="h-3 w-3" />
                  Imported
                </span>
              )}
            </div>
            <p className="text-xs text-muted-foreground">Account #{account.accountNumber}</p>
          </div>
        </div>
        <div className="text-right">
          <p className="text-base font-semibold text-primary">
            {formatDcr(isImported ? account.votingAuthority : account.totalBalance)} DCR
          </p>
          <p className="text-xs text-muted-foreground">
            {isImported ? 'Voting authority' : `Spendable ${formatDcr(account.spendableBalance)}`}
          </p>
        </div>
      </button>

      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-border/30 pt-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
            <div className="space-y-1.5">
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Coins className="h-3.5 w-3.5 text-success" />
                  Spendable
                </span>
                <span className="font-mono">{formatDcr(account.spendableBalance)}</span>
              </div>
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <LockIcon className="h-3.5 w-3.5 text-blue-500" />
                  Locked by tickets
                </span>
                <span className="font-mono">{formatDcr(account.lockedByTickets)}</span>
              </div>
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Clock className="h-3.5 w-3.5 text-warning" />
                  Immature rewards
                </span>
                <span className="font-mono">{formatDcr(account.immatureCoinbaseRewards)}</span>
              </div>
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Clock className="h-3.5 w-3.5 text-warning" />
                  Immature stake gen
                </span>
                <span className="font-mono">{formatDcr(account.immatureStakeGeneration)}</span>
              </div>
            </div>
            <div className="space-y-1.5">
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Vote className="h-3.5 w-3.5 text-purple-500" />
                  Voting authority
                </span>
                <span className="font-mono">{formatDcr(account.votingAuthority)}</span>
              </div>
              <div className="flex justify-between">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <AlertCircle className="h-3.5 w-3.5 text-muted-foreground" />
                  Unconfirmed
                </span>
                <span className="font-mono">{formatDcr(account.unconfirmedBalance)}</span>
              </div>
              <div className="flex justify-between pt-1.5 border-t border-border/30">
                <span className="font-medium">Total</span>
                <span className="font-mono font-semibold text-primary">
                  {formatDcr(account.totalBalance)}
                </span>
              </div>
            </div>
          </div>

          <ExtendedPubkeyReveal accountNumber={account.accountNumber} disabled={isImported} />

          <div className="flex items-center gap-2 pt-2 border-t border-border/30">
            <button
              onClick={() => onRename(account)}
              disabled={isImported}
              title={isImported ? 'The imported account cannot be renamed' : 'Rename account'}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-xs disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Edit2 className="h-3.5 w-3.5" />
              <span>Rename</span>
            </button>
            {isImported && (
              <span className="text-xs text-muted-foreground">
                Imported account — actions disabled
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
