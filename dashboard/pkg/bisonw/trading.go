// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
)

func boolArg(b bool) string {
	if b {
		return "true"
	}
	return "false"
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
