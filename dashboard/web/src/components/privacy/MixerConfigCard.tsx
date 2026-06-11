import { Settings } from 'lucide-react';

interface Props {
  mixedAccount: number;
  changeAccount: number;
}

export const MixerConfigCard = ({ mixedAccount, changeAccount }: Props) => (
  <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
    <div className="flex items-center gap-2 mb-3">
      <Settings className="h-4 w-4 text-muted-foreground" />
      <h3 className="text-sm font-semibold">Configuration</h3>
    </div>
    <dl className="grid grid-cols-1 md:grid-cols-2 gap-y-2 gap-x-6 text-sm">
      <div className="flex justify-between">
        <dt className="text-muted-foreground">Mixed account</dt>
        <dd className="font-mono">mixed · #{mixedAccount}</dd>
      </div>
      <div className="flex justify-between">
        <dt className="text-muted-foreground">Unmixed account</dt>
        <dd className="font-mono">unmixed · #{changeAccount}</dd>
      </div>
      <div className="flex justify-between">
        <dt className="text-muted-foreground">Branch</dt>
        <dd className="font-mono">0 (external)</dd>
      </div>
      <div className="flex justify-between">
        <dt className="text-muted-foreground">Network</dt>
        <dd>Peer-to-peer via dcrd</dd>
      </div>
    </dl>
  </div>
);
