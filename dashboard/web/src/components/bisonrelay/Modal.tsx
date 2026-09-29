// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, type FormEvent, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

// Modal is the shared Bison Relay dialog shell: rendered into <body> so no
// ancestor's backdrop-filter traps it, closed by Escape or a backdrop click
// unless busy. className styles the panel; as="form" makes it the form.
export const Modal = ({
  onClose,
  busy,
  className,
  as = 'div',
  onSubmit,
  children,
}: {
  onClose: () => void;
  busy?: boolean;
  className?: string;
  as?: 'div' | 'form';
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
  children: ReactNode;
}) => {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose, busy]);

  return createPortal(
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={() => {
        if (!busy) onClose();
      }}
    >
      {as === 'form' ? (
        <form onClick={(e) => e.stopPropagation()} onSubmit={onSubmit} className={className}>
          {children}
        </form>
      ) : (
        <div onClick={(e) => e.stopPropagation()} className={className}>
          {children}
        </div>
      )}
    </div>,
    document.body,
  );
};
