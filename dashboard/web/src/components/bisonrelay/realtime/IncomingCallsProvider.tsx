// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext, useContext, ReactNode } from 'react';
import {
  IncomingCallsState,
  useIncomingCallsState,
} from '../../../hooks/useIncomingCalls';

const IncomingCallsContext = createContext<IncomingCallsState | null>(null);

// IncomingCallsProvider holds the one list of calls waiting to be answered. The
// pill and the Realtime tab's banner both offer the same invitations, so they
// have to answer and dismiss from the same state; separate copies left one of
// them advertising a call the user had already taken.
export const IncomingCallsProvider = ({ children }: { children: ReactNode }) => {
  const state = useIncomingCallsState();
  return (
    <IncomingCallsContext.Provider value={state}>
      {children}
    </IncomingCallsContext.Provider>
  );
};

export const useIncomingCalls = (): IncomingCallsState => {
  const ctx = useContext(IncomingCallsContext);
  if (!ctx) {
    throw new Error('useIncomingCalls must be used inside IncomingCallsProvider');
  }
  return ctx;
};
