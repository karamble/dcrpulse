// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

interface TimeAgoProps {
  timestamp: string;
  showFull?: boolean;
}

export const TimeAgo = ({ timestamp, showFull = false }: TimeAgoProps) => {
  const formatTimeAgo = (ts: string) => {
    const date = new Date(ts);
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
    
    if (seconds < 60) return `${seconds}s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
  };

  const formatFull = (ts: string) => {
    const date = new Date(ts);
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      timeZoneName: 'short'
    });
  };

  if (showFull) {
    return (
      <span className="text-muted-foreground" title={formatFull(timestamp)}>
        {formatFull(timestamp)}
      </span>
    );
  }

  return (
    <span className="text-muted-foreground" title={formatFull(timestamp)}>
      {formatTimeAgo(timestamp)}
    </span>
  );
};

