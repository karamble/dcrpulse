// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Activity, AlertCircle, Loader2 } from 'lucide-react';

interface NodeStatusProps {
  status: 'running' | 'syncing' | 'stopped';
  syncProgress?: number;
  version?: string;
  syncMessage?: string;
}

export const NodeStatus = ({ status, syncProgress = 0, version, syncMessage }: NodeStatusProps) => {
  const getStatusConfig = () => {
    switch (status) {
      case 'running':
        return {
          icon: Activity,
          label: 'Fully Synced',
          color: 'text-success',
          bgColor: 'bg-success/10',
          borderColor: 'border-success/20',
        };
      case 'syncing':
        return {
          icon: Loader2,
          label: 'Syncing',
          color: 'text-warning',
          bgColor: 'bg-warning/10',
          borderColor: 'border-warning/20',
        };
      case 'stopped':
        return {
          icon: AlertCircle,
          label: 'Stopped',
          color: 'text-red-500',
          bgColor: 'bg-red-500/10',
          borderColor: 'border-red-500/20',
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
            <h3 className="text-lg font-semibold">Node Status</h3>
            <p className="text-sm text-muted-foreground">Decred {version || ''}</p>
          </div>
        </div>
        <div className={`px-6 py-3 rounded-xl ${status === 'running' ? 'bg-success text-white' : `${config.bgColor} border-2 ${config.borderColor}`}`}>
          <span className={`${status === 'running' ? 'text-white' : config.color} font-bold text-lg tracking-wide`}>
            {config.label}
          </span>
        </div>
      </div>
      
      {status === 'syncing' && (
        <div className="mt-6 space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted-foreground font-medium">{syncMessage || 'Blockchain Sync Progress'}</span>
            <span className="font-bold text-lg">{syncProgress.toFixed(1)}%</span>
          </div>
          <div className="relative h-3 bg-muted rounded-full overflow-hidden border border-border/50">
            <div 
              className="h-full bg-gradient-primary transition-all duration-500 ease-out relative overflow-hidden"
              style={{ width: `${syncProgress}%` }}
            >
              <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent animate-pulse" />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

