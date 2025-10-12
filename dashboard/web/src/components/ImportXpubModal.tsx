// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { X, AlertCircle, CheckCircle, Loader2 } from 'lucide-react';
import { importXpub } from '../services/api';

interface ImportXpubModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export const ImportXpubModal = ({ isOpen, onClose, onSuccess }: ImportXpubModalProps) => {
  const [xpub, setXpub] = useState('');
  const [accountName, setAccountName] = useState('imported');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);

  if (!isOpen) return null;

  const validateXpub = (value: string): boolean => {
    // Decred mainnet xpubs start with "dpub"
    // Testnet xpubs start with "tpub"
    return value.startsWith('dpub') || value.startsWith('tpub');
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess(false);

    // Validation
    if (!xpub.trim()) {
      setError('Please enter an xpub key');
      return;
    }

    if (!validateXpub(xpub.trim())) {
      setError('Invalid xpub format. Decred mainnet xpubs must start with "dpub"');
      return;
    }

    if (!accountName.trim()) {
      setError('Please enter an account name');
      return;
    }

    setLoading(true);

    try {
      // Always rescan when importing xpub to find historical transactions
      const result = await importXpub(xpub.trim(), accountName.trim(), true);
      
      if (result.success) {
        setSuccess(true);
        // Immediately trigger preparing state in parent
        onSuccess();
        // Close modal after showing success message briefly
        setTimeout(() => {
          handleClose();
        }, 2000);
      } else {
        setError(result.message || 'Failed to import xpub');
      }
    } catch (err: any) {
      console.error('Error importing xpub:', err);
      setError(err.response?.data?.message || err.message || 'Failed to import xpub');
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    if (!loading) {
      setXpub('');
      setAccountName('imported');
      setError('');
      setSuccess(false);
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="bg-background border border-border/50 rounded-xl shadow-2xl max-w-2xl w-full max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <div>
            <h2 className="text-2xl font-bold bg-gradient-primary bg-clip-text text-transparent">
              Import Extended Public Key
            </h2>
            <p className="text-sm text-muted-foreground mt-1">
              Import your xpub for watch-only wallet monitoring
            </p>
          </div>
          <button
            onClick={handleClose}
            disabled={loading}
            className="p-2 rounded-lg hover:bg-muted/20 transition-colors disabled:opacity-50"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Content */}
        <form onSubmit={handleSubmit} className="p-6 space-y-6">
          {/* Error Message */}
          {error && (
            <div className="p-4 rounded-lg bg-red-500/10 border border-red-500/20 flex items-start gap-3 animate-fade-in">
              <AlertCircle className="h-5 w-5 text-red-500 flex-shrink-0 mt-0.5" />
              <div>
                <p className="font-medium text-red-500">Import Failed</p>
                <p className="text-sm text-red-500/80 mt-1">{error}</p>
              </div>
            </div>
          )}

          {/* Success Message */}
          {success && (
            <div className="p-4 rounded-lg bg-success/10 border border-success/20 flex items-start gap-3 animate-fade-in">
              <CheckCircle className="h-5 w-5 text-success flex-shrink-0 mt-0.5" />
              <div>
                <p className="font-medium text-success">Import Successful!</p>
                <p className="text-sm text-success/80 mt-1">
                  Starting blockchain rescan to find your transactions...
                </p>
              </div>
            </div>
          )}

          {/* Info Box */}
          <div className="p-4 rounded-lg bg-primary/5 border border-primary/10">
            <h4 className="font-semibold text-primary mb-2">What is an Extended Public Key (xpub)?</h4>
            <p className="text-sm text-muted-foreground">
              An xpub allows you to monitor your wallet's balances and transactions without exposing your private keys. 
              This wallet operates in watch-only mode and cannot spend funds.
            </p>
            <p className="text-sm text-muted-foreground mt-2">
              <strong>Format:</strong> Decred mainnet xpubs start with <code className="px-1 py-0.5 rounded bg-muted/20 font-mono text-xs">dpub</code>
            </p>
          </div>

          {/* Xpub Input */}
          <div>
            <label htmlFor="xpub" className="block text-sm font-medium mb-2">
              Extended Public Key (xpub) <span className="text-red-500">*</span>
            </label>
            <textarea
              id="xpub"
              value={xpub}
              onChange={(e) => setXpub(e.target.value)}
              disabled={loading || success}
              placeholder="dpub..."
              rows={3}
              className="w-full px-4 py-3 rounded-lg bg-muted/5 border border-border/50 focus:border-primary/50 focus:outline-none focus:ring-2 focus:ring-primary/20 transition-all disabled:opacity-50 font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Paste your extended public key here
            </p>
          </div>

          {/* Account Name Input */}
          <div>
            <label htmlFor="accountName" className="block text-sm font-medium mb-2">
              Account Name <span className="text-red-500">*</span>
            </label>
            <input
              id="accountName"
              type="text"
              value={accountName}
              onChange={(e) => setAccountName(e.target.value)}
              disabled={loading || success}
              placeholder="imported"
              className="w-full px-4 py-3 rounded-lg bg-muted/5 border border-border/50 focus:border-primary/50 focus:outline-none focus:ring-2 focus:ring-primary/20 transition-all disabled:opacity-50"
            />
            <p className="text-xs text-muted-foreground mt-1">
              A friendly name for this account
            </p>
          </div>

          {/* Info about automatic rescan */}
          <div className="p-4 rounded-lg bg-info/5 border border-info/10">
            <p className="text-sm text-muted-foreground">
              <strong>Note:</strong> After import, the wallet will automatically rescan the blockchain from block 0 
              to find all your historical transactions. This typically takes 5-30 minutes depending on blockchain size.
            </p>
          </div>

          {/* Example */}
          <div className="p-4 rounded-lg bg-muted/5 border border-muted/10">
            <h4 className="text-xs font-semibold text-muted-foreground mb-2">Example xpub:</h4>
            <code className="text-xs font-mono text-muted-foreground break-all">
              dpubZF4LSCdF7y8x8CX1mGz4DEKHGTy9Jd5jMmhJPfTqPqTc...
            </code>
          </div>

          {/* Actions */}
          <div className="flex items-center justify-end gap-3 pt-4 border-t border-border/50">
            <button
              type="button"
              onClick={handleClose}
              disabled={loading}
              className="px-6 py-3 rounded-lg border border-border/50 hover:bg-muted/20 transition-all disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading || success}
              className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all disabled:opacity-50 flex items-center gap-2"
            >
              {loading ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Importing...
                </>
              ) : success ? (
                <>
                  <CheckCircle className="h-4 w-4" />
                  Imported!
                </>
              ) : (
                'Import Xpub'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

