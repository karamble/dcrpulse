// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Box, ChevronLeft, ChevronRight, ArrowRightLeft, Ticket, CheckCircle, XCircle, FileJson } from 'lucide-react';
import { getBlockByHeight, BlockDetail as BlockDetailType } from '../services/explorerApi';
import { CopyButton } from '../components/explorer/CopyButton';
import { TimeAgo } from '../components/explorer/TimeAgo';

export const BlockDetail = () => {
  const { heightOrHash } = useParams<{ heightOrHash: string }>();
  const navigate = useNavigate();
  const [block, setBlock] = useState<BlockDetailType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showRawJson, setShowRawJson] = useState(false);

  useEffect(() => {
    fetchBlock();
  }, [heightOrHash]);

  const fetchBlock = async () => {
    if (!heightOrHash) return;

    setLoading(true);
    setError('');

    try {
      // For now, assume it's a height (we can enhance this later to handle hashes)
      const height = parseInt(heightOrHash);
      if (isNaN(height)) {
        setError('Invalid block height');
        setLoading(false);
        return;
      }

      const blockData = await getBlockByHeight(height);
      setBlock(blockData);
      setLoading(false);
    } catch (err) {
      setError('Block not found');
      setLoading(false);
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const getTxTypeIcon = (type: string) => {
    switch (type) {
      case 'ticket':
        return <Ticket className="h-4 w-4 text-warning" />;
      case 'vote':
        return <CheckCircle className="h-4 w-4 text-success" />;
      case 'revocation':
        return <XCircle className="h-4 w-4 text-red-500" />;
      default:
        return <ArrowRightLeft className="h-4 w-4 text-blue-500" />;
    }
  };

  const getTxTypeColor = (type: string) => {
    switch (type) {
      case 'ticket':
        return 'text-warning';
      case 'vote':
        return 'text-success';
      case 'revocation':
        return 'text-red-500';
      case 'coinbase':
        return 'text-purple-500';
      default:
        return 'text-blue-500';
    }
  };

  const groupTransactionsByType = () => {
    if (!block) return { regular: [], tickets: [], votes: [], revocations: [], coinbase: [] };

    return {
      regular: block.transactions.filter(tx => tx.type === 'regular'),
      tickets: block.transactions.filter(tx => tx.type === 'ticket'),
      votes: block.transactions.filter(tx => tx.type === 'vote'),
      revocations: block.transactions.filter(tx => tx.type === 'revocation'),
      coinbase: block.transactions.filter(tx => tx.type === 'coinbase'),
    };
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
            <p className="mt-4 text-muted-foreground">Loading block...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error || !block) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <Box className="h-16 w-16 mx-auto text-muted-foreground/50 mb-4" />
            <h2 className="text-2xl font-bold mb-2">Block Not Found</h2>
            <p className="text-muted-foreground mb-6">{error}</p>
            <button
              onClick={() => navigate('/explorer')}
              className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
            >
              Back to Explorer
            </button>
          </div>
        </div>
      </div>
    );
  }

  const txGroups = groupTransactionsByType();

  return (
    <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
      <div className="max-w-7xl mx-auto space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={() => navigate('/explorer')}
              className="p-2 rounded-lg hover:bg-muted/20 transition-colors"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
            <div>
              <h1 className="text-3xl font-bold">Block #{block.height.toLocaleString()}</h1>
              <p className="text-sm text-muted-foreground">
                <TimeAgo timestamp={block.timestamp} showFull />
              </p>
            </div>
          </div>

          {/* Navigation */}
          <div className="flex items-center gap-2">
            <button
              onClick={() => navigate(`/explorer/block/${block.height - 1}`)}
              disabled={block.height === 0}
              className="flex items-center gap-2 px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronLeft className="h-4 w-4" />
              Previous
            </button>
            <button
              onClick={() => block.nextHash && navigate(`/explorer/block/${block.height + 1}`)}
              disabled={!block.nextHash}
              className="flex items-center gap-2 px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Next
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Block Information Card */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <Box className="h-5 w-5 text-primary" />
              <h2 className="text-xl font-semibold">Block Information</h2>
            </div>
            <button
              onClick={() => setShowRawJson(!showRawJson)}
              className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm"
            >
              <FileJson className="h-4 w-4" />
              {showRawJson ? 'Hide' : 'View'} JSON
            </button>
          </div>

          {showRawJson ? (
            <pre className="p-4 rounded-lg bg-muted/10 overflow-auto max-h-96 text-xs">
              {JSON.stringify(block, null, 2)}
            </pre>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Hash */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Block Hash</p>
                <div className="flex items-center gap-2">
                  <p className="font-mono text-sm break-all">{block.hash}</p>
                  <CopyButton text={block.hash} />
                </div>
              </div>

              {/* Previous Hash */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Previous Block</p>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => navigate(`/explorer/block/${block.height - 1}`)}
                    className="font-mono text-sm break-all hover:text-primary transition-colors"
                  >
                    {block.previousHash}
                  </button>
                  <CopyButton text={block.previousHash} />
                </div>
              </div>

              {/* Merkle Root */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Merkle Root</p>
                <div className="flex items-center gap-2">
                  <p className="font-mono text-sm break-all">{block.merkleRoot}</p>
                  <CopyButton text={block.merkleRoot} />
                </div>
              </div>

              {/* Stake Root */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Stake Root</p>
                <div className="flex items-center gap-2">
                  <p className="font-mono text-sm break-all">{block.stakeRoot}</p>
                  <CopyButton text={block.stakeRoot} />
                </div>
              </div>

              {/* Confirmations */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Confirmations</p>
                <p className="text-lg font-semibold">{block.confirmations.toLocaleString()}</p>
              </div>

              {/* Difficulty */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Difficulty</p>
                <p className="text-lg font-semibold">{block.difficulty.toFixed(8)}</p>
              </div>

              {/* Size */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Size</p>
                <p className="text-lg font-semibold">{formatSize(block.size)}</p>
              </div>

              {/* Transactions */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Transactions</p>
                <p className="text-lg font-semibold">{block.txCount}</p>
              </div>

              {/* Version */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Version</p>
                <p className="text-lg font-semibold">{block.version}</p>
              </div>

              {/* Stake Version */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Stake Version</p>
                <p className="text-lg font-semibold">{block.stakeVersion}</p>
              </div>

              {/* Vote Bits */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Vote Bits</p>
                <p className="text-lg font-semibold font-mono">0x{block.voteBits.toString(16)}</p>
              </div>

              {/* Nonce */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Nonce</p>
                <p className="text-lg font-semibold">{block.nonce}</p>
              </div>
            </div>
          )}
        </div>

        {/* Transactions */}
        {block.transactions.length > 0 && (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
            <div className="flex items-center gap-2 mb-6">
              <ArrowRightLeft className="h-5 w-5 text-primary" />
              <h2 className="text-xl font-semibold">Transactions ({block.transactions.length})</h2>
            </div>

            <div className="space-y-6">
              {/* Coinbase Transactions */}
              {txGroups.coinbase.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-purple-500 mb-3 flex items-center gap-2">
                    <ArrowRightLeft className="h-4 w-4" />
                    Coinbase ({txGroups.coinbase.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.coinbase.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Vote Transactions */}
              {txGroups.votes.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-success mb-3 flex items-center gap-2">
                    <CheckCircle className="h-4 w-4" />
                    Votes ({txGroups.votes.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.votes.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Ticket Transactions */}
              {txGroups.tickets.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-warning mb-3 flex items-center gap-2">
                    <Ticket className="h-4 w-4" />
                    Tickets ({txGroups.tickets.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.tickets.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Regular Transactions */}
              {txGroups.regular.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-blue-500 mb-3 flex items-center gap-2">
                    <ArrowRightLeft className="h-4 w-4" />
                    Regular ({txGroups.regular.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.regular.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Revocation Transactions */}
              {txGroups.revocations.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-red-500 mb-3 flex items-center gap-2">
                    <XCircle className="h-4 w-4" />
                    Revocations ({txGroups.revocations.length})
                  </h3>
                  <div className="space-y-2">
                    {txGroups.revocations.map((tx) => (
                      <button
                        key={tx.txid}
                        onClick={() => navigate(`/explorer/tx/${tx.txid}`)}
                        className="w-full p-4 rounded-lg bg-background/50 hover:bg-background/70 transition-colors text-left"
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 flex-1 min-w-0">
                            {getTxTypeIcon(tx.type)}
                            <span className="font-mono text-sm truncate">{tx.txid}</span>
                            <CopyButton text={tx.txid} />
                          </div>
                          <div className="flex items-center gap-4 text-sm">
                            <span className="text-muted-foreground">{tx.size} bytes</span>
                            <span className={getTxTypeColor(tx.type)}>{tx.totalValue.toFixed(2)} DCR</span>
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

