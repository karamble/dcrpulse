// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"strconv"
)

// Notifications returns up to num of the most recent notifications (raw
// []db.Notification: type, topic, subject, details, severity, stamp, acked, id).
func (c *Client) Notifications(ctx context.Context, num int) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "notifications", nil, []string{strconv.Itoa(num)}, &res)
	return res, err
}
