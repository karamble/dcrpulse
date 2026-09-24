// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package oggopus writes Opus packets into an Ogg container exactly as Bison
// Relay's bruig client does for audio notes, so a note produced here is byte
// for byte what bruig would have produced. The writer files are vendored from
// bisonrelay/internal/audio; see the header of each for the upstream commit.
package oggopus
