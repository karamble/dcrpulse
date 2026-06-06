// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { avatarDataUrl, colorForUid } from './bisonrelayAvatar';

export const AuthorAvatar = ({
  uid,
  nick,
  avatarB64,
  size,
}: {
  uid: string;
  nick: string;
  avatarB64?: string;
  size: 'sm' | 'md' | 'lg';
}) => {
  const dim =
    size === 'sm' ? 'h-7 w-7 text-[11px]' : size === 'lg' ? 'h-16 w-16 text-2xl' : 'h-10 w-10 text-sm';
  const dataUrl = avatarDataUrl(avatarB64);
  if (dataUrl) {
    return (
      <img
        src={dataUrl}
        alt=""
        className={`shrink-0 rounded-full object-cover bg-muted/30 ${dim}`}
      />
    );
  }
  const initial = nick.trim().charAt(0).toUpperCase() || '?';
  const bg = colorForUid(uid || nick);
  return (
    <span
      className={`shrink-0 rounded-full flex items-center justify-center font-semibold text-white ${bg} ${dim}`}
    >
      {initial}
    </span>
  );
};
