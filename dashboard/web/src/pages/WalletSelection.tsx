// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Wallet, Plus, Eye, ShieldCheck, Pencil, Trash2, Check, X, AlertCircle, ArrowLeft } from 'lucide-react';
import {
  listWallets,
  selectWallet,
  renameWallet,
  deleteWallet,
  type WalletInfo,
} from '../services/api';
import { WalletSetup } from '../components/WalletSetup';

interface WalletSelectionProps {
  // embedded is true when shown from an open wallet (the "Switch wallet"
  // route), enabling a Back action to the current wallet.
  embedded?: boolean;
}

export const WalletSelection = ({ embedded = false }: WalletSelectionProps) => {
  const navigate = useNavigate();
  const [wallets, setWallets] = useState<WalletInfo[] | null>(null);
  const [view, setView] = useState<'list' | 'create'>('list');
  const [editMode, setEditMode] = useState(false);
  const [switching, setSwitching] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Passphrase prompt shown only when a wallet refuses to open without one.
  const [passphraseFor, setPassphraseFor] = useState<string | null>(null);
  const [passphrase, setPassphrase] = useState('');

  // Rename / delete UI state.
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [deleting, setDeleting] = useState<string | null>(null);
  // Typed confirmation: the user must type DELETE to permanently purge a wallet.
  const [deleteConfirm, setDeleteConfirm] = useState('');

  const load = () => {
    listWallets()
      .then((res) => setWallets(res.wallets))
      .catch((err) => {
        console.error('listWallets failed:', err);
        setError('Failed to load wallets.');
        setWallets([]);
      });
  };

  useEffect(load, []);

  const doSelect = async (name: string, pass: string) => {
    setError(null);
    setSwitching(name);
    try {
      await selectWallet(name, pass);
      // Full reload so the layout re-evaluates with the new active wallet.
      window.location.assign('/wallet');
    } catch (err: any) {
      setSwitching(null);
      if (err?.response?.status === 401) {
        setPassphraseFor(name);
        setPassphrase('');
        return;
      }
      setError(err?.response?.data?.message || 'Failed to open wallet.');
    }
  };

  const handleRename = async (from: string) => {
    setError(null);
    try {
      await renameWallet(from, renameValue.trim());
      setRenaming(null);
      setRenameValue('');
      load();
    } catch (err: any) {
      setError(err?.response?.data?.message || 'Failed to rename wallet.');
    }
  };

  const handleDelete = async (name: string) => {
    setError(null);
    try {
      await deleteWallet(name);
      setDeleting(null);
      load();
    } catch (err: any) {
      setError(err?.response?.data?.message || 'Failed to delete wallet.');
    }
  };

  if (view === 'create') {
    return (
      <WalletSetup
        onComplete={() => window.location.assign('/wallet')}
        onCancel={() => setView('list')}
      />
    );
  }

  return (
    <div className="min-h-[60vh] flex items-center justify-center p-4">
      <div className="w-full max-w-2xl space-y-6">
        <div className="text-center space-y-2">
          <div className="flex justify-center">
            <div className="p-3 rounded-full bg-primary/10 border border-primary/20">
              <Wallet className="h-8 w-8 text-primary" />
            </div>
          </div>
          <h1 className="text-2xl font-bold">Select a wallet</h1>
          <p className="text-muted-foreground">Choose a wallet to open, or create a new one.</p>
        </div>

        {error && (
          <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg flex items-start gap-2">
            <AlertCircle className="h-5 w-5 text-red-500 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-red-500">{error}</p>
          </div>
        )}

        {wallets === null ? (
          <div className="flex justify-center py-8">
            <div className="animate-spin rounded-full h-10 w-10 border-4 border-primary border-t-transparent" />
          </div>
        ) : (
          <div className="space-y-3">
            {wallets.map((w) => (
              <div
                key={w.name}
                className="bg-gradient-card border border-border/50 rounded-xl p-4 flex items-center gap-4"
              >
                <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
                  <Wallet className="h-5 w-5 text-primary" />
                </div>

                <div className="flex-1 min-w-0">
                  {renaming === w.name ? (
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        className="flex-1 px-3 py-1 bg-background border border-border rounded focus:outline-none focus:ring-2 focus:ring-primary"
                        placeholder="New name"
                        autoFocus
                      />
                      <button onClick={() => handleRename(w.name)} className="p-1 text-green-500 hover:text-green-400">
                        <Check className="h-4 w-4" />
                      </button>
                      <button onClick={() => setRenaming(null)} className="p-1 text-muted-foreground hover:text-foreground">
                        <X className="h-4 w-4" />
                      </button>
                    </div>
                  ) : (
                    <>
                      <div className="flex items-center gap-2">
                        <span className="font-semibold truncate">{w.name}</span>
                        {w.active && <span className="text-xs px-2 py-0.5 rounded-full bg-primary/20 text-primary">Active</span>}
                      </div>
                      <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                        <span>{w.network}</span>
                        {w.isWatchOnly && (
                          <span className="flex items-center gap-1"><Eye className="h-3 w-3" /> Watch-only</span>
                        )}
                        {w.isPrivacy && (
                          <span className="flex items-center gap-1"><ShieldCheck className="h-3 w-3" /> Privacy</span>
                        )}
                        {!w.hasDb && <span className="text-warning">No database</span>}
                      </div>
                    </>
                  )}
                </div>

                {editMode ? (
                  <div className="flex items-center gap-1">
                    {!w.isDefault && !w.active && (
                      <button
                        onClick={() => {
                          setRenaming(w.name);
                          setRenameValue(w.name);
                        }}
                        className="p-2 text-muted-foreground hover:text-foreground"
                        title="Rename"
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                    )}
                    {!w.isDefault && !w.active && (
                      <button
                        onClick={() => {
                          setDeleting(w.name);
                          setDeleteConfirm('');
                        }}
                        className="p-2 text-red-500 hover:text-red-400"
                        title="Delete"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                ) : (
                  <button
                    onClick={() => doSelect(w.name, '')}
                    disabled={switching !== null || !w.hasDb}
                    className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
                  >
                    {switching === w.name ? 'Switching...' : w.active ? 'Open' : 'Select'}
                  </button>
                )}
              </div>
            ))}

            {wallets.length === 0 && (
              <p className="text-center text-muted-foreground py-4">No wallets found.</p>
            )}
          </div>
        )}

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {embedded && (
              <button
                onClick={() => navigate('/wallet')}
                className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowLeft className="h-4 w-4" /> Back
              </button>
            )}
            {wallets && wallets.length > 0 && (
              <button
                onClick={() => setEditMode((v) => !v)}
                className="text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                {editMode ? 'Done' : 'Edit'}
              </button>
            )}
          </div>
          <button
            onClick={() => setView('create')}
            className="flex items-center gap-2 px-4 py-2 border border-border rounded-lg hover:bg-muted/20 transition-colors"
          >
            <Plus className="h-4 w-4" /> Create new wallet
          </button>
        </div>
      </div>

      {/* Public passphrase prompt (shown only when required) */}
      {passphraseFor && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="bg-gradient-card border border-border/50 rounded-xl p-6 w-full max-w-md space-y-4">
            <h3 className="text-lg font-semibold">Public passphrase required</h3>
            <p className="text-sm text-muted-foreground">
              Wallet "{passphraseFor}" is encrypted. Enter its public passphrase to open it.
            </p>
            <input
              type="password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              className="w-full px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
              placeholder="Public passphrase"
              autoFocus
            />
            <div className="flex gap-3">
              <button
                onClick={() => {
                  setPassphraseFor(null);
                  setPassphrase('');
                }}
                className="flex-1 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  const name = passphraseFor;
                  setPassphraseFor(null);
                  doSelect(name, passphrase);
                }}
                className="flex-1 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors font-medium"
              >
                Open
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {deleting && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="bg-gradient-card border border-border/50 rounded-xl p-6 w-full max-w-md space-y-4">
            <h3 className="text-lg font-semibold text-red-500">Delete wallet "{deleting}"?</h3>
            <p className="text-sm text-muted-foreground">
              This permanently deletes wallet "{deleting}" and all of its data. This cannot be undone
              and the wallet is not backed up. Make sure you have its seed phrase before continuing.
            </p>
            <div className="space-y-1">
              <label className="text-xs font-medium text-muted-foreground">
                Type DELETE to confirm
              </label>
              <input
                type="text"
                value={deleteConfirm}
                onChange={(e) => setDeleteConfirm(e.target.value)}
                placeholder="DELETE"
                autoComplete="off"
                className="w-full px-3 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-red-500/50"
              />
            </div>
            <div className="flex gap-3">
              <button
                onClick={() => {
                  setDeleting(null);
                  setDeleteConfirm('');
                }}
                className="flex-1 py-2 border border-border rounded-lg hover:bg-background/50 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(deleting)}
                disabled={deleteConfirm !== 'DELETE'}
                className="flex-1 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600 transition-colors font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
