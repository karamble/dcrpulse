// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "encoding/json"

// GamingPresenceChanged carries an invalidation, not a potentially reordered
// ready flag. Browsers reread the current registry, including any other stream
// still connected for this game. No credential or wallet data leaves here.
func GamingPresenceChanged(game string) {
	payload, _ := json.Marshal(struct {
		Game string `json:"game"`
	}{game})
	PublishBisonrelayEvent("gaming-presence", payload)
}
