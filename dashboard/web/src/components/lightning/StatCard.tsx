const atomsPerDcr = 1e8;
export const fmtDcr = (atoms: number) => (atoms / atomsPerDcr).toFixed(8) + ' DCR';

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  sub?: string;
}

export const StatCard = ({ icon, label, value, sub }: StatCardProps) => (
  <div className="p-4 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-1">
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      {icon}
      <span>{label}</span>
    </div>
    <div className="text-lg font-semibold text-foreground break-all">{value}</div>
    {sub && <div className="text-xs text-muted-foreground">{sub}</div>}
  </div>
);
