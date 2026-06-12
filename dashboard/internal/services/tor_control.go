// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

// torControlEndpoint returns host:port of the Tor control port from env.
func torControlEndpoint() (string, bool) {
	ip := os.Getenv("TOR_PROXY_IP")
	if ip == "" {
		return "", false
	}
	port := os.Getenv("TOR_CONTROL_PORT")
	if port == "" {
		port = "9051"
	}
	return net.JoinHostPort(ip, port), true
}

type torControlConn struct {
	conn net.Conn
	r    *bufio.Reader
}

// dialTorControl opens an authenticated control-port connection. Auth is plain
// cookie auth (AUTHENTICATE <hex(cookie)>): the port is cookie-gated, only on
// the isolated Docker network, and never published to the host.
func dialTorControl() (*torControlConn, error) {
	addr, ok := torControlEndpoint()
	if !ok {
		return nil, fmt.Errorf("tor control endpoint not configured")
	}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	tc := &torControlConn{conn: conn, r: bufio.NewReader(conn)}
	if err := tc.authenticate(); err != nil {
		conn.Close()
		return nil, err
	}
	return tc, nil
}

func (tc *torControlConn) close() { _ = tc.conn.Close() }

func (tc *torControlConn) authenticate() error {
	cookie, err := os.ReadFile(filepath.Join(config.TorDataDir, "control_auth_cookie"))
	if err != nil {
		return fmt.Errorf("read control cookie: %w", err)
	}
	reply, err := tc.cmd("AUTHENTICATE " + hex.EncodeToString(cookie))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(reply, "250") {
		return fmt.Errorf("tor auth rejected: %s", reply)
	}
	return nil
}

func (tc *torControlConn) cmd(s string) (string, error) {
	if _, err := tc.conn.Write([]byte(s + "\r\n")); err != nil {
		return "", err
	}
	return tc.readReply()
}

// readReply reads a control-port reply, handling the single-line "250 ",
// continuation "250-", and data "250+...\r\n.\r\n" forms. Returns all lines
// joined by newline.
func (tc *torControlConn) readReply() (string, error) {
	var lines []string
	for {
		line, err := tc.r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		lines = append(lines, line)
		if len(line) < 4 {
			continue
		}
		switch line[3] {
		case ' ':
			return strings.Join(lines, "\n"), nil
		case '+':
			for {
				d, err := tc.r.ReadString('\n')
				if err != nil {
					return "", err
				}
				d = strings.TrimRight(d, "\r\n")
				if d == "." {
					break
				}
				lines = append(lines, d)
			}
		}
	}
}

// getInfo returns the value for a single-line GETINFO key.
func (tc *torControlConn) getInfo(key string) string {
	reply, err := tc.cmd("GETINFO " + key)
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(reply, "\n") {
		for _, p := range []string{"250-", "250+", "250 "} {
			ln = strings.TrimPrefix(ln, p)
		}
		if strings.HasPrefix(ln, key+"=") {
			return strings.TrimPrefix(ln, key+"=")
		}
	}
	return ""
}

// TorControlSnapshot connects to the control port and returns the live status.
func TorControlSnapshot() types.TorControlInfo {
	info := types.TorControlInfo{}
	tc, err := dialTorControl()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer tc.close()
	info.Reachable = true

	bs := tc.getInfo("status/bootstrap-phase")
	info.BootstrapPct = torKVInt(bs, "PROGRESS")
	info.BootstrapTag = torKVStr(bs, "TAG")
	info.Version = tc.getInfo("version")
	info.BytesRead, _ = strconv.ParseInt(strings.TrimSpace(tc.getInfo("traffic/read")), 10, 64)
	info.BytesWritten, _ = strconv.ParseInt(strings.TrimSpace(tc.getInfo("traffic/written")), 10, 64)
	info.Circuits = tc.countCircuits()
	return info
}

// countCircuits counts BUILT circuits in the circuit-status data block.
func (tc *torControlConn) countCircuits() int {
	reply, err := tc.cmd("GETINFO circuit-status")
	if err != nil {
		return 0
	}
	n := 0
	for _, ln := range strings.Split(reply, "\n") {
		if strings.Contains(ln, " BUILT ") {
			n++
		}
	}
	return n
}

// TorNewIdentity asks Tor to use fresh circuits (SIGNAL NEWNYM).
func TorNewIdentity() error {
	tc, err := dialTorControl()
	if err != nil {
		return err
	}
	defer tc.close()
	reply, err := tc.cmd("SIGNAL NEWNYM")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(reply, "250") {
		return fmt.Errorf("newnym rejected: %s", reply)
	}
	return nil
}

func torKVInt(s, key string) int {
	n, _ := strconv.Atoi(torKVStr(s, key))
	return n
}

func torKVStr(s, key string) string {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, key+"=") {
			return strings.Trim(strings.TrimPrefix(f, key+"="), "\"")
		}
	}
	return ""
}
