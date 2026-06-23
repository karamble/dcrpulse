// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Mainnet consensus constants used to derive countdowns/participation. Read from
// chain params would be ideal; these are fixed for mainnet.
const (
	atomsPerCoin      = 1e8
	ticketPriceWindow = 144   // blocks between stake-difficulty (ticket price) changes
	subsidyInterval   = 6144  // blocks between block-subsidy reductions
	ticketsPerBlock   = 5     // votes (and max fresh tickets) per block
	targetPoolSize    = 40960 // steady-state live ticket pool target (8192 * 5)
)

// ---- widget payload shapes (must match the umbrelOS template schemas) ----

type statItem struct {
	Title   string `json:"title,omitempty"`
	Text    string `json:"text,omitempty"`
	Subtext string `json:"subtext,omitempty"`
	Icon    string `json:"icon,omitempty"`
}

// statsWidget covers both four-stats and three-stats.
type statsWidget struct {
	Type  string     `json:"type"`
	Link  string     `json:"link,omitempty"`
	Items []statItem `json:"items"`
}

type textWithProgress struct {
	Type          string  `json:"type"`
	Link          string  `json:"link,omitempty"`
	Title         string  `json:"title,omitempty"`
	Text          string  `json:"text,omitempty"`
	Subtext       string  `json:"subtext,omitempty"`
	ProgressLabel string  `json:"progressLabel,omitempty"`
	Progress      float64 `json:"progress"`
}

type gaugeItem struct {
	Title    string  `json:"title,omitempty"`
	Text     string  `json:"text,omitempty"`
	Subtext  string  `json:"subtext,omitempty"`
	Progress float64 `json:"progress"`
}

type twoStatsGauge struct {
	Type  string      `json:"type"`
	Link  string      `json:"link,omitempty"`
	Items []gaugeItem `json:"items"`
}

type listItem struct {
	Text    string `json:"text,omitempty"`
	Subtext string `json:"subtext,omitempty"`
}

type listWidget struct {
	Type        string     `json:"type"`
	Link        string     `json:"link,omitempty"`
	NoItemsText string     `json:"noItemsText,omitempty"`
	Items       []listItem `json:"items"`
}

type emojiItem struct {
	Emoji string `json:"emoji,omitempty"`
	Text  string `json:"text,omitempty"`
}

type listEmojiWidget struct {
	Type  string      `json:"type"`
	Link  string      `json:"link,omitempty"`
	Items []emojiItem `json:"items"`
}

type buttonItem struct {
	Text string `json:"text,omitempty"`
	Icon string `json:"icon,omitempty"`
	Link string `json:"link"`
}

type textWithButtons struct {
	Type    string       `json:"type"`
	Title   string       `json:"title,omitempty"`
	Text    string       `json:"text,omitempty"`
	Subtext string       `json:"subtext,omitempty"`
	Buttons []buttonItem `json:"buttons"`
}

// ---- typed dcrd RPC helpers ----

type chainInfo struct {
	Blocks               int64   `json:"blocks"`
	Headers              int64   `json:"headers"`
	BestBlockHash        string  `json:"bestblockhash"`
	VerificationProgress float64 `json:"verificationprogress"`
	InitialBlockDownload bool    `json:"initialblockdownload"`
	Deployments          map[string]struct {
		Status string `json:"status"`
	} `json:"deployments"`
}

func (s *server) chainInfo() (*chainInfo, error) {
	raw, err := s.dcrd.call("getblockchaininfo")
	if err != nil {
		return nil, err
	}
	var ci chainInfo
	return &ci, json.Unmarshal(raw, &ci)
}

type blockHeader struct {
	Height   int64   `json:"height"`
	PoolSize int64   `json:"poolsize"`
	SBits    float64 `json:"sbits"`
}

func (s *server) bestHeader(hash string) (*blockHeader, error) {
	raw, err := s.dcrd.call("getblockheader", hash, true)
	if err != nil {
		return nil, err
	}
	var h blockHeader
	return &h, json.Unmarshal(raw, &h)
}

type stakeDiff struct {
	Current float64 `json:"current"`
}

func (s *server) stakeDifficulty() (stakeDiff, error) {
	var sd stakeDiff
	raw, err := s.dcrd.call("getstakedifficulty")
	if err != nil {
		return sd, err
	}
	return sd, json.Unmarshal(raw, &sd)
}

// stakeDiffEstimate is dcrd's estimatestakediff result. Expected is the live
// estimate for the next window's ticket price; getstakedifficulty's "next" only
// recomputes at window boundaries, so mid-window it just mirrors the current price.
type stakeDiffEstimate struct {
	Expected float64 `json:"expected"`
}

func (s *server) estimateStakeDiff() (stakeDiffEstimate, error) {
	var e stakeDiffEstimate
	raw, err := s.dcrd.call("estimatestakediff")
	if err != nil {
		return e, err
	}
	return e, json.Unmarshal(raw, &e)
}

type mempoolInfo struct {
	Size  int64 `json:"size"`
	Bytes int64 `json:"bytes"`
}

func (s *server) mempoolInfo() (mempoolInfo, error) {
	var mi mempoolInfo
	raw, err := s.dcrd.call("getmempoolinfo")
	if err != nil {
		return mi, err
	}
	return mi, json.Unmarshal(raw, &mi)
}

type miningInfo struct {
	NetworkHashPS float64 `json:"networkhashps"`
	Difficulty    float64 `json:"difficulty"`
}

func (s *server) miningInfo() (miningInfo, error) {
	var mi miningInfo
	raw, err := s.dcrd.call("getmininginfo")
	if err != nil {
		return mi, err
	}
	return mi, json.Unmarshal(raw, &mi)
}

type subsidy struct {
	Pow       int64 `json:"pow"`
	Pos       int64 `json:"pos"`
	Developer int64 `json:"developer"`
	Total     int64 `json:"total"`
}

func (s *server) blockSubsidy(height, voters int64) (subsidy, error) {
	var sub subsidy
	raw, err := s.dcrd.call("getblocksubsidy", height, voters)
	if err != nil {
		return sub, err
	}
	return sub, json.Unmarshal(raw, &sub)
}

func (s *server) scalarInt(method string, params ...interface{}) (int64, error) {
	raw, err := s.dcrd.call(method, params...)
	if err != nil {
		return 0, err
	}
	var n int64
	return n, json.Unmarshal(raw, &n)
}

func (s *server) scalarFloat(method string, params ...interface{}) (float64, error) {
	raw, err := s.dcrd.call(method, params...)
	if err != nil {
		return 0, err
	}
	var f float64
	return f, json.Unmarshal(raw, &f)
}

func (s *server) treasuryBalance() (int64, error) {
	raw, err := s.dcrd.call("gettreasurybalance")
	if err != nil {
		return 0, err
	}
	var t struct {
		Balance int64 `json:"balance"`
	}
	return t.Balance, json.Unmarshal(raw, &t)
}

func (s *server) mempoolTicketCount() (int64, error) {
	raw, err := s.dcrd.call("getrawmempool", false, "tickets")
	if err != nil {
		return 0, err
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

// ---- widget builders ----

func (s *server) widgetSync() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	progress, label := 1.0, "Synced"
	if ci.InitialBlockDownload || ci.VerificationProgress < 0.9999 {
		label = "Syncing"
		if ci.Headers > 0 {
			progress = float64(ci.Blocks) / float64(ci.Headers)
		} else {
			progress = ci.VerificationProgress
		}
	}
	return textWithProgress{
		Type:          "text-with-progress",
		Link:          "/explorer",
		Title:         "Blockchain",
		Text:          commafy(ci.Blocks),
		ProgressLabel: label,
		Progress:      clamp01(progress),
	}, nil
}

func (s *server) widgetNodeStats() (any, error) {
	peers, err := s.scalarInt("getconnectioncount")
	if err != nil {
		return nil, err
	}
	mp, err := s.mempoolInfo()
	if err != nil {
		return nil, err
	}
	mi, err := s.miningInfo()
	if err != nil {
		return nil, err
	}
	return statsWidget{
		Type: "four-stats",
		Items: []statItem{
			{Title: "Peers", Text: commafy(peers)},
			{Title: "Mempool", Text: commafy(mp.Size), Subtext: "txs"},
			{Title: "Hashrate", Text: fmtHashrate(mi.NetworkHashPS)},
			{Title: "Difficulty", Text: fmtCompact(mi.Difficulty)},
		},
	}, nil
}

func (s *server) widgetTicketPrice() (any, error) {
	sd, err := s.stakeDifficulty()
	if err != nil {
		return nil, err
	}
	est, err := s.estimateStakeDiff()
	if err != nil {
		return nil, err
	}
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	pos := ci.Blocks % ticketPriceWindow
	return textWithProgress{
		Type:          "text-with-progress",
		Link:          "/wallet/staking",
		Title:         "Ticket price",
		Text:          fmtDCR(sd.Current) + " DCR",
		Subtext:       "Next ~" + fmtDCR(est.Expected) + " DCR",
		ProgressLabel: commafy(ticketPriceWindow-pos) + " blocks to next",
		Progress:      float64(pos) / float64(ticketPriceWindow),
	}, nil
}

func (s *server) widgetTicketPool() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	hdr, err := s.bestHeader(ci.BestBlockHash)
	if err != nil {
		return nil, err
	}
	val, err := s.scalarFloat("getticketpoolvalue")
	if err != nil {
		return nil, err
	}
	mtix, err := s.mempoolTicketCount()
	if err != nil {
		return nil, err
	}
	return statsWidget{
		Type: "four-stats",
		Link: "/wallet/staking",
		Items: []statItem{
			{Title: "Pool size", Text: commafy(hdr.PoolSize), Subtext: "tickets"},
			{Title: "Pool value", Text: fmtCompact(val), Subtext: "DCR"},
			{Title: "Participation", Text: fmtPct(float64(hdr.PoolSize) / float64(targetPoolSize) * 100)},
			{Title: "In mempool", Text: commafy(mtix), Subtext: "tickets"},
		},
	}, nil
}

func (s *server) widgetStaking() (any, error) {
	sd, err := s.stakeDifficulty()
	if err != nil {
		return nil, err
	}
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	hdr, err := s.bestHeader(ci.BestBlockHash)
	if err != nil {
		return nil, err
	}
	return statsWidget{
		Type: "three-stats",
		Link: "/wallet/staking",
		Items: []statItem{
			{Icon: "ticket", Text: fmtDCR(sd.Current), Subtext: "DCR price"},
			{Icon: "stack-2", Text: commafy(hdr.PoolSize), Subtext: "pool"},
			{Icon: "chart-pie", Text: fmtPct(float64(hdr.PoolSize) / float64(targetPoolSize) * 100), Subtext: "participation"},
		},
	}, nil
}

func (s *server) widgetPriceGauges() (any, error) {
	sd, err := s.stakeDifficulty()
	if err != nil {
		return nil, err
	}
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	hdr, err := s.bestHeader(ci.BestBlockHash)
	if err != nil {
		return nil, err
	}
	pos := ci.Blocks % ticketPriceWindow
	part := float64(hdr.PoolSize) / float64(targetPoolSize)
	return twoStatsGauge{
		Type: "two-stats-with-guage",
		Link: "/wallet/staking",
		Items: []gaugeItem{
			{Title: "Ticket price", Text: fmtDCR(sd.Current) + " DCR", Progress: float64(pos) / float64(ticketPriceWindow)},
			{Title: "Participation", Text: fmtPct(part * 100), Progress: clamp01(part)},
		},
	}, nil
}

func (s *server) widgetSupply() (any, error) {
	supply, err := s.scalarInt("getcoinsupply")
	if err != nil {
		return nil, err
	}
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	sub, err := s.blockSubsidy(ci.Blocks+1, ticketsPerBlock)
	if err != nil {
		return nil, err
	}
	return statsWidget{
		Type: "four-stats",
		Items: []statItem{
			{Title: "Supply", Text: fmtCompact(atomsToDCR(supply)), Subtext: "DCR"},
			{Title: "Block reward", Text: fmtDCR(atomsToDCR(sub.Total)), Subtext: "DCR"},
			{Title: "Stake reward", Text: fmtDCR(atomsToDCR(sub.Pos)), Subtext: "DCR"},
			{Title: "Next reduction", Text: commafy(subsidyInterval - ci.Blocks%subsidyInterval), Subtext: "blocks"},
		},
	}, nil
}

func (s *server) widgetSubsidyCountdown() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	pos := ci.Blocks % subsidyInterval
	return textWithProgress{
		Type:          "text-with-progress",
		Title:         "Next subsidy reduction",
		Text:          commafy(subsidyInterval-pos) + " blocks",
		ProgressLabel: fmtPct(float64(pos) / float64(subsidyInterval) * 100),
		Progress:      float64(pos) / float64(subsidyInterval),
	}, nil
}

func (s *server) widgetNetwork() (any, error) {
	supply, err := s.scalarInt("getcoinsupply")
	if err != nil {
		return nil, err
	}
	treasury, err := s.treasuryBalance()
	if err != nil {
		return nil, err
	}
	mi, err := s.miningInfo()
	if err != nil {
		return nil, err
	}
	return statsWidget{
		Type: "four-stats",
		Items: []statItem{
			{Title: "Supply", Text: fmtCompact(atomsToDCR(supply)), Subtext: "DCR"},
			{Title: "Treasury", Text: fmtCompact(atomsToDCR(treasury)), Subtext: "DCR"},
			{Title: "Hashrate", Text: fmtHashrate(mi.NetworkHashPS)},
			{Title: "Difficulty", Text: fmtCompact(mi.Difficulty)},
		},
	}, nil
}

func (s *server) widgetVotes() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	items := []listItem{}
	for id, d := range ci.Deployments {
		if d.Status == "started" || d.Status == "lockedin" {
			items = append(items, listItem{Text: id, Subtext: titleCase(d.Status)})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Text < items[j].Text })
	return listWidget{
		Type:        "list",
		Link:        "/wallet/governance",
		NoItemsText: "No active consensus votes",
		Items:       items,
	}, nil
}

func (s *server) widgetStatus() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	hdr, err := s.bestHeader(ci.BestBlockHash)
	if err != nil {
		return nil, err
	}
	sd, err := s.stakeDifficulty()
	if err != nil {
		return nil, err
	}
	treasury, err := s.treasuryBalance()
	if err != nil {
		return nil, err
	}
	return listEmojiWidget{
		Type: "list-emoji",
		Items: []emojiItem{
			{Emoji: "\U0001F9F1", Text: "Height " + commafy(ci.Blocks)},
			{Emoji: "\U0001F3AB", Text: "Pool " + commafy(hdr.PoolSize)},
			{Emoji: "\U0001F4B0", Text: fmtDCR(sd.Current) + " DCR ticket"},
			{Emoji: "\U0001F3E6", Text: fmtCompact(atomsToDCR(treasury)) + " DCR treasury"},
		},
	}, nil
}

func (s *server) widgetLaunch() (any, error) {
	ci, err := s.chainInfo()
	if err != nil {
		return nil, err
	}
	status := "Synced"
	if ci.InitialBlockDownload || ci.VerificationProgress < 0.9999 {
		status = "Syncing"
	}
	return textWithButtons{
		Type:    "text-with-buttons",
		Title:   "Decred Pulse",
		Text:    "Block " + commafy(ci.Blocks),
		Subtext: status,
		Buttons: []buttonItem{
			{Text: "Explorer", Link: "/explorer"},
			{Text: "Staking", Link: "/wallet/staking"},
			{Text: "DEX", Link: "/dex"},
		},
	}, nil
}

// ---- formatting ----

func atomsToDCR(atoms int64) float64 { return float64(atoms) / atomsPerCoin }

func fmtDCR(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func fmtCompact(v float64) string {
	switch {
	case v >= 1e9:
		return strconv.FormatFloat(v/1e9, 'f', 2, 64) + "B"
	case v >= 1e6:
		return strconv.FormatFloat(v/1e6, 'f', 2, 64) + "M"
	case v >= 1e3:
		return strconv.FormatFloat(v/1e3, 'f', 2, 64) + "k"
	default:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}

func fmtPct(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) + "%" }

func fmtHashrate(hps float64) string {
	units := []string{"H/s", "kH/s", "MH/s", "GH/s", "TH/s", "PH/s", "EH/s"}
	i := 0
	for hps >= 1000 && i < len(units)-1 {
		hps /= 1000
		i++
	}
	return strconv.FormatFloat(hps, 'f', 2, 64) + " " + units[i]
}

func commafy(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
