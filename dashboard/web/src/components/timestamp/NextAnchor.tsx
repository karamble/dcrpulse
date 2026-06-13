// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Clock } from 'lucide-react';

// dcrtime flushes pending digests into an anchor transaction hourly, on the
// hour. minutesToNextHour estimates the wait until the next flush.
function minutesToNextHour(): number {
  const now = new Date();
  const next = new Date(now);
  next.setHours(now.getHours() + 1, 0, 0, 0);
  return Math.max(1, Math.round((next.getTime() - now.getTime()) / 60000));
}

export const NextAnchor = () => {
  const [mins, setMins] = useState(minutesToNextHour);
  useEffect(() => {
    const t = setInterval(() => setMins(minutesToNextHour()), 10000);
    return () => clearInterval(t);
  }, []);
  return (
    <span
      className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
      title="dcrtime anchors pending digests to the Decred chain hourly, on the hour."
    >
      <Clock className="h-3.5 w-3.5" />
      Next anchor in ~{mins} min
    </span>
  );
};
