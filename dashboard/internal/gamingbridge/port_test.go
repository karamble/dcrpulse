package gamingbridge

import (
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/karamble/dcrgaming-sdk/pkg/gaming/gamingpb"
)

// waitAddr waits for the bridge's port to be open (want true) or closed.
func waitAddr(t *testing.T, s *Server, want bool) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if addr := s.Addr(); (addr != "") == want {
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the bridge port did not become open=%v within 3s", want)
	return ""
}

func TestTheBridgePortClosesWhileSwitchedOff(t *testing.T) {
	for name, set := range map[string]func(*bridgeRig, bool){
		"bridge switch": func(r *bridgeRig, on bool) { r.enabled.Store(on) },
		"app password":  func(r *bridgeRig, on bool) { r.appPassword.Store(on) },
	} {
		t.Run(name, func(t *testing.T) {
			r := newBridgeRig(t)
			creds := r.register(t, "poker")
			old := waitAddr(t, r.srv, true)

			set(r, false)
			waitAddr(t, r.srv, false)
			if conn, err := net.DialTimeout("tcp", old, time.Second); err == nil {
				conn.Close()
				t.Fatalf("something still accepts connections on %s while the bridge is off", old)
			}

			set(r, true)
			r.Addr = waitAddr(t, r.srv, true)
			ctx, cancel := callCtx(t)
			defer cancel()
			if _, err := r.dial(t, creds).ChainTip(ctx, &gamingpb.ChainTipRequest{}); err != nil {
				t.Fatalf("the bridge did not answer after being switched back on: %v", err)
			}
		})
	}
}

func TestFinancialWorkerRunsWhileThePortIsClosed(t *testing.T) {
	r := newBridgeRig(t)
	<-r.workerStarted
	r.enabled.Store(false)
	waitAddr(t, r.srv, false)
	select {
	case <-r.workerStopped:
		t.Fatal("switching the bridge off stopped the financial worker")
	case <-time.After(1500 * time.Millisecond):
	}
}

func TestConnectionsBeyondTheCapWaitForAFreeSlot(t *testing.T) {
	old := maxBridgeConns
	maxBridgeConns = 1
	t.Cleanup(func() { maxBridgeConns = old })
	r := newBridgeRig(t)
	addr := waitAddr(t, r.srv, true)

	first, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	time.Sleep(100 * time.Millisecond)

	second, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	tc := tls.Client(second, &tls.Config{InsecureSkipVerify: true})
	_ = second.SetDeadline(time.Now().Add(time.Second))
	// Served, the handshake would fail fast for want of a client certificate;
	// held at the cap, nothing answers until the deadline.
	var nerr net.Error
	if err := tc.Handshake(); !errors.As(err, &nerr) || !nerr.Timeout() {
		t.Fatalf("a connection past the cap was served: %v", err)
	}
}
