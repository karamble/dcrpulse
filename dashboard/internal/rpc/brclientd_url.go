// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
)

// ShortIDHex is a checked Bison Relay identifier: 64 hex digits, the
// zkidentity.ShortID brclientd decodes. A struct rather than a defined string
// type, so no other package can convert one into existence.
type ShortIDHex struct{ hex string }

// shortIDRe accepts either case, matching brclientd's hex.DecodeString.
var shortIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ErrBadShortID quotes no part of the rejected value; the text reaches logs
// and untrusted callers.
var ErrBadShortID = errors.New("brclientd: identifier is not 64 hex digits")

// ParseShortIDHex is the only way to obtain a usable ShortIDHex.
func ParseShortIDHex(s string) (ShortIDHex, error) {
	if !shortIDRe.MatchString(s) {
		return ShortIDHex{}, ErrBadShortID
	}
	return ShortIDHex{hex: s}, nil
}

// String returns the identifier as brclientd will read it.
func (id ShortIDHex) String() string { return id.hex }

// brPath is a request path assembled by this package. Defined and unexported,
// so an untyped string constant converts implicitly but a string variable does
// not: `"/gc/" + gcid` will not compile.
type brPath string

// brclientdRoute renders a path from a literal prefix, a checked identifier and
// a literal suffix. suffix may span segments: brclientd matches the action as
// one string after a single SplitN, so "/history/clear" keeps its slash.
func brclientdRoute(prefix brPath, id ShortIDHex, suffix brPath) (brPath, error) {
	// The zero ShortIDHex is constructible outside this package and has not
	// been through ParseShortIDHex; without this, "/gc" + "/" + "" is "/gc/".
	if !shortIDRe.MatchString(id.hex) {
		return "", ErrBadShortID
	}
	return prefix + "/" + brPath(id.hex) + suffix, nil
}

// brclientdPort selects one of brclientd's two HTTPS listeners: the status
// port carries the control surface, the setup port only answers before the
// daemon has an identity.
type brclientdPort int

const (
	statusPort brclientdPort = iota
	setupPort
)

// brclientdEndpoint renders the absolute URL for a brclientd request. The path
// goes in url.URL.Path and the query in RawQuery, so a '?' or '#' is encoded
// instead of starting a new URL component.
//
// No RawPath and no per-segment escaping: the only dynamic part is a
// ShortIDHex, whose 64 characters are all unreserved. Escaping would not help
// anyway, since brclientd reads the decoded r.URL.Path.
func brclientdEndpoint(p brclientdPort, path brPath, query map[string]string) (string, error) {
	return brclientdURL("https", p, path, query)
}

// brclientdWSEndpoint is brclientdEndpoint for the upgrade routes. Both keep
// the scheme out of the caller's hands, so it is spelled only here.
func brclientdWSEndpoint(p brclientdPort, path brPath) (string, error) {
	return brclientdURL("wss", p, path, nil)
}

func brclientdURL(scheme string, p brclientdPort, path brPath, query map[string]string) (string, error) {
	cfg := brclientdConfig()
	port := cfg.StatusPort
	if p == setupPort {
		port = cfg.Port
	}
	if cfg.Host == "" || port == "" {
		return "", errors.New("brclientd: host/port not configured")
	}
	if len(path) == 0 || path[0] != '/' {
		return "", fmt.Errorf("brclientd: route %q is not absolute", string(path))
	}
	u := url.URL{
		Scheme: scheme,
		// JoinHostPort brackets an IPv6 literal; "host:port" does not.
		Host: net.JoinHostPort(cfg.Host, port),
		Path: string(path),
	}
	if len(query) > 0 {
		vals := url.Values{}
		for k, v := range query {
			vals.Set(k, v)
		}
		u.RawQuery = vals.Encode()
	}
	return u.String(), nil
}

// brclientdPostJSONID and brclientdGetRawID keep an identifier-bearing call
// site to one line.

func brclientdPostJSONID(ctx context.Context, prefix brPath, id ShortIDHex, suffix brPath, body any) error {
	path, err := brclientdRoute(prefix, id, suffix)
	if err != nil {
		return err
	}
	return brclientdPostJSON(ctx, path, body)
}

func brclientdGetRawID(ctx context.Context, prefix brPath, id ShortIDHex, suffix brPath, query map[string]string) (json.RawMessage, error) {
	path, err := brclientdRoute(prefix, id, suffix)
	if err != nil {
		return nil, err
	}
	return brclientdGetRaw(ctx, path, query)
}
