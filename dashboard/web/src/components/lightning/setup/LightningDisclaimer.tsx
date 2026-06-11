import { AlertTriangle, Zap } from 'lucide-react';

interface Props {
  onAcknowledge: () => void;
}

export const LightningDisclaimer = ({ onAcknowledge }: Props) => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-6 max-w-2xl">
    <div className="flex items-center gap-3">
      <Zap className="h-6 w-6 text-warning" />
      <h2 className="text-xl font-semibold">Enable Lightning Network</h2>
    </div>

    <p className="text-sm text-muted-foreground">
      Lightning is a layer-2 payment network for Decred. Activating it
      creates a dedicated wallet account, starts the dcrlnd daemon, and
      gives you channels for fast, low-fee payments. Before continuing,
      please read the following carefully.
    </p>

    <div className="rounded-lg bg-warning/10 border border-warning/30 p-4 space-y-2">
      <div className="flex items-center gap-2 text-warning font-semibold text-sm">
        <AlertTriangle className="h-4 w-4" />
        Lightning is experimental
      </div>
      <ul className="text-xs text-foreground/80 list-disc list-inside space-y-1">
        <li>
          Funds locked into Lightning channels are at risk of partial or
          full loss if a counterparty misbehaves and you do not have a
          recent channel backup.
        </li>
        <li>
          You MUST back up your Static Channel Backup (SCB) file after
          opening any channel. Without it, channel-state recovery is
          impossible.
        </li>
        <li>
          Force-closing a channel ties up funds for a multi-day timelock.
        </li>
        <li>
          The Lightning wallet uses its own internal seed managed by
          dcrlnd. The dashboard does not display this seed; recovery
          happens via the SCB backup mechanism.
        </li>
      </ul>
    </div>

    <p className="text-sm text-muted-foreground">
      Continuing will create a new dcrwallet account named{' '}
      <code className="px-1 py-0.5 rounded bg-muted/30 text-xs font-mono">
        lightning
      </code>{' '}
      and prompt for your wallet passphrase. The dedicated account
      isolates Lightning funds from your main on-chain balance.
    </p>

    <div className="flex justify-end gap-2 pt-2">
      <button
        type="button"
        onClick={onAcknowledge}
        className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm"
      >
        I Understand, Continue
      </button>
    </div>
  </div>
);
