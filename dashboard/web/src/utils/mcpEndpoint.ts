// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// A bind address like 0.0.0.0 or :: means "listen on all interfaces"; it is
// not a valid target to CONNECT to. These helpers turn a bind into the host
// an agent should actually dial. The MCP ports are published on loopback, so
// a wildcard bind maps to 127.0.0.1; the user substitutes their own host,
// SSH tunnel, or public IP once they expose the port.

const WILDCARD_HOSTS = new Set(['', '0.0.0.0', '::', '[::]']);

export const connectHost = (host: string): string =>
  WILDCARD_HOSTS.has(host) ? '127.0.0.1' : host;

// isWildcardBind reports whether a bind host listens on all interfaces (for
// wording like "all interfaces" vs "local only").
export const isWildcardBind = (host: string): boolean => WILDCARD_HOSTS.has(host);

// connectFromBind splits a combined "host:port" bind into a connectable host
// and its port. Tolerates bracketed IPv6 ("[::]:8891").
export const connectFromBind = (bind: string): { host: string; port: string } => {
  const idx = bind.lastIndexOf(':');
  if (idx < 0) {
    return { host: connectHost(bind), port: '' };
  }
  const rawHost = bind.slice(0, idx);
  const port = bind.slice(idx + 1);
  return { host: connectHost(rawHost), port };
};
