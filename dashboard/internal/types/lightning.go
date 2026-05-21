// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// LightningStatus is the high-level state the dashboard frontend uses
// to decide whether to render the setup wizard, the unlock screen, or
// the Overview tab. Mirrors the staged ConnectPage flow Decrediton
// renders in app/components/views/LNPage/ConnectPage/.
type LightningStatus struct {
	// Stage is one of:
	//   "unavailable"  dcrlnd container unreachable
	//   "needs-setup"  no dedicated lightning account yet (sentinel
	//                  file absent); show the disclaimer + wizard
	//   "needs-unlock" sentinel exists, dcrlnd is up, wallet locked
	//   "syncing"      unlocked, currently catching up to chain/graph
	//   "ready"        unlocked and fully synced
	Stage           string `json:"stage"`
	IdentityPubkey  string `json:"identityPubkey,omitempty"`
	Alias           string `json:"alias,omitempty"`
	BlockHeight     uint32 `json:"blockHeight,omitempty"`
	SyncedToChain   bool   `json:"syncedToChain,omitempty"`
	SyncedToGraph   bool   `json:"syncedToGraph,omitempty"`
	NumActiveChans  uint32 `json:"numActiveChans,omitempty"`
	NumPendingChans uint32 `json:"numPendingChans,omitempty"`
}

// LightningInfo is the verbatim GetInfo response trimmed to the fields
// the UI displays.
type LightningInfo struct {
	IdentityPubkey      string `json:"identityPubkey"`
	Alias               string `json:"alias"`
	Version             string `json:"version"`
	BlockHeight         uint32 `json:"blockHeight"`
	BlockHash           string `json:"blockHash"`
	SyncedToChain       bool   `json:"syncedToChain"`
	SyncedToGraph       bool   `json:"syncedToGraph"`
	NumActiveChannels   uint32 `json:"numActiveChannels"`
	NumInactiveChannels uint32 `json:"numInactiveChannels"`
	NumPendingChannels  uint32 `json:"numPendingChannels"`
	NumPeers            uint32 `json:"numPeers"`
	BestHeaderTimestamp int64  `json:"bestHeaderTimestamp"`
	Chains              []string `json:"chains"`
}

// LightningBalance is WalletBalance + ChannelBalance merged into the
// shape Decrediton's OverviewTab displays in the 6-card grid.
type LightningBalance struct {
	OnChainConfirmed   int64 `json:"onChainConfirmed"`
	OnChainUnconfirmed int64 `json:"onChainUnconfirmed"`
	OnChainTotal       int64 `json:"onChainTotal"`
	ChannelLocal       int64 `json:"channelLocal"`
	ChannelRemote      int64 `json:"channelRemote"`
	ChannelPending     int64 `json:"channelPending"`
}

// LightningActivityEntry is one row in the recent-activity feed.
type LightningActivityEntry struct {
	Kind      string `json:"kind"` // "invoice" | "payment" | "channel"
	Timestamp int64  `json:"timestamp"`
	Amount    int64  `json:"amount"`
	State     string `json:"state"`
	Memo      string `json:"memo,omitempty"`
}

type LightningActivity struct {
	Entries []LightningActivityEntry `json:"entries"`
}

// LightningSetupRequest is the wizard's submission payload.
type LightningSetupRequest struct {
	Passphrase string `json:"passphrase"`
}

// LightningUnlockRequest unlocks dcrlnd's wallet after the first init.
type LightningUnlockRequest struct {
	Passphrase string `json:"passphrase"`
}

// PeerPreset is one entry in the open-channel form's autocomplete
// datalist. URI is the LN node identifier (`pubkey@host:port`). Label
// is a human-readable handle (typically the brserver hostname).
// IsFallback marks the hardcoded hub0 entry served when brseeder is
// unreachable.
type PeerPreset struct {
	Label      string `json:"label"`
	URI        string `json:"uri"`
	IsFallback bool   `json:"isFallback,omitempty"`
}

// LightningChannelStatus discriminates open/pending/closed channels.
// Mirrors Decrediton's CHANNEL_STATUS_* + pendingStatus props.
const (
	ChannelStatusOpen              = "open"
	ChannelStatusPendingOpen       = "pending-open"
	ChannelStatusPendingCloseCoop  = "pending-close-coop"
	ChannelStatusPendingCloseForce = "pending-close-force"
	ChannelStatusPendingWaitClose  = "pending-wait-close"
	ChannelStatusClosed            = "closed"
)

// LightningChannel is one row in the channels list. Fields that don't
// apply to a particular status (e.g. CSV delay on a closed channel)
// are zero-valued; the frontend renders status-dependent subsets.
type LightningChannel struct {
	Status         string `json:"status"`
	ChannelPoint   string `json:"channelPoint"`
	ChannelID      uint64 `json:"channelId,omitempty"`
	RemotePubkey   string `json:"remotePubkey"`
	RemoteAlias    string `json:"remoteAlias,omitempty"`
	Capacity       int64  `json:"capacity"`
	LocalBalance   int64  `json:"localBalance"`
	RemoteBalance  int64  `json:"remoteBalance"`
	CommitFee      int64  `json:"commitFee,omitempty"`
	UnsettledBal   int64  `json:"unsettledBalance,omitempty"`
	TotalSentAtoms int64  `json:"totalSent,omitempty"`
	TotalRecvAtoms int64  `json:"totalReceived,omitempty"`
	NumUpdates     uint64 `json:"numUpdates,omitempty"`
	CSVDelay       uint32 `json:"csvDelay,omitempty"`
	Active         bool   `json:"active,omitempty"`
	Private        bool   `json:"private,omitempty"`
	Initiator      bool   `json:"initiator,omitempty"`
	CloseType      string `json:"closeType,omitempty"`    // closed channels only
	ClosingTxHash  string `json:"closingTxHash,omitempty"` // closed channels only
	SettledBalance int64  `json:"settledBalance,omitempty"`
	TimeLockedBal  int64  `json:"timeLockedBalance,omitempty"`
	LimboBalance   int64  `json:"limboBalance,omitempty"`

	// Funding-tx confirmation progress, populated for pending-open
	// channels by looking the funding tx up in dcrwallet's local index.
	// CurrentConfs is the number of confirmations the funding tx has
	// right now; RequiredConfs is the threshold dcrlnd will treat as
	// "channel open" for this specific channel (3-6, adaptive on
	// channel size per server.go:1192-1232). Zero means unknown
	// (channel not in pending-open state, or funding tx not yet
	// observed by the wallet).
	CurrentConfs  int32 `json:"currentConfs,omitempty"`
	RequiredConfs int32 `json:"requiredConfs,omitempty"`
}

type LightningChannels struct {
	Channels []LightningChannel `json:"channels"`
}

type OpenChannelRequest struct {
	PeerURI      string `json:"peerUri"`      // `pubkey@host:port` or bare pubkey
	LocalAtoms   int64  `json:"localAtoms"`   // local funding amount
	PushAtoms    int64  `json:"pushAtoms,omitempty"`
	Private      bool   `json:"private,omitempty"`
}

type OpenChannelResponse struct {
	FundingTxid string `json:"fundingTxid"`
	OutputIndex uint32 `json:"outputIndex"`
}

type CloseChannelRequest struct {
	ChannelPoint string `json:"channelPoint"`
	Force        bool   `json:"force"`
}

type CloseChannelResponse struct {
	ClosingTxid string `json:"closingTxid"`
}

type NodeMatch struct {
	Pubkey string `json:"pubkey"`
	Alias  string `json:"alias,omitempty"`
	Color  string `json:"color,omitempty"`
}

type NodeSearchResponse struct {
	Matches []NodeMatch `json:"matches"`
}

type AutopilotStatus struct {
	Active bool `json:"active"`
}

// ChannelEvent is a tagged-union event pushed over the WebSocket.
// Type values match dcrlnd's ChannelEventUpdate.UpdateType (lowercased).
type ChannelEvent struct {
	Type         string `json:"type"`
	ChannelPoint string `json:"channelPoint,omitempty"`
	RemotePubkey string `json:"remotePubkey,omitempty"`
}

// LightningNetworkInfo is the global LN graph aggregate surface from
// dcrlnd's GetNetworkInfo RPC. All "size" fields are in atoms.
type LightningNetworkInfo struct {
	NumNodes             uint32  `json:"numNodes"`
	NumChannels          uint32  `json:"numChannels"`
	TotalNetworkCapacity int64   `json:"totalNetworkCapacity"`
	AvgChannelSize       float64 `json:"avgChannelSize"`
	MedianChannelSize    int64   `json:"medianChannelSize"`
	MinChannelSize       int64   `json:"minChannelSize"`
	MaxChannelSize       int64   `json:"maxChannelSize"`
	GraphDiameter        uint32  `json:"graphDiameter"`
	AvgOutDegree         float64 `json:"avgOutDegree"`
}

// TopLightningNode is one row in the Top-N nodes table.
type TopLightningNode struct {
	Pubkey        string `json:"pubkey"`
	Alias         string `json:"alias,omitempty"`
	Color         string `json:"color,omitempty"`
	NumChannels   uint32 `json:"numChannels"`
	CapacityAtoms int64  `json:"capacityAtoms"`
}

// LightningNetworkPanel is the merged response for the Overview's
// global-network-stats section.
type LightningNetworkPanel struct {
	Info     LightningNetworkInfo `json:"info"`
	TopNodes []TopLightningNode   `json:"topNodes"`
}
