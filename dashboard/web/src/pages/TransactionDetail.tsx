// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowRightLeft, ChevronLeft, FileJson, Ticket, CheckCircle, XCircle, Coins } from 'lucide-react';
import { getTransaction, TransactionDetail as TransactionDetailType } from '../services/explorerApi';
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

  useEffect(() => {
    fetchTransaction();
  }, [txhash]);

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
                <pre className="p-4 rounded-lg bg-muted/10 overflow-auto max-h-96 text-xs font-mono">
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

