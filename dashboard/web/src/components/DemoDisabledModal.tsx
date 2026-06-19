// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Info, X } from 'lucide-react';

interface Props {
  isOpen: boolean;
  onClose: () => void;
}

// Shown whenever a visitor triggers an action that is disabled on a public demo
// instance (either proactively via useDemo, or after the backend returns a
// 403 demo_disabled which the api.ts interceptor maps here).
export const DemoDisabledModal = ({ isOpen, onClose }: Props) => {
  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <div className="flex items-center gap-2">
            <Info className="h-5 w-5 text-primary" />
            <h3 className="text-lg font-semibold">Disabled in the demo</h3>
          </div>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-3 text-sm text-muted-foreground">
          <p>
            This is a public demo of dcrpulse. Actions that move funds or change
            the setup are disabled so the instance stays safe for everyone.
          </p>
          <p>
            Run your own full stack to use every feature:{' '}
            <a
              href="https://github.com/karamble/dcrpulse"
              target="_blank"
              rel="noreferrer"
              className="text-primary hover:underline"
            >
              github.com/karamble/dcrpulse
            </a>
          </p>
        </div>

        <div className="flex justify-end p-6 pt-2 border-t border-border/50">
          <button
            onClick={onClose}
            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-semibold hover:opacity-90"
          >
            Got it
          </button>
        </div>
      </div>
    </div>
  );
};
