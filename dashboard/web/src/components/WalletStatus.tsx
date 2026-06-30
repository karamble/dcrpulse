// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link } from 'react-router-dom';
import { Activity, AlertCircle, Clock, Eye, Loader2, Lock, ShieldCheck, Ticket, Unlock, Wallet } from 'lucide-react';
import { InsecureRpcWarning } from './InsecureRpcWarning';

interface WalletStatusProps {
  status: 'synced' | 'syncing' | 'no_wallet' | 'disconnected' | 'locked';
  version?: string;
  unlocked?: boolean;
  mixerRunning?: boolean;
  autobuyerRunning?: boolean;
  voteTrickleRunning?: boolean;
  isWatchOnly?: boolean;
}

export const WalletStatus = ({
  status,
  version,
  unlocked = false,
  mixerRunning = false,
  autobuyerRunning = false,
  voteTrickleRunning = false,
  isWatchOnly = false,
}: WalletStatusProps) => {
  const getStatusConfig = () => {
    switch (status) {
      case 'synced':
        return {
          icon: Activity,
          label: 'Fully Synced',
          color: 'text-success',
          bgColor: 'bg-success/15',
          borderColor: 'border-success/30',
        };
      case 'syncing':
        return {
          icon: Loader2,
          label: 'Syncing',
          color: 'text-warning',
          bgColor: 'bg-warning/10',
          borderColor: 'border-warning/20',
        };
      case 'no_wallet':
        return {
          icon: Wallet,
          label: 'No Xpub Imported',
          color: 'text-muted-foreground',
          bgColor: 'bg-muted/10',
          borderColor: 'border-muted/20',
        };
      case 'locked':
        return {
          icon: Lock,
          label: 'Wallet Locked',
          color: 'text-warning',
          bgColor: 'bg-warning/10',
          borderColor: 'border-warning/20',
        };
      case 'disconnected':
        return {
          icon: AlertCircle,
          label: 'Disconnected',
          color: 'text-red-500',
          bgColor: 'bg-red-500/10',
          borderColor: 'border-red-500/20',
        };
      default:
        return {
          icon: AlertCircle,
          label: 'Unknown',
          color: 'text-muted-foreground',
          bgColor: 'bg-muted/10',
          borderColor: 'border-muted/20',
        };
    }
  };

  const config = getStatusConfig();
  const StatusIcon = config.icon;

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-4">
          <div className={`p-3 rounded-xl ${config.bgColor} border ${config.borderColor}`}>
            <StatusIcon className={`h-6 w-6 ${config.color} ${status === 'syncing' ? 'animate-spin' : ''}`} />
          </div>
          <div>
            <h3 className="text-lg font-semibold">Wallet Status</h3>
            <p className="text-sm text-muted-foreground">
              {isWatchOnly ? (
                <>
                  Watch-only
                  {version && (
                    <>
                      <span className="text-border"> • </span>
                      {`dcrwallet ${version}`}
                    </>
                  )}
                </>
              ) : version ? (
                `dcrwallet ${version}`
              ) : (
                'Watch-Only Wallet'
              )}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-3">
          <InsecureRpcWarning kind="wallet" />
          {autobuyerRunning && (
            <Link
              to="/wallet/staking/autobuyer"
              title="Autobuyer running - open Staking page"
              className="p-3 rounded-xl bg-success/15 border-2 border-success/30 hover:bg-success/25 transition-colors"
            >
              <Ticket className="h-6 w-6 text-success animate-pulse" />
            </Link>
          )}
          {mixerRunning && (
            <Link
              to="/wallet/privacy"
              title="Mixer running - open Privacy page"
              className="p-3 rounded-xl bg-success/15 border-2 border-success/30 hover:bg-success/25 transition-colors"
            >
              <ShieldCheck className="h-6 w-6 text-success animate-pulse" />
            </Link>
          )}
          {voteTrickleRunning && (
            <Link
              to="/wallet/governance/proposals"
              title="Vote trickle running - open Proposals page"
              className="p-3 rounded-xl bg-success/15 border-2 border-success/30 hover:bg-success/25 transition-colors"
            >
              <Clock className="h-6 w-6 text-success animate-pulse" />
            </Link>
          )}
          {/* A watch-only wallet holds no private keys, so the lock/unlock state is
              meaningless; show a Watch-only badge linking to the offline-signing flow
              (the way to spend) instead of the misleading "Spending locked". */}
          {isWatchOnly ? (
            <Link
              to="/wallet/transactions/offline"
              title="Watch-only wallet: no private keys here. Sign on your hardware device, then broadcast from the Offline signing tab."
              className="flex items-center gap-2 px-4 py-3 rounded-xl bg-primary/10 border-2 border-primary/20 hover:bg-primary/20 transition-colors"
            >
              <Eye className="h-5 w-5 text-primary" />
              <span className="text-primary font-semibold text-sm">Watch-only</span>
            </Link>
          ) : (
            version &&
            (unlocked ? (
              <div
                title="Private keys are currently unlocked."
                className="flex items-center gap-2 px-4 py-3 rounded-xl bg-warning/10 border-2 border-warning/20"
              >
                <Unlock className="h-5 w-5 text-warning" />
                <span className="text-warning font-semibold text-sm">Spending unlocked</span>
              </div>
            ) : (
              <div
                title="Spending requires your wallet passphrase (entered per action)."
                className="flex items-center gap-2 px-4 py-3 rounded-xl bg-muted/10 border-2 border-border/40"
              >
                <Lock className="h-5 w-5 text-muted-foreground" />
                <span className="text-muted-foreground font-semibold text-sm">Spending locked</span>
              </div>
            ))
          )}
          <div className={`px-6 py-3 rounded-xl ${config.bgColor} border-2 ${config.borderColor}`}>
            <span className={`${config.color} font-bold text-lg tracking-wide`}>
              {config.label}
            </span>
          </div>
        </div>
      </div>
      
      {/* Progress bar removed - now using unified SyncProgressBar component */}

      {status === 'no_wallet' && (
        <div className="mt-4 p-4 rounded-lg bg-muted/10 border border-border/50">
          <p className="text-sm text-muted-foreground">
            No xpub key has been imported yet. Click "Add X-Pub" to import your extended public key for watch-only monitoring.
          </p>
        </div>
      )}
    </div>
  );
};

