// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
  type ReactNode,
} from 'react';
import { getDemoStatus, setDemoDisabledHandler } from '../services/api';
import { DemoDisabledModal } from './DemoDisabledModal';

interface DemoContextValue {
  // demoMode is true when the backend runs read-only (DEMO_MODE=true).
  demoMode: boolean;
  // Opens the "Disabled in the demo" modal; use to proactively guard buttons.
  showDemoDisabledModal: () => void;
}

const DemoContext = createContext<DemoContextValue>({
  demoMode: false,
  showDemoDisabledModal: () => {},
});

export const useDemo = () => useContext(DemoContext);

// DemoProvider fetches the demo flag once on load, registers the api.ts
// interceptor handler so any blocked backend call surfaces the modal, and hosts
// the single modal instance for the whole app.
export const DemoProvider = ({ children }: { children: ReactNode }) => {
  const [demoMode, setDemoMode] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const showDemoDisabledModal = useCallback(() => setModalOpen(true), []);

  useEffect(() => {
    let active = true;
    getDemoStatus()
      .then((s) => {
        if (active) setDemoMode(s.demo);
      })
      .catch(() => {
        // No demo-status endpoint / error => treat as a normal instance.
      });
    setDemoDisabledHandler(() => setModalOpen(true));
    return () => {
      active = false;
      setDemoDisabledHandler(null);
    };
  }, []);

  return (
    <DemoContext.Provider value={{ demoMode, showDemoDisabledModal }}>
      {children}
      <DemoDisabledModal isOpen={modalOpen} onClose={() => setModalOpen(false)} />
    </DemoContext.Provider>
  );
};
