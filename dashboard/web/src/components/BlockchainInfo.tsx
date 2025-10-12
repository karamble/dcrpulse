// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Blocks } from 'lucide-react';
import { Link } from 'react-router-dom';
import { BlockchainInfo as BlockchainInfoType } from '../services/api';

interface BlockchainInfoProps {
  data?: BlockchainInfoType;
}

export const BlockchainInfo = ({ data }: BlockchainInfoProps) => {
  const formatTimeAgo = (timestamp: number) => {
    const now = Date.now() / 1000; // Convert to seconds
    const diff = now - timestamp;
    
    if (diff < 60) return `${Math.floor(diff)}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:border-primary/20 transition-all duration-300 group animate-fade-in">
      <div className="flex items-center gap-3 mb-6">
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
          <Blocks className="h-6 w-6 text-primary" />
        </div>
        <div>
          <h3 className="text-lg font-semibold">Recent Blocks</h3>
          <p className="text-sm text-muted-foreground">Latest mined blocks</p>
        </div>
      </div>
      
      <div className="space-y-3">
        {data?.recentBlocks && data.recentBlocks.length > 0 ? (
          data.recentBlocks.map((block, index) => (
            <Link
              key={block.hash}
              to={`/explorer/block/${block.height}`}
              className="flex items-center justify-between p-3 rounded-lg bg-muted/10 hover:bg-muted/20 border border-border/30 hover:border-primary/30 transition-all duration-200 group/block"
            >
              <div className="flex items-center gap-3">
                <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-primary/10 border border-primary/20">
                  <span className="text-xs font-bold text-primary">{index + 1}</span>
                </div>
                <div>
                  <div className="font-semibold text-sm group-hover/block:text-primary transition-colors">
                    Block #{block.height.toLocaleString()}
                  </div>
                  <div className="text-xs text-muted-foreground font-mono">
                    {block.hash.substring(0, 16)}...
                  </div>
                </div>
              </div>
              <div className="text-xs text-muted-foreground">
                {formatTimeAgo(block.timestamp)}
              </div>
            </Link>
          ))
        ) : (
          <div className="text-center py-4 text-muted-foreground">
            Loading blocks...
          </div>
        )}
      </div>
    </div>
  );
};

