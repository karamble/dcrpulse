package rpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// pointBrclientdAt keeps the throwaway certs and aims the client at addr.
func pointBrclientdAt(t *testing.T, addr string) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	cfg := BrclientdCfg
	cfg.Host, cfg.StatusPort = host, port
	InitBrclientdConfig(cfg)
}

func TestAPostThatNeverConnectedIsNotReached(t *testing.T) {
	writeThrowawayCerts(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()
	pointBrclientdAt(t, addr)

	err = brclientdPostJSON(context.Background(), "/gc/x/message", map[string]string{"message": "m"})
	var unreached *BrclientdNotReachedError
	if !errors.As(err, &unreached) {
		t.Fatalf("a refused dial came back as %v", err)
	}
}

func TestAPostLostAfterConnectingIsNotNotReached(t *testing.T) {
	writeThrowawayCerts(t)
	cert, err := tls.LoadX509KeyPair(BrclientdCfg.ServerCertPath, BrclientdCfg.ClientKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	defer srv.Close()
	pointBrclientdAt(t, srv.Listener.Addr().String())

	err = brclientdPostJSON(context.Background(), "/gc/x/message", map[string]string{"message": "m"})
	var unreached *BrclientdNotReachedError
	var status *BrclientdStatusError
	if err == nil || errors.As(err, &unreached) || errors.As(err, &status) {
		t.Fatalf("a request brclientd may have received came back as %v", err)
	}
}
