// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import packageJson from '../../package.json';

interface FooterProps {
  dcrdVersion?: string;
  dcrwalletVersion?: string;
  dcrlndVersion?: string;
  brclientdVersion?: string;
  bisonwVersion?: string;
  lastUpdate?: string;
  className?: string;
}

export const Footer = ({ dcrdVersion, dcrwalletVersion, dcrlndVersion, brclientdVersion, bisonwVersion, lastUpdate, className }: FooterProps) => {
  const dashboardVersion = packageJson.version;

  return (
    <footer className={`mt-8 ${className ?? ''}`}>
      <div className="flex flex-wrap items-center justify-center gap-2 text-sm text-muted-foreground">
        {lastUpdate && (
          <>
            <span>Last updated: {lastUpdate}</span>
            <span className="text-border">•</span>
          </>
        )}
        <a
          href="https://github.com/karamble/dcrpulse"
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-primary transition-colors"
        >
          Decred Pulse v{dashboardVersion}
        </a>
        {dcrdVersion && (
          <>
            <span className="text-border">•</span>
            <a
              href="https://github.com/decred/dcrd"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              dcrd {dcrdVersion}
            </a>
          </>
        )}
        {dcrwalletVersion && (
          <>
            <span className="text-border">•</span>
            <a
              href="https://github.com/decred/dcrwallet"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              dcrwallet {dcrwalletVersion}
            </a>
          </>
        )}
        {dcrlndVersion && (
          <>
            <span className="text-border">•</span>
            <a
              href="https://github.com/decred/dcrlnd"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              dcrlnd {dcrlndVersion}
            </a>
          </>
        )}
        {brclientdVersion && (
          <>
            <span className="text-border">•</span>
            <a
              href="https://github.com/karamble/brclientd"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              brclientd {brclientdVersion}
            </a>
          </>
        )}
        {bisonwVersion && (
          <>
            <span className="text-border">•</span>
            <a
              href="https://github.com/decred/dcrdex"
              target="_blank"
              rel="noopener noreferrer"
              className="hover:text-primary transition-colors"
            >
              bisonw {bisonwVersion}
            </a>
          </>
        )}
      </div>
    </footer>
  );
};

