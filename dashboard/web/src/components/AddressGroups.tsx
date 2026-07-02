// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Mirrors the hardware wallet's verification grid: 5-char groups on a
// single line with the outer groups emphasized. Both screens share the same
// visual shape so a human can compare them at a glance.

const chunk5 = (value: string): string[] => {
  const groups: string[] = [];
  for (let i = 0; i < value.length; i += 5) groups.push(value.slice(i, i + 5));
  return groups;
};

interface AddressGroupsProps {
  value: string;
  className?: string;
  // change tints the whole address so a change-to-own-wallet output reads
  // differently from a recipient at a glance.
  variant?: 'default' | 'change';
}

export const AddressGroups = ({ value, className = '', variant = 'default' }: AddressGroupsProps) => {
  const clean = value.trim();
  const body = variant === 'change' ? 'text-warning/80' : undefined;
  const outerClass =
    variant === 'change' ? 'text-warning font-semibold' : 'text-success font-semibold';
  if (clean.length <= 10) {
    return <span className={`font-mono ${body ?? ''} ${className}`}>{clean}</span>;
  }
  const groups = chunk5(clean);
  return (
    <span className={`font-mono ${className}`} aria-label={clean}>
      <span aria-hidden="true" className="inline-flex gap-x-2 whitespace-nowrap">
        {groups.map((g, gi) => {
          const outer = gi === 0 || gi === groups.length - 1;
          return (
            <span key={gi} className={outer ? outerClass : body}>
              {g}
            </span>
          );
        })}
      </span>
    </span>
  );
};

interface KeyEndsProps {
  value: string;
  chars?: number;
  className?: string;
}

// First/last characters of a long key (dpub) with the middle elided, the way
// the device presents extended keys for verification.
export const KeyEnds = ({ value, chars = 12, className = '' }: KeyEndsProps) => {
  const clean = value.replace(/\s+/g, '');
  if (clean.length <= chars * 2 + 3) {
    return <span className={`font-mono break-all ${className}`}>{clean}</span>;
  }
  return (
    <span className={`font-mono break-all ${className}`} aria-label={clean}>
      <span className="text-success font-semibold">{clean.slice(0, chars)}</span>
      <span className="text-muted-foreground px-1">…</span>
      <span className="text-success font-semibold">{clean.slice(-chars)}</span>
    </span>
  );
};
