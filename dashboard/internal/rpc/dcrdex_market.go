// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// bisonw's /ws carries the public market feed. The order book (with recent
// matches) and candlesticks are pushed as notifications after a loadmarket /
// loadcandles subscription; the RPC server has no request/response route for
// them. These helpers open a one-shot subscription, read the first matching
// snapshot, and close, turning the streaming feed into a unary call.
//
// msgjson message types (decred.org/dcrdex/dex/msgjson): Request = 1,
// Notification = 3.
const (
	dexMsgRequest      = 1
	dexMsgNotification = 3
)

// dexWSMessage mirrors the msgjson.Message envelope on the wire.
type dexWSMessage struct {
	Type    int             `json:"type"`
	Route   string          `json:"route,omitempty"`
	ID      uint64          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// dexMarketLoad is the loadmarket payload, extended with Dur for loadcandles
// (client/websocket marketLoad / candlesLoad).
type dexMarketLoad struct {
	Host  string `json:"host"`
	Base  uint32 `json:"base"`
	Quote uint32 `json:"quote"`
	Dur   string `json:"dur,omitempty"`
}

// dexBookUpdate mirrors core.BookUpdate: a feed notification repeats the action
// and carries the action-specific snapshot in Payload (the order book for
// "book", the candle set for "candles").
type dexBookUpdate struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// dexMarketSnapshot dials bisonw's RPC /ws, subscribes to the market with
// loadmarket (and loadcandles when dur is set), and returns the first "book" and
// "candles" notification payloads. The feed is public, so no app password or DEX
// unlock is required. book is the core.MarketOrderBook (base, quote, book);
// candles is the core.CandlesPayload (dur, ms, candles).
func dexMarketSnapshot(ctx context.Context, host string, base, quote uint32, dur string) (book, candles json.RawMessage, err error) {
	c, err := DcrdexClient()
	if err != nil {
		return nil, nil, err
	}
	conn, resp, err := DialDcrdexWS(ctx, c)
	if err != nil {
		if resp != nil {
			return nil, nil, fmt.Errorf("dcrdex ws: %w (http %d)", err, resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("dcrdex ws: %w", err)
	}
	defer conn.Close()

	send := func(id uint64, route string, p dexMarketLoad) error {
		pb, _ := json.Marshal(p)
		mb, _ := json.Marshal(dexWSMessage{Type: dexMsgRequest, Route: route, ID: id, Payload: pb})
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.TextMessage, mb)
	}

	load := dexMarketLoad{Host: host, Base: base, Quote: quote}
	if err := send(1, "loadmarket", load); err != nil {
		return nil, nil, fmt.Errorf("dcrdex ws: loadmarket: %w", err)
	}
	needCandles := dur != ""
	if needCandles {
		load.Dur = dur
		if err := send(2, "loadcandles", load); err != nil {
			return nil, nil, fmt.Errorf("dcrdex ws: loadcandles: %w", err)
		}
	}
	// Stop the temporary feed once the snapshots are read.
	defer func() { _ = send(3, "unmarket", dexMarketLoad{}) }()

	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	for book == nil || (needCandles && candles == nil) {
		_, data, rerr := conn.ReadMessage()
		if rerr != nil {
			if book == nil {
				return nil, nil, fmt.Errorf("dcrdex ws: read: %w", rerr)
			}
			break // got the book; tolerate a missing candle set
		}
		var msg dexWSMessage
		if json.Unmarshal(data, &msg) != nil || msg.Type != dexMsgNotification {
			continue
		}
		var bu dexBookUpdate
		if json.Unmarshal(msg.Payload, &bu) != nil {
			continue
		}
		switch msg.Route {
		case "book":
			book = bu.Payload
		case "candles":
			candles = bu.Payload
		}
	}
	return book, candles, nil
}

// DcrdexOrderBook returns the live order book snapshot for a market: the buy and
// sell depth, current-epoch orders, and recent matches. Public data, so it works
// whether or not the DEX session is unlocked.
func DcrdexOrderBook(ctx context.Context, host string, base, quote uint32) (json.RawMessage, error) {
	book, _, err := dexMarketSnapshot(ctx, host, base, quote, "")
	return book, err
}

// DcrdexCandles returns the candlestick set for a market and bin duration (one
// of the market's candleDurs, e.g. "24h", "1h", "5m"). Public data.
func DcrdexCandles(ctx context.Context, host string, base, quote uint32, dur string) (json.RawMessage, error) {
	_, candles, err := dexMarketSnapshot(ctx, host, base, quote, dur)
	if err != nil {
		return nil, err
	}
	if candles == nil {
		return nil, fmt.Errorf("dcrdex: no candles returned for duration %q", dur)
	}
	return candles, nil
}
