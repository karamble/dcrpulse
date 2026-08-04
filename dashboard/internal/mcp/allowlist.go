// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// ipAllowlist is an agent's parsed source-IP restriction. entries holds the
// canonical display/persist forms, prefixes the parsed forms used to match.
// A nil pointer or an empty prefix set means no restriction, which also covers
// agents persisted before the field existed.
type ipAllowlist struct {
	entries  []string
	prefixes []netip.Prefix
}

// parseIPEntry parses one allowlist entry: a single IPv4/IPv6 address or a
// CIDR prefix. v4-mapped forms are unmapped and zones stripped so entries
// match the normalized request address (netip.Prefix.Contains never crosses
// the mapped/unmapped boundary); prefixes are stored masked.
func parseIPEntry(s string) (string, netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return "", netip.Prefix{}, fmt.Errorf("invalid IP or CIDR entry %q", s)
		}
		if p.Addr().Is4In6() {
			// A mapped prefix shorter than /96 cannot be expressed as an
			// IPv4 range and would never match a normalized address.
			if p.Bits() < 96 {
				return "", netip.Prefix{}, fmt.Errorf("invalid IP or CIDR entry %q", s)
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		p = p.Masked()
		return p.String(), p, nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", netip.Prefix{}, fmt.Errorf("invalid IP or CIDR entry %q", s)
	}
	addr = addr.Unmap().WithZone("")
	return addr.String(), netip.PrefixFrom(addr, addr.BitLen()), nil
}

// ParseAllowedIPs validates and canonicalizes agent allowed-IP entries. Blank
// entries are skipped; an invalid entry rejects the whole set by name so a
// typo cannot silently widen access. An empty result means no restriction.
func ParseAllowedIPs(entries []string) ([]string, error) {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		canonical, _, err := parseIPEntry(e)
		if err != nil {
			return nil, err
		}
		out = append(out, canonical)
	}
	return out, nil
}

// setIPAllowlist parses and installs the agent's allowed-IP entries. The API
// only stores validated canonical forms, so an unparseable entry can appear
// only through a hand-edited config file; it is dropped with a warning so the
// operator knows the effective list shrank.
func (a *agent) setIPAllowlist(entries []string) {
	al := &ipAllowlist{}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		canonical, p, err := parseIPEntry(e)
		if err != nil {
			mcpLog.Warnf("Agent %s: dropping invalid allowed-IP entry %s",
				sanitizeLogField(a.name), sanitizeLogField(e))
			continue
		}
		al.entries = append(al.entries, canonical)
		al.prefixes = append(al.prefixes, p)
	}
	a.allowedIPs.Store(al)
}

// allowedIPEntries returns the agent's canonical allowed-IP entries; empty
// means any address. Always non-nil so JSON views render [] rather than null.
func (a *agent) allowedIPEntries() []string {
	if a == nil {
		return []string{}
	}
	if al := a.allowedIPs.Load(); al != nil && al.entries != nil {
		return al.entries
	}
	return []string{}
}

// remoteAllowed reports whether a request from remoteAddr ("host:port") passes
// the agent's allowed-IP list. Unrestricted agents always pass; restricted
// agents fail closed on any unparseable remote address.
func (a *agent) remoteAllowed(remoteAddr string) bool {
	if a == nil {
		return false
	}
	al := a.allowedIPs.Load()
	if al == nil || len(al.prefixes) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap().WithZone("")
	for _, p := range al.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// setAllowedIPs replaces an agent's allowed-IP entries. Unlike setDomains it
// needs no onChange: enforcement happens per request at auth time, not in the
// cached scoped server. Returns false if no such agent.
func (r *registry) setAllowedIPs(id string, entries []string) bool {
	r.mu.Lock()
	a := r.agents[id]
	if a != nil {
		a.setIPAllowlist(entries)
	}
	r.mu.Unlock()
	return a != nil
}

// DeniedAttempt is the most recent allowed-IP denial of an agent's token,
// surfaced to the dashboard so the operator can see (and allow) the address an
// agent is actually connecting from - behind Docker or a proxy that address is
// rarely the one the user expects. In-memory only; cleared by the agent's next
// successful request. The agent itself never sees it (the denial stays a
// generic 401).
type DeniedAttempt struct {
	IP string    `json:"ip"`
	At time.Time `json:"at"`
}

// recordDenied remembers the source address of a denied request in the form
// the allowlist would need (bare IP, unmapped, zone stripped), falling back to
// the raw string when it does not parse.
func (a *agent) recordDenied(remoteAddr string, now time.Time) {
	ip := remoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		ip = host
		if addr, err := netip.ParseAddr(host); err == nil {
			ip = addr.Unmap().WithZone("").String()
		}
	}
	a.lastDenied.Store(&DeniedAttempt{IP: ip, At: now})
}
