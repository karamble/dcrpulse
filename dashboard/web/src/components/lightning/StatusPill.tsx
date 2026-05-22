import type { ReactNode } from 'react';

export type StatusTone = 'success' | 'warning' | 'destructive' | 'info' | 'muted';

const toneClasses: Record<StatusTone, string> = {
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  destructive: 'bg-destructive/15 text-destructive',
  info: 'bg-primary/15 text-primary',
  muted: 'bg-muted/30 text-muted-foreground',
};

interface Props {
  label: string;
  tone: StatusTone;
  icon?: ReactNode;
}

export const StatusPill = ({ label, tone, icon }: Props) => (
  <span
    className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs ${toneClasses[tone]}`}
  >
    {icon}
    {label}
  </span>
);
