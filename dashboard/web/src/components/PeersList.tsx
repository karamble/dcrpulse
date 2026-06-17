// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Wifi, Clock, HardDrive, Activity, Star } from 'lucide-react';
import { Peer } from '../services/api';

interface PeersListProps {
  peers?: Peer[];
}

// OnionIcon marks a peer connected over Tor - a .onion address (outbound) or an
// inbound connection arriving via the local Tor hidden service. Inline SVG to
// avoid a new dependency; styled like the lucide icons used in this card.
const OnionIcon = ({ className }: { className?: string }) => (
  <svg
    className={className}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
  >
    <title>Connected over Tor (.onion)</title>
    <path d="M12 3C8 8 6 11 6 14.5 6 18.5 8.7 21 12 21s6-2.5 6-6.5C18 11 16 8 12 3Z" />
    <path d="M12 3c0-1 .5-1.5 1.5-1.5" />
    <path d="M9.2 14.5c0 3.5 1.2 6 2.8 6s2.8-2.5 2.8-6" />
  </svg>
);

export const PeersList = ({ peers = [] }: PeersListProps) => {
  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Wifi className="h-6 w-6 text-primary" />
        </div>
        <div className="flex-1">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Connected Peers</h3>
            <span className="px-3 py-1 rounded-md text-sm font-medium bg-primary/10 text-primary border border-primary/20">
              {peers.length} Active
            </span>
          </div>
          <p className="text-sm text-muted-foreground">Active network connections</p>
        </div>
      </div>
      
      <div className="max-h-[400px] overflow-y-auto">
        <div className="space-y-3">
          {peers.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No peers connected
            </div>
          ) : (
            peers.map((peer) => (
              <div
                key={peer.id}
                className="p-4 rounded-lg bg-muted/30 border border-border/30 hover:border-primary/30 transition-all"
              >
                <div className="flex items-start justify-between mb-2">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className={`shrink-0 p-2 rounded-lg border ${
                      peer.tor
                        ? 'bg-[#7D4698]/10 border-[#7D4698]/30'
                        : peer.isSyncNode
                        ? 'bg-primary/10 border-primary/20'
                        : 'bg-success/10 border-success/20'
                    }`}>
                      {peer.tor ? (
                        <OnionIcon className="h-4 w-4 text-[#7D4698]" />
                      ) : peer.isSyncNode ? (
                        <Star className="h-4 w-4 text-primary" />
                      ) : (
                        <Wifi className="h-4 w-4 text-success" />
                      )}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-start gap-2 min-w-0">
                        {peer.tor && peer.inbound ? (
                          <p className="text-sm font-medium min-w-0 break-all">
                            Inbound via Tor{' '}
                            <span className="text-muted-foreground">({peer.address})</span>
                          </p>
                        ) : (
                          <p className="text-sm font-medium min-w-0 break-all">{peer.address}</p>
                        )}
                        {peer.isSyncNode && (
                          <span className="shrink-0 mt-0.5 px-1.5 py-0.5 text-[10px] font-medium bg-primary/20 text-primary rounded">
                            SYNC
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        dcrd {peer.version}
                      </p>
                    </div>
                  </div>
                </div>
                
                <div className="grid grid-cols-3 gap-3 mt-3 text-xs">
                  <div className="flex items-center gap-1.5">
                    <Activity className="h-3 w-3 text-muted-foreground" />
                    <span className="text-muted-foreground">Ping:</span>
                    <span className="font-medium">{peer.latency}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Clock className="h-3 w-3 text-muted-foreground" />
                    <span className="text-muted-foreground">Up:</span>
                    <span className="font-medium">{peer.connTime}</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <HardDrive className="h-3 w-3 text-muted-foreground" />
                    <span className="text-muted-foreground">Traffic:</span>
                    <span className="font-medium">{peer.traffic}</span>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};

