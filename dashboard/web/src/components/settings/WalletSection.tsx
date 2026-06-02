// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { CheckCircle2, KeyRound, LogOut, Search, Wallet } from 'lucide-react';
import { changePassphrase, closeActiveWallet, discoverAddresses, getSettings } from '../../services/api';
import { ChangePassphraseModal } from './ChangePassphraseModal';
import { DiscoverAddressesModal } from './DiscoverAddressesModal';

export const WalletSection = () => {
  const [gapLimit, setGapLimit] = useState<number>(200);
  const [passModalOpen, setPassModalOpen] = useState(false);
  const [discoverModalOpen, setDiscoverModalOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  useEffect(() => {
    getSettings()
      .then((s) => {
        if (s.wallet?.gapLimit) setGapLimit(s.wallet.gapLimit);
      })
      .catch(() => {});
  }, []);

  const handleChangePassphrase = async (oldPass: string, newPass: string) => {
    await changePassphrase(oldPass, newPass);
    setPassModalOpen(false);
    setFeedback('Private passphrase changed.');
  };

  const handleDiscover = async (passphrase: string, gap: number) => {
    await discoverAddresses(passphrase, gap);
    setDiscoverModalOpen(false);
    setGapLimit(gap);
    setFeedback('Address discovery complete.');
  };

  const handleCloseWallet = async () => {
    setClosing(true);
    setFeedback(null);
    try {
      await closeActiveWallet();
      // Full reload so the layout returns to the wallet list.
      window.location.assign('/wallet');
    } catch (err) {
      console.error('closeActiveWallet failed:', err);
      setClosing(false);
      setFeedback('Failed to close wallet.');
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <Wallet className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Wallet</h3>
      </div>

      {feedback && (
        <div className="flex items-center gap-2 text-sm text-success">
          <CheckCircle2 className="h-4 w-4" />
          {feedback}
        </div>
      )}

      <div className="flex items-center justify-between p-4 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <KeyRound className="h-4 w-4 text-primary" />
            <span className="font-medium">Private passphrase</span>
          </div>
          <p className="text-sm text-muted-foreground">
            Rotate the wallet's signing passphrase. Required for tickets, sends, and unlocks.
          </p>
        </div>
        <button
          onClick={() => {
            setFeedback(null);
            setPassModalOpen(true);
          }}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors"
        >
          Change…
        </button>
      </div>

      <div className="flex items-center justify-between p-4 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <Search className="h-4 w-4 text-primary" />
            <span className="font-medium">Address discovery</span>
          </div>
          <p className="text-sm text-muted-foreground">
            Scan the chain for previously-used addresses under a chosen gap limit. Run after
            importing an xpub or restoring from a seed with high address activity. Current gap
            limit: <span className="font-mono">{gapLimit}</span>.
          </p>
        </div>
        <button
          onClick={() => {
            setFeedback(null);
            setDiscoverModalOpen(true);
          }}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors whitespace-nowrap"
        >
          Discover…
        </button>
      </div>

      <div className="flex items-center justify-between p-4 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <LogOut className="h-4 w-4 text-primary" />
            <span className="font-medium">Close wallet</span>
          </div>
          <p className="text-sm text-muted-foreground">
            Close the active wallet and return to the wallet list. Other wallets stay available to
            open.
          </p>
        </div>
        <button
          onClick={handleCloseWallet}
          disabled={closing}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium transition-colors disabled:opacity-50 whitespace-nowrap"
        >
          {closing ? 'Closing…' : 'Close wallet'}
        </button>
      </div>

      <ChangePassphraseModal
        isOpen={passModalOpen}
        onSubmit={handleChangePassphrase}
        onClose={() => setPassModalOpen(false)}
      />
      <DiscoverAddressesModal
        isOpen={discoverModalOpen}
        defaultGapLimit={gapLimit}
        onSubmit={handleDiscover}
        onClose={() => setDiscoverModalOpen(false)}
      />
    </div>
  );
};
