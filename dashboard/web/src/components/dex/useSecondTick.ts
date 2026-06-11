// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';

// useSecondTick re-renders the caller on a fixed interval (default 1s) so
// relative "age" labels advance without waiting for a data refresh. Returns a
// counter that changes each tick; pass active=false to pause the interval.
export const useSecondTick = (active = true, ms = 1000): number => {
  const [n, setN] = useState(0);
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => setN((x) => x + 1), ms);
    return () => window.clearInterval(id);
  }, [active, ms]);
  return n;
};
