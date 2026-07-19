interface Props {
  score: number;
  heuristic?: string;
}

export const ScoreMeter = ({ score, heuristic }: Props) => (
  <span
    className="inline-flex items-center gap-1.5"
    title={`Autopilot score${heuristic ? ` (${heuristic})` : ''}: ${score.toFixed(2)}`}
  >
    <span className="w-12 h-1 rounded-full bg-muted/30 overflow-hidden">
      <span
        className="block h-full bg-primary/60"
        style={{ width: `${Math.max(0, Math.min(100, score * 100))}%` }}
      />
    </span>
    <span className="text-[10px] font-mono text-muted-foreground">{score.toFixed(2)}</span>
  </span>
);
