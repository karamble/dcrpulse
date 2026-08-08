// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { timeAgo, toYMDTime } from '../../utils/date';

interface TimeAgoProps {
  timestamp: string;
  showFull?: boolean;
}

export const TimeAgo = ({ timestamp, showFull = false }: TimeAgoProps) => {
  const formatFull = (ts: string) => {
    const date = new Date(ts);
    return toYMDTime(date);
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
      {timeAgo(timestamp)}
    </span>
  );
};

