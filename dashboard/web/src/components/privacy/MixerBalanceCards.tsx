import { ChevronRight, Coins, Shuffle } from 'lucide-react';

interface Props {
  unmixedBalance: number;
  mixedBalance: number;
  running: boolean;
}

const fmt = (v: number) => v.toFixed(8);

export const MixerBalanceCards = ({ unmixedBalance, mixedBalance, running }: Props) => (
  <div className="grid grid-cols-1 md:grid-cols-[1fr_auto_1fr] gap-3 items-center">
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-2">
        <Coins className="h-4 w-4 text-warning" />
        <span>Unmixed (source)</span>
      </div>
      <p className="text-2xl font-bold text-foreground">{fmt(unmixedBalance)} DCR</p>
    </div>
    <div
      className={`hidden md:flex items-center justify-center px-3 -space-x-3 ${
        running ? 'text-success' : 'text-muted-foreground'
      }`}
      aria-hidden="true"
    >
      <ChevronRight className={`h-7 w-7 ${running ? 'animate-pulse' : 'opacity-50'}`} />
      <ChevronRight
        className={`h-7 w-7 ${running ? 'animate-pulse [animation-delay:200ms]' : 'opacity-50'}`}
      />
      <ChevronRight
        className={`h-7 w-7 ${running ? 'animate-pulse [animation-delay:400ms]' : 'opacity-50'}`}
      />
    </div>
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-primary/30">
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-2">
        <Shuffle className="h-4 w-4 text-primary" />
        <span>Mixed</span>
      </div>
      <p className="text-2xl font-bold text-primary">{fmt(mixedBalance)} DCR</p>
    </div>
  </div>
);
