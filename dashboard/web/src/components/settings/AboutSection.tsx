// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import {
  BookOpen,
  ExternalLink,
  Github,
  Globe,
  Info,
  Layers,
  MessageCircle,
  Search,
  Send,
} from 'lucide-react';
import { getDashboardData, getWalletStatus } from '../../services/api';
import packageJson from '../../../package.json';

const DCRPULSE_VERSION = `v${packageJson.version}`;

interface LinkRowProps {
  icon: React.ReactNode;
  title: string;
  href: string;
  subtitle?: string;
}

const LinkRow = ({ icon, title, href, subtitle }: LinkRowProps) => (
  <a
    href={href}
    target="_blank"
    rel="noopener noreferrer nofollow"
    className="flex items-center gap-3 p-3 rounded-lg bg-muted/10 border border-border/50 hover:bg-muted/20 transition-colors"
  >
    <div className="p-2 rounded-lg bg-primary/10 border border-primary/20 text-primary shrink-0">
      {icon}
    </div>
    <div className="flex-1 min-w-0">
      <div className="font-medium truncate">{title}</div>
      {subtitle && <div className="text-xs text-muted-foreground truncate">{subtitle}</div>}
    </div>
    <ExternalLink className="h-4 w-4 text-muted-foreground shrink-0" />
  </a>
);

interface VersionCellProps {
  label: string;
  value: string;
}

const VersionCell = ({ label, value }: VersionCellProps) => (
  <div className="p-3 rounded-lg bg-muted/10 border border-border/50">
    <div className="text-xs text-muted-foreground uppercase tracking-wide">{label}</div>
    <div className="font-mono mt-1">{value || '-'}</div>
  </div>
);

export const AboutSection = () => {
  const [walletVersion, setWalletVersion] = useState<string>('');
  const [nodeVersion, setNodeVersion] = useState<string>('');

  useEffect(() => {
    getWalletStatus()
      .then((s: any) => {
        if (s?.version) setWalletVersion(s.version);
      })
      .catch(() => {});
    getDashboardData()
      .then((d) => {
        if (d?.nodeStatus?.version) setNodeVersion(d.nodeStatus.version);
      })
      .catch(() => {});
  }, []);

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <Info className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Application</h3>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-sm">
          <VersionCell label="dcrpulse" value={DCRPULSE_VERSION} />
          <VersionCell label="dcrd" value={nodeVersion} />
          <VersionCell label="dcrwallet" value={walletVersion} />
        </div>
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <BookOpen className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Sources</h3>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <LinkRow
            icon={<Github className="h-4 w-4" />}
            title="dcrpulse on GitHub"
            subtitle="github.com/karamble/dcrpulse"
            href="https://github.com/karamble/dcrpulse"
          />
          <LinkRow
            icon={<Github className="h-4 w-4" />}
            title="Decred on GitHub"
            subtitle="github.com/decred"
            href="https://github.com/decred"
          />
          <LinkRow
            icon={<Globe className="h-4 w-4" />}
            title="Decred"
            subtitle="decred.org"
            href="https://decred.org"
          />
          <LinkRow
            icon={<BookOpen className="h-4 w-4" />}
            title="Documentation"
            subtitle="docs.decred.org"
            href="https://docs.decred.org/"
          />
          <LinkRow
            icon={<Layers className="h-4 w-4" />}
            title="Voting Service Providers"
            subtitle="decred.org/vsp"
            href="https://decred.org/vsp"
          />
          <LinkRow
            icon={<Search className="h-4 w-4" />}
            title="Block Explorer"
            subtitle="dcrdata.decred.org"
            href="https://dcrdata.decred.org"
          />
        </div>
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <div className="flex items-center gap-2">
          <MessageCircle className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold">Communications</h3>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <LinkRow
            icon={<MessageCircle className="h-4 w-4" />}
            title="Matrix Chat"
            subtitle="chat.decred.org"
            href="https://chat.decred.org/"
          />
          <LinkRow
            icon={<Send className="h-4 w-4" />}
            title="Telegram"
            subtitle="t.me/Decred"
            href="https://t.me/Decred"
          />
        </div>
      </div>
    </div>
  );
};
