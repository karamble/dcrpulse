// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowRightLeft, ChevronLeft, FileJson, Ticket, CheckCircle, XCircle, Coins, Landmark, Loader2 } from 'lucide-react';
import { getTransaction, getVoteParsingProgress, TransactionDetail as TransactionDetailType, VoteParsingProgress } from '../services/explorerApi';
import { CopyButton } from '../components/explorer/CopyButton';
import { TimeAgo } from '../components/explorer/TimeAgo';
import { InputOutputList } from '../components/explorer/InputOutputList';

export const TransactionDetail = () => {
  const { txhash } = useParams<{ txhash: string }>();
  const navigate = useNavigate();
  const [tx, setTx] = useState<TransactionDetailType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showRawJson, setShowRawJson] = useState(false);
  const [showRawHex, setShowRawHex] = useState(false);
  const [voteProgress, setVoteProgress] = useState<VoteParsingProgress | null>(null);
  const [isPolling, setIsPolling] = useState(false);
  const hasRefreshedForComplete = useRef(false);

  useEffect(() => {
    fetchTransaction();
    hasRefreshedForComplete.current = false; // Reset on txhash change
  }, [txhash]);

  // Poll for vote parsing progress
  useEffect(() => {
    if (!tx || tx.type !== 'tspend' || !txhash) return;
    
    // If voting is complete, no need to poll (regardless of vote count)
    if (tx.votingInfo?.votingComplete) {
      setIsPolling(false);
      return;
    }

    // If we already refreshed after completion, don't poll again
    if (hasRefreshedForComplete.current) {
      setIsPolling(false);
      return;
    }

    // Start polling
    setIsPolling(true);
    const pollProgress = async () => {
      try {
        const progress = await getVoteParsingProgress(txhash);
        setVoteProgress(progress);

        // If parsing is complete, refresh transaction data once and stop polling
        if (!progress.isParsing && !hasRefreshedForComplete.current) {
          setIsPolling(false);
          hasRefreshedForComplete.current = true;
          // Refresh transaction to get final vote data
          setTimeout(() => {
            fetchTransaction();
          }, 500);
        }
      } catch (err) {
        console.error('Error fetching vote progress:', err);
      }
    };

    // Initial poll
    pollProgress();

    // Set up polling interval
    const pollInterval = setInterval(pollProgress, 2000); // Poll every 2 seconds

    return () => {
      clearInterval(pollInterval);
      setIsPolling(false);
    };
  }, [tx?.votingInfo?.votingComplete, txhash]);

  const fetchTransaction = async () => {
    if (!txhash) return;

    setLoading(true);
    setError('');

    try {
      const txData = await getTransaction(txhash);
      setTx(txData);
      setLoading(false);
    } catch (err) {
      setError('Transaction not found');
      setLoading(false);
    }
  };

  const getTxTypeIcon = (type: string) => {
    switch (type) {
      case 'ticket':
        return <Ticket className="h-6 w-6 text-warning" />;
      case 'vote':
        return <CheckCircle className="h-6 w-6 text-success" />;
      case 'revocation':
        return <XCircle className="h-6 w-6 text-red-500" />;
      case 'coinbase':
        return <Coins className="h-6 w-6 text-purple-500" />;
      case 'tspend':
        return <Landmark className="h-6 w-6 text-amber-500" />;
      case 'treasurybase':
        return <Landmark className="h-6 w-6 text-amber-600" />;
      default:
        return <ArrowRightLeft className="h-6 w-6 text-blue-500" />;
    }
  };

  const getTxTypeColor = (type: string) => {
    switch (type) {
      case 'ticket':
        return 'bg-warning/10 text-warning border-warning/20';
      case 'vote':
        return 'bg-success/10 text-success border-success/20';
      case 'revocation':
        return 'bg-red-500/10 text-red-500 border-red-500/20';
      case 'coinbase':
        return 'bg-purple-500/10 text-purple-500 border-purple-500/20';
      case 'tspend':
        return 'bg-amber-500/10 text-amber-500 border-amber-500/20';
      case 'treasurybase':
        return 'bg-amber-600/10 text-amber-600 border-amber-600/20';
      default:
        return 'bg-blue-500/10 text-blue-500 border-blue-500/20';
    }
  };

  const getTxTypeName = (type: string) => {
    switch (type) {
      case 'ticket':
        return 'Ticket Purchase (SSTx)';
      case 'vote':
        return 'Vote (SSGen)';
      case 'revocation':
        return 'Revocation (SSRtx)';
      case 'coinbase':
        return 'Coinbase';
      case 'tspend':
        return 'Treasury Spend (TSpend)';
      case 'treasurybase':
        return 'Treasury Addition (TBase)';
      default:
        return 'Regular Transaction';
    }
  };

  const formatSize = (bytes: number) => {
    return `${bytes.toLocaleString()} bytes`;
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-primary"></div>
            <p className="mt-4 text-muted-foreground">Loading transaction...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error || !tx) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center py-20">
            <ArrowRightLeft className="h-16 w-16 mx-auto text-muted-foreground/50 mb-4" />
            <h2 className="text-2xl font-bold mb-2">Transaction Not Found</h2>
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

  return (
    <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
      <div className="max-w-7xl mx-auto space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="flex items-start gap-4">
            <button
              onClick={() => navigate('/explorer')}
              className="p-2 rounded-lg hover:bg-muted/20 transition-colors mt-1"
            >
              <ChevronLeft className="h-6 w-6" />
            </button>
            <div className="flex-1">
              <div className="flex items-center gap-3 mb-2">
                {getTxTypeIcon(tx.type)}
                <h1 className="text-3xl font-bold">Transaction</h1>
              </div>
              <div className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border text-sm font-medium ${getTxTypeColor(tx.type)}`}>
                {getTxTypeName(tx.type)}
              </div>
              <p className="text-sm text-muted-foreground mt-2">
                <TimeAgo timestamp={tx.timestamp} showFull />
              </p>
            </div>
          </div>

          <button
            onClick={() => setShowRawJson(!showRawJson)}
            className="flex items-center gap-2 px-3 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm"
          >
            <FileJson className="h-4 w-4" />
            {showRawJson ? 'Hide' : 'View'} JSON
          </button>
        </div>

        {/* Transaction Hash */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <p className="text-sm text-muted-foreground mb-2">Transaction ID</p>
          <div className="flex items-center gap-3">
            <p className="font-mono text-lg break-all flex-1">{tx.txid}</p>
            <CopyButton text={tx.txid} label="Copy" />
          </div>
        </div>

        {/* Transaction Information */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <h2 className="text-xl font-semibold mb-6">Transaction Information</h2>

          {showRawJson ? (
            <pre className="p-4 rounded-lg bg-muted/10 overflow-auto max-h-96 text-xs">
              {JSON.stringify(tx, null, 2)}
            </pre>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Block */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Block</p>
                {tx.blockHeight > 0 ? (
                  <button
                    onClick={() => navigate(`/explorer/block/${tx.blockHeight}`)}
                    className="text-lg font-semibold hover:text-primary transition-colors"
                  >
                    #{tx.blockHeight.toLocaleString()}
                  </button>
                ) : (
                  <p className="text-lg font-semibold text-warning">Mempool</p>
                )}
              </div>

              {/* Confirmations */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Confirmations</p>
                <p className="text-lg font-semibold">{tx.confirmations.toLocaleString()}</p>
              </div>

              {/* Block Hash */}
              {tx.blockHash && (
                <div className="p-4 rounded-lg bg-background/50 md:col-span-2">
                  <p className="text-sm text-muted-foreground mb-2">Block Hash</p>
                  <div className="flex items-center gap-2">
                    <p className="font-mono text-sm break-all">{tx.blockHash}</p>
                    <CopyButton text={tx.blockHash} />
                  </div>
                </div>
              )}

              {/* Size */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Size</p>
                <p className="text-lg font-semibold">{formatSize(tx.size)}</p>
              </div>

              {/* Total Value */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Total Output Value</p>
                <p className="text-lg font-semibold">{tx.totalValue.toFixed(8)} DCR</p>
              </div>

              {/* Fee */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Fee</p>
                <p className="text-lg font-semibold text-warning">{tx.fee.toFixed(8)} DCR</p>
              </div>

              {/* Fee Rate */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Fee Rate</p>
                <p className="text-lg font-semibold">
                  {tx.size > 0 ? ((tx.fee / tx.size) * 1000).toFixed(5) : '0'} DCR/KB
                </p>
              </div>

              {/* Version */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Version</p>
                <p className="text-lg font-semibold">{tx.version}</p>
              </div>

              {/* Lock Time */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Lock Time</p>
                <p className="text-lg font-semibold">{tx.lockTime}</p>
              </div>

              {/* Expiry */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Expiry</p>
                <p className="text-lg font-semibold">{tx.expiry}</p>
              </div>
            </div>
          )}
        </div>

        {/* Treasury Spend Information */}
        {tx.type === 'tspend' && (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-amber-500/30 bg-amber-500/5">
            <div className="flex items-center gap-2 mb-6">
              <Landmark className="h-5 w-5 text-amber-500" />
              <h2 className="text-xl font-semibold">Treasury Spend Details</h2>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Politeia Key */}
              {tx.politeiaKey && (
                <div className="p-4 rounded-lg bg-background/50 md:col-span-2">
                  <p className="text-sm text-muted-foreground mb-2">Politeia Proposal Key</p>
                  <div className="flex items-center gap-2">
                    <p className="font-mono text-sm break-all flex-1">{tx.politeiaKey}</p>
                    <CopyButton text={tx.politeiaKey} />
                  </div>
                </div>
              )}

              {/* Expiry Height */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Expiry Height</p>
                <p className="text-lg font-semibold">{tx.expiry.toLocaleString()}</p>
              </div>

              {/* Recipients */}
              {tx.recipientCount !== undefined && tx.recipientCount > 0 && (
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Recipients</p>
                  <p className="text-lg font-semibold">{tx.recipientCount}</p>
                </div>
              )}

              {/* Total Payout */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Total Payout</p>
                <p className="text-lg font-semibold text-amber-500">{tx.totalValue.toFixed(8)} DCR</p>
              </div>

              {/* Version */}
              <div className="p-4 rounded-lg bg-background/50">
                <p className="text-sm text-muted-foreground mb-2">Transaction Version</p>
                <p className="text-lg font-semibold">
                  {tx.version}
                  {tx.version === 3 && <span className="text-xs ml-2 text-amber-500">(Treasury)</span>}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Treasury Spend Approval/Voting */}
        {tx.type === 'tspend' && tx.votingInfo && (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
            <div className="mb-6">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-xl font-semibold">Treasury Spend Approval</h2>
                <div className="flex items-center gap-2">
                  {isPolling && voteProgress?.isParsing ? (
                    <span className="px-3 py-1 rounded-full text-sm font-medium bg-blue-500/10 text-blue-500 flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Counting Votes...
                    </span>
                  ) : (
                    <>
                      <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                        tx.votingInfo.votingComplete ? 'bg-blue-500/10 text-blue-500' : 'bg-yellow-500/10 text-yellow-500'
                      }`}>
                        {tx.votingInfo.votingComplete ? 'Voting Complete' : 'Ongoing Vote'}
                      </span>
                      {tx.votingInfo.votingComplete && tx.votingInfo.votesCast > 0 && (
                        <span className={`px-3 py-1 rounded-full text-sm font-medium ${
                          tx.votingInfo.approvalRate >= 75 ? 'bg-green-500/10 text-green-500' :
                          tx.votingInfo.approvalRate >= 50 ? 'bg-yellow-500/10 text-yellow-500' :
                          'bg-red-500/10 text-red-500'
                        }`}>
                          {tx.votingInfo.approvalRate >= 75 ? 'Fast Approval' :
                           tx.votingInfo.approvalRate >= 50 ? 'Approval' : 'Rejected'}
                        </span>
                      )}
                    </>
                  )}
                </div>
              </div>

              {/* Vote Counting Progress */}
              {isPolling && voteProgress?.isParsing && (
                <div className="mb-6">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-sm text-muted-foreground">Scanning blocks for votes...</span>
                    <span className="text-sm font-semibold">{voteProgress.progress.toFixed(1)}%</span>
                  </div>
                  <div className="w-full bg-muted/20 rounded-full h-3 overflow-hidden mb-2">
                    <div 
                      className="h-full bg-blue-500 transition-all duration-500"
                      style={{ width: `${voteProgress.progress}%` }}
                    />
                  </div>
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span>Block {voteProgress.currentBlock.toLocaleString()} of {(voteProgress.currentBlock + voteProgress.totalBlocks - Math.floor(voteProgress.totalBlocks * voteProgress.progress / 100)).toLocaleString()}</span>
                    {voteProgress.estimatedTime > 0 && (
                      <span>~{voteProgress.estimatedTime}s remaining</span>
                    )}
                  </div>
                  <div className="mt-3 p-3 rounded-lg bg-blue-500/5 border border-blue-500/20">
                    <p className="text-sm">
                      <span className="font-semibold">Current Results:</span>{' '}
                      <span className="text-green-500">{voteProgress.yesVotes.toLocaleString()} Yes</span>
                      {' | '}
                      <span className="text-red-500">{voteProgress.noVotes.toLocaleString()} No</span>
                      <span className="text-muted-foreground ml-2">(Still counting...)</span>
                    </p>
                  </div>
                </div>
              )}

              {/* Approval Progress Bar - Only show when we have vote data */}
              {(!isPolling || !voteProgress?.isParsing) && tx.votingInfo.votesCast > 0 && (
                <div className="mb-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-sm text-muted-foreground">Approval Rate</span>
                    <span className="text-lg font-semibold">{tx.votingInfo.approvalRate.toFixed(1)}%</span>
                  </div>
                  <div className="w-full bg-muted/20 rounded-full h-4 overflow-hidden">
                    <div 
                      className={`h-full transition-all ${
                        tx.votingInfo.approvalRate >= 75 ? 'bg-green-500' :
                        tx.votingInfo.approvalRate >= 50 ? 'bg-yellow-500' : 'bg-red-500'
                      }`}
                      style={{ width: `${tx.votingInfo.approvalRate}%` }}
                    />
                  </div>
                  <p className="text-sm text-muted-foreground mt-2">
                    {tx.votingInfo.approvalRate.toFixed(1)}% approval of {tx.votingInfo.votesCast.toLocaleString()} votes
                  </p>
                </div>
              )}

              {/* Quorum Status - Only show when we have vote data */}
              {(!isPolling || !voteProgress?.isParsing) && tx.votingInfo.votesCast > 0 && (
                <div className="p-3 rounded-lg bg-background/50 mb-4">
                  <p className="text-sm">
                    <span className={tx.votingInfo.quorumAchieved ? 'text-green-500' : 'text-yellow-500'}>
                      {tx.votingInfo.quorumAchieved ? '✓ Quorum achieved' : '⚠ Quorum not yet reached'}
                    </span>
                    {' '}({tx.votingInfo.votesCast.toLocaleString()} of {tx.votingInfo.quorumRequired.toLocaleString()} needed votes)
                  </p>
                </div>
              )}
            </div>

            {/* Voting Statistics - Only show when we have valid vote data */}
            {(!isPolling || !voteProgress?.isParsing) && tx.votingInfo.votesCast > 0 && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Yes Votes */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Yes Votes</p>
                  <p className="text-lg font-semibold text-green-500">
                    {tx.votingInfo.yesVotes.toLocaleString()} votes ({((tx.votingInfo.yesVotes / tx.votingInfo.votesCast) * 100).toFixed(1)}%)
                  </p>
                </div>

                {/* No Votes */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">No Votes</p>
                  <p className="text-lg font-semibold text-red-500">
                    {tx.votingInfo.noVotes.toLocaleString()} votes ({((tx.votingInfo.noVotes / tx.votingInfo.votesCast) * 100).toFixed(1)}%)
                  </p>
                </div>

                {/* Eligible Votes */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Eligible Votes</p>
                  <p className="text-lg font-semibold">
                    {tx.votingInfo.eligibleVotes.toLocaleString()}
                  </p>
                </div>

                {/* Votes Cast / Turnout */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Votes Cast</p>
                  <p className="text-lg font-semibold">
                    {tx.votingInfo.votesCast.toLocaleString()} ({tx.votingInfo.turnoutRate.toFixed(1)}% turnout)
                  </p>
                </div>

                {/* Voting Started */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">Voting Started</p>
                  <p className="text-sm font-semibold">
                    <TimeAgo timestamp={tx.votingInfo.votingStartTime} showFull />
                  </p>
                </div>

                {/* Voting Ended */}
                <div className="p-4 rounded-lg bg-background/50">
                  <p className="text-sm text-muted-foreground mb-2">
                    {tx.votingInfo.votingComplete ? 'Voting Ended' : 'Voting Ends'}
                  </p>
                  <p className="text-sm font-semibold">
                    <TimeAgo timestamp={tx.votingInfo.votingEndTime} showFull />
                  </p>
                </div>

                {/* Voting Period */}
                <div className="p-4 rounded-lg bg-background/50 md:col-span-2">
                  <p className="text-sm text-muted-foreground mb-2">Voting Period</p>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => navigate(`/explorer/block/${tx.votingInfo?.votingStartBlock}`)}
                      className="text-sm font-semibold hover:text-primary transition-colors"
                    >
                      {tx.votingInfo.votingStartBlock.toLocaleString()}
                    </button>
                    <span className="text-muted-foreground">–</span>
                    <button
                      onClick={() => navigate(`/explorer/block/${tx.votingInfo?.votingEndBlock}`)}
                      className="text-sm font-semibold hover:text-primary transition-colors"
                    >
                      {tx.votingInfo.votingEndBlock.toLocaleString()}
                    </button>
                  </div>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Inputs and Outputs */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <InputOutputList inputs={tx.inputs} outputs={tx.outputs} />
        </div>

        {/* Raw Transaction Hex */}
        {tx.rawHex && (
          <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-semibold">Raw Transaction</h2>
              <button
                onClick={() => setShowRawHex(!showRawHex)}
                className="text-sm text-primary hover:text-primary/80 transition-colors"
              >
                {showRawHex ? 'Hide' : 'Show'} Hex
              </button>
            </div>

            {showRawHex && (
              <div className="relative">
                <pre className="p-4 pt-12 rounded-lg bg-muted/10 overflow-y-auto max-h-96 text-xs font-mono whitespace-pre-wrap break-all">
                  {tx.rawHex}
                </pre>
                <div className="absolute top-2 right-2">
                  <CopyButton text={tx.rawHex} label="Copy Hex" />
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

