// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link } from 'react-router-dom';
import { Activity, AlertCircle, Loader2, Lock, ShieldCheck, Ticket, Wallet } from 'lucide-react';

interface WalletStatusProps {
  status: 'synced' | 'syncing' | 'no_wallet' | 'disconnected' | 'locked';
  version?: string;
  unlocked?: boolean;
  mixerRunning?: boolean;
  autobuyerRunning?: boolean;
}

export const WalletStatus = ({
  status,
  version,
  unlocked = false,
  mixerRunning = false,
  autobuyerRunning = false,
}: WalletStatusProps) => {
  const getStatusConfig = () => {
    switch (status) {
      case 'synced':
        return {
          icon: Activity,
          label: unlocked ? 'Fully Synced & Unlocked' : 'Fully Synced',
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
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className={`p-3 rounded-xl ${config.bgColor} border ${config.borderColor}`}>
            <StatusIcon className={`h-6 w-6 ${config.color} ${status === 'syncing' ? 'animate-spin' : ''}`} />
          </div>
          <div>
            <h3 className="text-lg font-semibold">Wallet Status</h3>
            <p className="text-sm text-muted-foreground">
              {version ? `dcrwallet ${version}` : 'Watch-Only Wallet'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
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

