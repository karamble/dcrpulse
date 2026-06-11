import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { setupPrivacy } from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';

interface Props {
  onConfigured: () => void;
}

export const PrivacySetupCard = ({ onConfigured }: Props) => {
  const [modalOpen, setModalOpen] = useState(false);

  const handleSetup = async (passphrase: string) => {
    await setupPrivacy(passphrase);
    setModalOpen(false);
    onConfigured();
  };

  return (
    <div className="p-8 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 text-center space-y-4">
      <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-primary/10 border border-primary/20">
        <ShieldCheck className="h-8 w-8 text-primary" />
      </div>
      <h2 className="text-2xl font-semibold">Set up privacy</h2>
      <p className="max-w-xl mx-auto text-sm text-muted-foreground">
        The CoinJoin mixer needs two accounts: an <span className="font-semibold">unmixed</span> account
        that holds funds awaiting mixing, and a <span className="font-semibold">mixed</span> account
        that receives the privacy-improved outputs. Once set up, funds in the unmixed account are
        gradually mixed peer-to-peer over the Decred network with other participants.
      </p>
      <p className="max-w-xl mx-auto text-xs text-muted-foreground">
        After mixing, only spend from the mixed account - sending from the unmixed account can
        compromise the privacy you just gained.
      </p>
      <button
        onClick={() => setModalOpen(true)}
        className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all inline-flex items-center gap-2"
      >
        <ShieldCheck className="h-5 w-5" />
        Set up privacy
      </button>

      <PassphraseModal
        isOpen={modalOpen}
        title="Set up privacy"
        description="Enter your wallet passphrase to create the 'mixed' and 'unmixed' accounts."
        submitLabel="Create accounts"
        busyLabel="Creating…"
        onSubmit={handleSetup}
        onClose={() => setModalOpen(false)}
      />
    </div>
  );
};
