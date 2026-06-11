// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "encoding/json"

// ThemeStore is the cross-wallet theme document persisted in the global
// config. Shipped themes live in the frontend bundle; only the active
// selection and any user-created themes are persisted. Custom themes are
// kept as opaque JSON because the frontend owns the theme schema and
// validates it; the backend only round-trips and bounds them.
type ThemeStore struct {
	Schema        int               `json:"schema"`
	ActiveThemeID string            `json:"activeThemeId"`
	CustomThemes  []json.RawMessage `json:"customThemes"`
}
