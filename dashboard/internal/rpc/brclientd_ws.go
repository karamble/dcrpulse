// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// BrclientdWSClient maintains a persistent JSON-RPC 2.0 over WebSocket
// connection to brclientd's clientrpc /ws endpoint and demultiplexes both
// unary calls and server-streamed responses by request ID. Streaming
// subscriptions are remembered and re-established automatically across
// reconnects so consumers (e.g. the PM/KX event broadcaster) do not need
// to handle disconnects themselves.
type BrclientdWSClient struct {
	mu      sync.Mutex
	conn    *websocket.Conn
	writeMu sync.Mutex

	pending     map[string]chan inboundMsg
	streams     map[string]chan inboundMsg
	subscribers []*subscription
	subMu       sync.RWMutex

	nextID atomic.Int64
	closed chan struct{}
}

type subscription struct {
	method    string
	params    any
	onEvent   func(json.RawMessage)
	cancelled atomic.Bool
}

type inboundMsg struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type outboundMsg struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      string  `json:"id"`
	Method  string  `json:"method"`
	Params  any     `json:"params,omitempty"`
}

var (
	wsClient *BrclientdWSClient
	wsOnce   sync.Once
)

// BrclientdWS returns the process-wide WebSocket client. Lazily constructs
// on first call.
func BrclientdWS() *BrclientdWSClient {
	wsOnce.Do(func() {
		wsClient = &BrclientdWSClient{
			pending: make(map[string]chan inboundMsg),
			streams: make(map[string]chan inboundMsg),
			closed:  make(chan struct{}),
		}
	})
	return wsClient
}

// Run dials brclientd and keeps the connection alive with reconnect
// backoff. Blocks until ctx is done. Re-subscribes registered streams on
// every successful reconnect.
func (c *BrclientdWSClient) Run(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.dialAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("brclientd-ws: %v (reconnect in %s)", err, backoff)
		}
		select {
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *BrclientdWSClient) dialAndServe(ctx context.Context) error {
	if BrclientdCfg.Host == "" || BrclientdCfg.Port == "" {
		return errors.New("brclientd: host/port not configured")
	}
	tlsCfg, err := loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "wss", Host: BrclientdCfg.Host + ":" + BrclientdCfg.Port, Path: "/ws"}
	dialer := &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	const (
		pingInterval = 20 * time.Second
		readDeadline = 60 * time.Second
	)
	_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readDeadline))
	})

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		conn.Close()
		c.failAllPending(errors.New("connection closed"))
	}()

	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-t.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	log.Printf("brclientd-ws: connected to %s", u.String())

	c.subMu.RLock()
	subs := make([]*subscription, 0, len(c.subscribers))
	subs = append(subs, c.subscribers...)
	c.subMu.RUnlock()
	for _, s := range subs {
		if !s.cancelled.Load() {
			if err := c.openStream(s); err != nil {
				log.Printf("brclientd-ws: re-subscribe %s: %v", s.method, err)
			}
		}
	}

	readErr := make(chan error, 1)
	go func() { readErr <- c.readLoop(conn) }()

	select {
	case err := <-readErr:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *BrclientdWSClient) readLoop(conn *websocket.Conn) error {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg inboundMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("brclientd-ws: decode: %v", err)
			continue
		}
		id := idAsString(msg.ID)
		if id == "" {
			continue
		}
		c.mu.Lock()
		pendingCh, hasPending := c.pending[id]
		streamCh, hasStream := c.streams[id]
		c.mu.Unlock()
		switch {
		case hasPending:
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			select {
			case pendingCh <- msg:
			default:
			}
		case hasStream:
			select {
			case streamCh <- msg:
			default:
			}
		}
	}
}

func (c *BrclientdWSClient) failAllPending(err error) {
	c.mu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	for id, ch := range c.streams {
		close(ch)
		delete(c.streams, id)
	}
	c.mu.Unlock()
}

// Call performs a unary JSON-RPC over the WS connection.
func (c *BrclientdWSClient) Call(ctx context.Context, method string, params, result any) error {
	id := strconv.FormatInt(c.nextID.Add(1), 10)
	respCh := make(chan inboundMsg, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.send(outboundMsg{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return err
	}
	select {
	case resp, ok := <-respCh:
		if !ok {
			return errors.New("connection closed")
		}
		if resp.Error != nil {
			return fmt.Errorf("brclientd-ws %s: code %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("decode %s: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe registers a streaming subscription. onEvent fires for every
// event delivered on the stream; cancellation runs the returned func.
// The subscription is replayed automatically on every reconnect, and
// queued for the next reconnect if the WS is not connected yet.
func (c *BrclientdWSClient) Subscribe(method string, params any, onEvent func(json.RawMessage)) (cancel func(), err error) {
	s := &subscription{method: method, params: params, onEvent: onEvent}
	c.subMu.Lock()
	c.subscribers = append(c.subscribers, s)
	c.subMu.Unlock()

	c.mu.Lock()
	connected := c.conn != nil
	c.mu.Unlock()
	if connected {
		if err := c.openStream(s); err != nil {
			log.Printf("brclientd-ws: open stream %s: %v (will retry on reconnect)", method, err)
		}
	}

	return func() {
		s.cancelled.Store(true)
		c.subMu.Lock()
		for i, x := range c.subscribers {
			if x == s {
				c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
				break
			}
		}
		c.subMu.Unlock()
	}, nil
}

func (c *BrclientdWSClient) openStream(s *subscription) error {
	id := strconv.FormatInt(c.nextID.Add(1), 10)
	ch := make(chan inboundMsg, 32)
	c.mu.Lock()
	c.streams[id] = ch
	c.mu.Unlock()

	if err := c.send(outboundMsg{JSONRPC: "2.0", ID: id, Method: s.method, Params: s.params}); err != nil {
		c.mu.Lock()
		delete(c.streams, id)
		c.mu.Unlock()
		return err
	}
	go func() {
		for msg := range ch {
			if s.cancelled.Load() {
				return
			}
			if msg.Error != nil {
				log.Printf("brclientd-ws stream %s: code %d: %s", s.method, msg.Error.Code, msg.Error.Message)
				continue
			}
			if len(msg.Result) > 0 {
				s.onEvent(msg.Result)
			}
		}
	}()
	return nil
}

func (c *BrclientdWSClient) send(msg outboundMsg) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("not connected")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(msg)
}

func idAsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case json.Number:
		return string(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
