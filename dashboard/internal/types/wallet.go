// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "time"

// WalletDashboardData represents all wallet dashboard metrics
type WalletDashboardData struct {
	WalletStatus WalletStatus       `json:"walletStatus"`
	AccountInfo  AccountInfo        `json:"accountInfo"`
	Accounts     []AccountInfo      `json:"accounts"`
	StakingInfo  *WalletStakingInfo `json:"stakingInfo,omitempty"`
	LastUpdate   time.Time          `json:"lastUpdate"`
}

type WalletStatus struct {
	Status           string  `json:"status"` // "locked", "unlocked", "syncing", "synced", "no_wallet", "disconnected"
	SyncProgress     float64 `json:"syncProgress"`
	SyncHeight       int64   `json:"syncHeight"`
	BestBlockHash    string  `json:"bestBlockHash"`
	Version          string  `json:"version"`
	Unlocked         bool    `json:"unlocked"`
	DaemonConnected  bool    `json:"daemonConnected"`
	RescanInProgress bool    `json:"rescanInProgress"`
	SyncMessage      string  `json:"syncMessage"`
	IsWatchOnly      bool    `json:"isWatchOnly"` // dcrwallet reports the wallet is watching-only (no spending keys)
}

type AccountInfo struct {
	AccountName             string  `json:"accountName"`
	TotalBalance            float64 `json:"totalBalance"`
	SpendableBalance        float64 `json:"spendableBalance"`
	ImmatureBalance         float64 `json:"immatureBalance"`
	UnconfirmedBalance      float64 `json:"unconfirmedBalance"`
	LockedByTickets         float64 `json:"lockedByTickets"`
	VotingAuthority         float64 `json:"votingAuthority"`
	ImmatureCoinbaseRewards float64 `json:"immatureCoinbaseRewards"`
	ImmatureStakeGeneration float64 `json:"immatureStakeGeneration"`
	AccountNumber           uint32  `json:"accountNumber"`
	AccountEncrypted        bool    `json:"accountEncrypted"`
	AccountUnlocked         bool    `json:"accountUnlocked"`
	// Reserved marks accounts other daemons bind to by name (mixed/unmixed/
	// lightning/dex) or the imported bucket; the UI hides their rename action.
	Reserved bool `json:"reserved"`
	// Bip44Index is the real BIP44 account index for an imported xpub account
	// (AccountNumber >= 2^31), recorded at import. Nil/omitted for normal accounts,
	// whose AccountNumber already is the BIP44 index. Used to label the account and
	// to drive offline-signing derivation.
	Bip44Index *uint32 `json:"bip44Index,omitempty"`
	// Wallet-wide totals (only populated in primary AccountInfo)
	CumulativeTotal      float64 `json:"cumulativeTotal,omitempty"`
	TotalSpendable       float64 `json:"totalSpendable,omitempty"`
	TotalLockedByTickets float64 `json:"totalLockedByTickets,omitempty"`
}

type Transaction struct {
	TxID                 string    `json:"txid"`
	Amount               float64   `json:"amount"`
	Fee                  float64   `json:"fee,omitempty"`
	Confirmations        int64     `json:"confirmations"`
	BlockHash            string    `json:"blockHash,omitempty"`
	BlockTime            int64     `json:"blockTime,omitempty"`
	Time                 time.Time `json:"time"`
	Category             string    `json:"category"` // "send", "receive", "immature", "generate"
	TxType               string    `json:"txType"`   // "regular", "ticket", "vote", "revocation"
	Address              string    `json:"address,omitempty"`
	Account              string    `json:"account,omitempty"`
	Vout                 uint32    `json:"vout"`
	Generated            bool      `json:"generated,omitempty"`
	IsMixed              bool      `json:"isMixed,omitempty"`              // CoinJoin/StakeShuffle transaction
	IsVSPFee             bool      `json:"isVSPFee,omitempty"`             // VSP fee payment
	RelatedTicket        string    `json:"relatedTicket,omitempty"`        // Ticket txid for VSP fees
	BlockHeight          int64     `json:"blockHeight,omitempty"`          // Confirmed block height
	IsTicketMature       bool      `json:"isTicketMature,omitempty"`       // Vote: passed 256-block maturity
	BlocksUntilSpendable int64     `json:"blocksUntilSpendable,omitempty"` // Vote: blocks until spendable
}

type TransactionListResponse struct {
	Transactions []Transaction `json:"transactions"`
	Total        int           `json:"total"`
}

type Address struct {
	Address string `json:"address"`
	Account string `json:"account"`
	Used    bool   `json:"used"`
	Path    string `json:"path"` // BIP44 path
}

type ImportXpubRequest struct {
	Xpub        string `json:"xpub"`
	AccountName string `json:"accountName"`
	// AccountIndex is the real BIP44 account index (0,1,2...) the xpub was derived
	// from on the signing device. dcrwallet assigns imported xpub accounts an
	// internal number >= 2^31, so this records the true index for offline signing
	// (the device derives m/44'/coin'/AccountIndex'/branch/index). Optional: nil for
	// a monitor-only xpub with no hardware wallet to spend from; a pointer so an
	// omitted value is distinguishable from a deliberate 0.
	AccountIndex *uint32 `json:"accountIndex,omitempty"`
	Rescan       bool    `json:"rescan"`
}

type ImportXpubResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	AccountNum uint32 `json:"accountNum,omitempty"`
}

type NextAddressResponse struct {
	Address       string `json:"address"`
	AccountNumber uint32 `json:"accountNumber"`
}

type ValidateAddressResponse struct {
	IsValid       bool   `json:"isValid"`
	IsMine        bool   `json:"isMine"`
	AccountNumber uint32 `json:"accountNumber"`
}

// TxRecipient is one recipient/amount pair of a transaction. It lets a single
// transaction pay several destinations at once.
type TxRecipient struct {
	Address     string `json:"address"`
	AmountAtoms int64  `json:"amountAtoms"`
}

// ConstructTransactionRequest builds an unsigned transaction. Outputs carries one
// or more recipients; the legacy single Address/AmountAtoms pair is still accepted
// and treated as a one-output Outputs slice. SendAll sweeps the whole spendable
// balance to a single Address (Outputs is ignored).
type ConstructTransactionRequest struct {
	SourceAccount uint32        `json:"sourceAccount"`
	Address       string        `json:"address"`
	AmountAtoms   int64         `json:"amountAtoms"`
	Outputs       []TxRecipient `json:"outputs,omitempty"`
	SendAll       bool          `json:"sendAll"`
}

type ConstructTransactionResponse struct {
	UnsignedTxHex       string `json:"unsignedTxHex"`
	InputsTotalAtoms    int64  `json:"inputsTotalAtoms"`
	OutputsTotalAtoms   int64  `json:"outputsTotalAtoms"`
	ChangeAtoms         int64  `json:"changeAtoms"`
	FeeAtoms            int64  `json:"feeAtoms"`
	TotalDebitedAtoms   int64  `json:"totalDebitedAtoms"`
	EstimatedSignedSize uint32 `json:"estimatedSignedSize"`
}

type SignPublishTransactionRequest struct {
	SourceAccount uint32 `json:"sourceAccount"`
	UnsignedTxHex string `json:"unsignedTxHex"`
	Passphrase    string `json:"passphrase"`
}

type SignPublishTransactionResponse struct {
	TxHash string `json:"txHash"`
}

// DecodeSignedTxRequest carries a signed transaction for preview. SignedTxB64 is
// the base64 of a hardware-wallet file's raw bytes (the Passport ".dcrtx" file is
// the raw serialized transaction); SignedTx is plain-text input (a bare hex string
// or an export with "=== ... ===" sections). When both are set, SignedTxB64 wins.
type DecodeSignedTxRequest struct {
	SignedTxB64 string `json:"signedTxB64,omitempty"`
	SignedTx    string `json:"signedTx,omitempty"`
}

type SignedTxPreviewOutput struct {
	Index       uint32 `json:"index"`
	Address     string `json:"address,omitempty"`
	AmountAtoms int64  `json:"amountAtoms"`
	ScriptClass string `json:"scriptClass"`
	IsMine      bool   `json:"isMine"`
}

// SignedTxPreview is the decoded summary shown before broadcasting. FeeKnown is
// false when an input lacks its committed input amount, in which case FeeAtoms is
// not meaningful. TxHex echoes the normalized hex so the broadcast call sends
// exactly what was previewed.
type SignedTxPreview struct {
	Txid              string                  `json:"txid"`
	SizeBytes         int                     `json:"sizeBytes"`
	InputsTotalAtoms  int64                   `json:"inputsTotalAtoms"`
	OutputsTotalAtoms int64                   `json:"outputsTotalAtoms"`
	FeeAtoms          int64                   `json:"feeAtoms"`
	FeeKnown          bool                    `json:"feeKnown"`
	Outputs           []SignedTxPreviewOutput `json:"outputs"`
	TxHex             string                  `json:"txHex"`
}

type BroadcastSignedTxRequest struct {
	SignedTxB64 string `json:"signedTxB64,omitempty"`
	SignedTx    string `json:"signedTx,omitempty"`
}

type BroadcastSignedTxResponse struct {
	TxHash           string `json:"txHash"`
	AlreadyBroadcast bool   `json:"alreadyBroadcast,omitempty"`
}

// SignRequestExport carries the base64 CBOR SignRequest for an air-gapped hardware
// wallet, plus the same amount/fee summary the Send preview shows.
type SignRequestExport struct {
	SignRequestB64      string `json:"signRequestB64"`
	SignRequestUR       string `json:"signRequestUR"`
	AccountFp           string `json:"accountFp,omitempty"`
	InputsTotalAtoms    int64  `json:"inputsTotalAtoms"`
	OutputsTotalAtoms   int64  `json:"outputsTotalAtoms"`
	ChangeAtoms         int64  `json:"changeAtoms"`
	FeeAtoms            int64  `json:"feeAtoms"`
	TotalDebitedAtoms   int64  `json:"totalDebitedAtoms"`
	EstimatedSignedSize uint32 `json:"estimatedSignedSize"`
}

// DeviceBalanceAccount is one account row of the device balance export: the
// fingerprint sent to the device plus display metadata for the panel.
type DeviceBalanceAccount struct {
	Name   string  `json:"name"`
	Number uint32  `json:"number"`
	Fp     string  `json:"fp"`
	Atoms  int64   `json:"atoms"`
	Dcr    float64 `json:"dcr"`
}

// DeviceBalanceExport carries the CBOR BalanceUpdate an air-gapped device
// imports for its display (a balance.dcr file on microSD, or a UR QR):
// per-account balances keyed by account fingerprint plus the DCR/USD rate.
// RateUsd is 0 when no rate was available (fiat omitted on the device).
type DeviceBalanceExport struct {
	BalanceB64 string                 `json:"balanceB64"`
	BalanceUR  string                 `json:"balanceUR"`
	Accounts   []DeviceBalanceAccount `json:"accounts"`
	RateUsd    float64                `json:"rateUsd"`
	AsOf       int64                  `json:"asOf"`
	FileName   string                 `json:"fileName"`
}

type RescanRequest struct {
	BeginHeight int32 `json:"beginHeight"`
}

type RescanResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type SyncProgressResponse struct {
	IsRescanning bool    `json:"isRescanning"`
	ScanHeight   int64   `json:"scanHeight"`
	ChainHeight  int64   `json:"chainHeight"`
	Progress     float64 `json:"progress"`
	Message      string  `json:"message"`
}

// WalletStakingInfo represents wallet staking information
type WalletStakingInfo struct {
	// From getstakeinfo
	BlockHeight    int64   `json:"blockHeight"`
	Difficulty     float64 `json:"difficulty"`
	TotalSubsidy   float64 `json:"totalSubsidy"`
	OwnMempoolTix  int32   `json:"ownMempoolTix"`
	Immature       int32   `json:"immature"`
	Unspent        int32   `json:"unspent"`
	Voted          int32   `json:"voted"`
	Revoked        int32   `json:"revoked"`
	UnspentExpired int32   `json:"unspentExpired"`
	PoolSize       int32   `json:"poolSize"`
	AllMempoolTix  int32   `json:"allMempoolTix"`
	// From estimatestakediff
	EstimatedMin      float64 `json:"estimatedMin"`
	EstimatedMax      float64 `json:"estimatedMax"`
	EstimatedExpected float64 `json:"estimatedExpected"`
	// From getstakedifficulty
	CurrentDifficulty float64 `json:"currentDifficulty"`
	NextDifficulty    float64 `json:"nextDifficulty"`
	// From dcrd getblocksubsidy at current height + 1, voters=5
	BlockSubsidyHeight          int64   `json:"blockSubsidyHeight"`
	BlockSubsidyTotal           float64 `json:"blockSubsidyTotal"`
	BlockSubsidyPoS             float64 `json:"blockSubsidyPos"`
	BlockSubsidyPoW             float64 `json:"blockSubsidyPow"`
	BlockSubsidyTreasury        float64 `json:"blockSubsidyTreasury"`
	BlocksUntilSubsidyReduction int64   `json:"blocksUntilSubsidyReduction"`
	SubsidyReductionInterval    int64   `json:"subsidyReductionInterval"`
}
