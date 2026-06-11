// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"strconv"
)

func boolArg(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TradeParams are the parameters for placing an order. Qty and Rate are in
// atomic units (qtyAtomic / message-rate), as DCRDEX expects.
type TradeParams struct {
	AppPass string
	Host    string
	IsLimit bool
	Sell    bool
	Base    uint32
	Quote   uint32
	Qty     uint64
	Rate    uint64
	TifNow  bool
	Options map[string]any
}

// Trade places a single limit or market order. The raw result holds the order
// id, signature and timestamp.
func (c *Client) Trade(ctx context.Context, p TradeParams) (json.RawMessage, error) {
	opts := "{}"
	if len(p.Options) > 0 {
		if b, err := json.Marshal(p.Options); err == nil {
			opts = string(b)
		}
	}
	args := []string{
		p.Host,
		boolArg(p.IsLimit),
		boolArg(p.Sell),
		strconv.FormatUint(uint64(p.Base), 10),
		strconv.FormatUint(uint64(p.Quote), 10),
		strconv.FormatUint(p.Qty, 10),
		strconv.FormatUint(p.Rate, 10),
		boolArg(p.TifNow),
		opts,
	}
	var res json.RawMessage
	err := c.Call(ctx, "trade", []string{p.AppPass}, args, &res)
	return res, err
}

// Cancel cancels an order by its hex-encoded order ID.
func (c *Client) Cancel(ctx context.Context, orderID string) error {
	return c.Call(ctx, "cancel", nil, []string{orderID}, nil)
}

// MyOrders returns the user's active and recent orders (raw). host is optional;
// pass "" for all hosts.
func (c *Client) MyOrders(ctx context.Context, host string) (json.RawMessage, error) {
	var args []string
	if host != "" {
		args = []string{host}
	}
	var res json.RawMessage
	err := c.Call(ctx, "myorders", nil, args, &res)
	return res, err
}
