import { useCallback, useEffect, useState } from 'react';
import { Plus, AlertCircle } from 'lucide-react';
import { AccountInfo, getAccounts } from '../services/api';
import { AccountRow, isImportedAccount } from '../components/accounts/AccountRow';
import { CreateAccountModal } from '../components/accounts/CreateAccountModal';
import { RenameAccountModal } from '../components/accounts/RenameAccountModal';

export const AccountsPage = () => {
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<AccountInfo | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const data = await getAccounts();
      setAccounts(data);
    } catch (err: any) {
      const status = err?.response?.status;
      if (status === 503) {
        setError('Wallet is not loaded yet.');
      } else {
        setError('Failed to load accounts');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const sorted = [...accounts].sort((a, b) => {
    const aImp = isImportedAccount(a);
    const bImp = isImportedAccount(b);
    if (aImp !== bImp) return aImp ? 1 : -1;
    return a.accountNumber - b.accountNumber;
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-end">
        <button
          onClick={() => setCreateOpen(true)}
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all hover:opacity-90"
        >
          <Plus className="h-4 w-4" />
          <span>New Account</span>
        </button>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-destructive/10 border border-destructive/30 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {loading ? (
        <div className="p-12 rounded-xl bg-gradient-card border border-border/50 text-center text-muted-foreground">
          Loading accounts…
        </div>
      ) : sorted.length === 0 ? (
        <div className="p-12 rounded-xl bg-gradient-card border border-border/50 text-center">
          <p className="text-muted-foreground">No accounts found.</p>
          <p className="text-sm text-muted-foreground mt-2">
            Use "New Account" above to create one.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {sorted.map((account) => (
            <AccountRow
              key={account.accountNumber}
              account={account}
              onRename={(a) => setRenameTarget(a)}
            />
          ))}
        </div>
      )}

      <CreateAccountModal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        onSuccess={() => {
          setCreateOpen(false);
          load();
        }}
      />

      {renameTarget && (
        <RenameAccountModal
          isOpen={!!renameTarget}
          accountNumber={renameTarget.accountNumber}
          currentName={renameTarget.accountName}
          onClose={() => setRenameTarget(null)}
          onSuccess={() => {
            setRenameTarget(null);
            load();
          }}
        />
      )}
    </div>
  );
};
