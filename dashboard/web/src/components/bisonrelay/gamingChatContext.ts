// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext } from 'react';

// GamingChatCtx carries the open group chat down to a game invite chip.
//
// A table plays in the conversation its invitation arrived in, so accepting one
// has to say which that is - and the chip renders several components below
// anything that knows. This follows the image viewer's context rather than
// threading a prop through every message renderer in between.
//
// It lives in its own module so the chip does not have to import the page that
// renders it, which would be a cycle.
export const GamingChatCtx = createContext<string | null>(null);
