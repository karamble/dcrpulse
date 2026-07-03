// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { X, AlertCircle, CheckCircle, Loader2 } from 'lucide-react';
import { importXpub, getAccounts } from '../services/api';
import { useWalletReady } from '../hooks/useWalletReady';
import { KeyEnds } from './AddressGroups';
import { AccountExportPicker, SelectedAccountEntry } from './AccountExportPicker';

// Reserved system accounts that other daemons / dcrwallet bind to by name and
// must never be reused for an imported xpub. Mirrors services.IsReservedAccountName.
const RESERVED_ACCOUNT_NAMES = ['mixed', 'unmixed', 'lightning', 'dex', 'imported'];

interface ImportXpubModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export const ImportXpubModal = ({ isOpen, onClose, onSuccess }: ImportXpubModalProps) => {
  // The BIP44 account index only matters for a watch-only wallet, where imported
  // per-account xpubs are later spent through offline signing (the device derives
  // by account index). A normal wallet importing an xpub for monitoring defaults
  // to account 0 and is not asked.
  const { isWatchOnly } = useWalletReady();
  const [xpub, setXpub] = useState('');
  const [accountName, setAccountName] = useState('');
  const [accountIndex, setAccountIndex] = useState('');
  const [existingNames, setExistingNames] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);
  // Selection from a device account-export file; non-empty switches the modal
  // from manual entry to the sequential file import.
  const [fileSelected, setFileSelected] = useState<SelectedAccountEntry[]>([]);
  // account -> live status while the sequential file import runs.
  const [queue, setQueue] = useState<Record<number, string> | null>(null);

  // Load existing account names when the modal opens so the new account name can
  // be checked for collisions client-side (the backend is the authoritative check).
  useEffect(() => {
    if (!isOpen) return;
    getAccounts()
      .then((accts) => setExistingNames(accts.map((a) => a.accountName)))
      .catch(() => setExistingNames([]));
  }, [isOpen]);

  if (!isOpen) return null;

  const trimmedName = accountName.trim();
  const nameError =
    trimmedName.length === 0
      ? null
      : trimmedName.length > 50
        ? 'Account name must be 50 characters or fewer'
        : RESERVED_ACCOUNT_NAMES.includes(trimmedName.toLowerCase())
          ? `'${trimmedName}' is a reserved account name`
          : existingNames.some((n) => n.toLowerCase() === trimmedName.toLowerCase())
            ? `An account named '${trimmedName}' already exists`
            : null;

  const idxTrimmed = accountIndex.trim();
  const hasIndex = idxTrimmed !== '';
  const indexNum = Number(idxTrimmed);
  const indexError = !hasIndex
    ? null
    : !/^\d+$/.test(idxTrimmed)
      ? 'Account index must be a whole number'
      : !Number.isSafeInteger(indexNum) || indexNum > 2147483647
        ? 'Account index must be 0 to 2147483647'
        : null;

  const validateXpub = (value: string): boolean => {
    // Decred mainnet xpubs start with "dpub"
    // Testnet xpubs start with "tpub"
    return value.startsWith('dpub') || value.startsWith('tpub');
  };

  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

  // Sequential import of the file selection through the existing single-import
  // path. importxpub is rate-limited (one call per 30s) and each import starts
  // an async rescan that dcrwallet serializes, so the entries are spaced out.
  const runFileImport = async () => {
    setLoading(true);
    const status: Record<number, string> = {};
    fileSelected.forEach((en) => {
      status[en.account] = 'pending';
    });
    setQueue({ ...status });
    let imported = 0;
    for (let i = 0; i < fileSelected.length; i++) {
      const en = fileSelected[i];
      status[en.account] = 'importing...';
      setQueue({ ...status });
      try {
        const result = await importXpub(en.dpub, en.editedName || `account-${en.account}`, en.account, true);
        if (!result.success) throw new Error(result.message || 'import failed');
        status[en.account] = 'imported';
        imported++;
      } catch (err: any) {
        const body = err?.response?.data;
        status[en.account] = typeof body === 'string' ? body : err?.message || 'failed';
      }
      setQueue({ ...status });
      if (i < fileSelected.length - 1) {
        status[fileSelected[i + 1].account] = 'waiting out the 30s rate limit...';
        setQueue({ ...status });
        await sleep(31_000);
      }
    }
    setLoading(false);
    if (imported > 0) {
      setSuccess(true);
      onSuccess();
    } else {
      setError('No accounts were imported - see the per-account results above.');
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess(false);

    if (fileSelected.length > 0) {
      await runFileImport();
      return;
    }

    // Validation
    if (!xpub.trim()) {
      setError('Please enter an xpub key');
      return;
    }

    if (!validateXpub(xpub.trim())) {
      setError('Invalid xpub format. Decred mainnet xpubs must start with "dpub"');
      return;
    }

    if (!trimmedName) {
      setError('Please enter an account name');
      return;
    }

    if (nameError) {
      setError(nameError);
      return;
    }

    if (indexError) {
      setError(indexError);
      return;
    }

    setLoading(true);

    try {
      // Always rescan when importing xpub to find historical transactions
      const result = await importXpub(xpub.trim(), trimmedName, hasIndex ? indexNum : undefined, true);

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
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : body?.message || err?.message || 'Failed to import xpub';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    if (!loading) {
      setXpub('');
      setAccountName('');
      setAccountIndex('');
      setError('');
      setSuccess(false);
      setFileSelected([]);
      setQueue(null);
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

          {/* Import from a device account-export file */}
          {!success && (
            <div className="p-4 rounded-lg bg-muted/5 border border-border/50 space-y-2">
              <h4 className="text-sm font-semibold">From the hardware wallet's SD card</h4>
              <AccountExportPicker disabled={loading} onSelectionChange={setFileSelected} />
            </div>
          )}

          {queue && (
            <div className="p-4 rounded-lg bg-muted/5 border border-border/50 space-y-1">
              <h4 className="text-sm font-semibold mb-1">Import progress</h4>
              {fileSelected.map((en) => (
                <div key={en.account} className="flex justify-between gap-3 text-sm">
                  <span className="truncate">
                    {en.editedName} (m/44'/42'/{en.account}')
                  </span>
                  <span className="text-muted-foreground shrink-0">{queue[en.account]}</span>
                </div>
              ))}
            </div>
          )}

          {fileSelected.length === 0 && !queue && (
          <>
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
            {validateXpub(xpub.trim()) && xpub.trim().length > 30 && (
              <div className="mt-2 p-3 rounded-lg bg-muted/5 border border-border/50">
                <p className="text-xs text-muted-foreground mb-1">Verify against your device</p>
                <KeyEnds value={xpub} className="text-sm" />
              </div>
            )}
          </div>

          {/* Account Name Input */}
          <div>
            <label htmlFor="accountName" className="block text-sm font-medium mb-2">
              Account Name <span className="text-red-500">*</span>
            </label>
            <input
              id="accountName"
              type="text"
              maxLength={50}
              value={accountName}
              onChange={(e) => setAccountName(e.target.value)}
              disabled={loading || success}
              placeholder="e.g. savings-xpub"
              className="w-full px-4 py-3 rounded-lg bg-muted/5 border border-border/50 focus:border-primary/50 focus:outline-none focus:ring-2 focus:ring-primary/20 transition-all disabled:opacity-50"
            />
            {nameError ? (
              <p className="text-xs text-destructive mt-1">{nameError}</p>
            ) : (
              <p className="text-xs text-muted-foreground mt-1">
                A friendly name for this account
              </p>
            )}
          </div>

          {/* Account Index Input - watch-only wallets only (needed for offline signing) */}
          {isWatchOnly && (
          <div>
            <label htmlFor="accountIndex" className="block text-sm font-medium mb-2">
              Account Index <span className="text-xs font-normal text-muted-foreground">(optional)</span>
            </label>
            <input
              id="accountIndex"
              type="text"
              inputMode="numeric"
              value={accountIndex}
              onChange={(e) => setAccountIndex(e.target.value)}
              disabled={loading || success}
              placeholder="Leave empty for a monitor-only xpub"
              className="w-full px-4 py-3 rounded-lg bg-muted/5 border border-border/50 focus:border-primary/50 focus:outline-none focus:ring-2 focus:ring-primary/20 transition-all disabled:opacity-50"
            />
            {indexError ? (
              <p className="text-xs text-destructive mt-1">{indexError}</p>
            ) : (
              <p className="text-xs text-muted-foreground mt-1">
                Only needed to spend from this account via offline signing on a hardware wallet.
                Leave empty for a monitor-only xpub; otherwise enter the BIP44 account number you
                exported from the device (0, 1, 2, ...).
              </p>
            )}
          </div>
          )}

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
          </>
          )}

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
              disabled={
                loading ||
                success ||
                (fileSelected.length === 0 && (trimmedName.length === 0 || !!nameError || !!indexError))
              }
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
              ) : fileSelected.length > 0 ? (
                `Import ${fileSelected.length} account${fileSelected.length === 1 ? '' : 's'}`
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

