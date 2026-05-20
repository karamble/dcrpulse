import { ShieldCheck, Circle } from 'lucide-react';

interface Props {
  running: boolean;
}

export const MixerStatusBadge = ({ running }: Props) => {
  if (running) {
    return (
      <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs bg-success/15 text-success border border-success/30">
        <ShieldCheck className="h-3.5 w-3.5" />
        Mixer running
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs bg-muted/20 text-muted-foreground border border-border">
      <Circle className="h-3.5 w-3.5" />
      Mixer stopped
    </span>
  );
};
