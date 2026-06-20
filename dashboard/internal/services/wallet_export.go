// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/rpc"
)

// ExportTypes enumerates the supported transaction-history/statistics CSV
// exports, mirroring Decrediton's Export tab (Transactions, Daily Balances,
// Balances, Vote Time, Tickets).
var ExportTypes = map[string]bool{
	"transactions":  true,
	"tickets":       true,
	"votetime":      true,
	"balances":      true,
	"dailybalances": true,
}

// ExportWalletCSV writes the requested CSV export to w. The byte format matches
// Decrediton's exportStatToCSV: comma separated, LF terminated (including the
// final line), string cells double-quoted, atom amounts rendered as DCR with 8
// decimals, timestamps in UTC ISO8601 (...Z), and nil cells left empty.
func ExportWalletCSV(ctx context.Context, w io.Writer, typ string) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC client not initialized")
	}
	cw := &csvWriter{w: w}
	var err error
	switch typ {
	case "transactions":
		err = exportTransactions(ctx, cw)
	case "tickets":
		err = exportTickets(ctx, cw)
	case "votetime":
		err = exportVoteTime(ctx, cw)
	case "balances":
		err = exportBalances(ctx, cw, false)
	case "dailybalances":
		err = exportBalances(ctx, cw, true)
	default:
		return fmt.Errorf("unknown export type %q", typ)
	}
	if err != nil {
		return err
	}
	return cw.err
}

// csvWriter accumulates write errors so callers can check once at the end.
type csvWriter struct {
	w   io.Writer
	err error
}

func (c *csvWriter) writeLine(cells ...string) {
	if c.err != nil {
		return
	}
	_, c.err = io.WriteString(c.w, strings.Join(cells, ",")+"\n")
}

func csvQuote(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

func csvAmount(atoms int64) string {
	return strconv.FormatFloat(dcrutil.Amount(atoms).ToCoin(), 'f', 8, 64)
}

func csvTimeUTC(unixSec int64) string {
	return time.Unix(unixSec, 0).UTC().Format("2006-01-02T15:04:05.000") + "Z"
}

// exportTx is a transaction normalized from a gRPC TransactionDetails, carrying
// the fields the exports need.
type exportTx struct {
	hash       string
	rawTx      []byte
	height     int32
	timestamp  int64
	txType     string
	rawType    pb.TransactionDetails_TransactionType
	fee        int64
	amount     int64
	creditsSum int64
	debitsSum  int64
	credits    []*pb.TransactionDetails_Output
	debits     []*pb.TransactionDetails_Input
	direction  string
}

func txTypeName(t pb.TransactionDetails_TransactionType) string {
	switch t {
	case pb.TransactionDetails_TICKET_PURCHASE:
		return "ticket"
	case pb.TransactionDetails_VOTE:
		return "vote"
	case pb.TransactionDetails_REVOCATION:
		return "revocation"
	case pb.TransactionDetails_COINBASE:
		return "coinbase"
	default:
		return "regular"
	}
}

func isStakeType(t pb.TransactionDetails_TransactionType) bool {
	switch t {
	case pb.TransactionDetails_TICKET_PURCHASE, pb.TransactionDetails_VOTE, pb.TransactionDetails_REVOCATION:
		return true
	}
	return false
}

func hashHex(b []byte) string {
	h, err := chainhash.NewHash(b)
	if err != nil {
		return ""
	}
	return h.String()
}

// newExportTx mirrors Decrediton's formatTransaction (wallet/service.js): amount
// is credits-debits, and direction is derived only for non-stake transactions.
func newExportTx(t *pb.TransactionDetails, height int32, blockTime int64) exportTx {
	var credits, debits int64
	for _, c := range t.GetCredits() {
		credits += c.GetAmount()
	}
	for _, d := range t.GetDebits() {
		debits += d.GetPreviousAmount()
	}
	amount := credits - debits
	fee := t.GetFee()
	rawType := t.GetTransactionType()
	ts := blockTime
	if ts == 0 {
		ts = t.GetTimestamp()
	}
	tx := exportTx{
		hash:       hashHex(t.GetHash()),
		rawTx:      t.GetTransaction(),
		height:     height,
		timestamp:  ts,
		txType:     txTypeName(rawType),
		rawType:    rawType,
		fee:        fee,
		amount:     amount,
		creditsSum: credits,
		debitsSum:  debits,
		credits:    t.GetCredits(),
		debits:     t.GetDebits(),
	}
	if !isStakeType(rawType) {
		switch {
		case amount > 0:
			tx.direction = "received"
		case amount < 0 && fee == -amount:
			tx.direction = "ticketfee"
		default:
			tx.direction = "sent"
		}
	}
	return tx
}

// fetchMinedExportTxs streams the wallet's full mined transaction history in
// ascending block order.
func fetchMinedExportTxs(ctx context.Context) ([]exportTx, error) {
	stream, err := rpc.WalletGrpcClient.GetTransactions(ctx, &pb.GetTransactionsRequest{
		StartingBlockHeight: 0,
		EndingBlockHeight:   -1,
	})
	if err != nil {
		return nil, err
	}
	var out []exportTx
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
		if mb := resp.GetMinedTransactions(); mb != nil {
			for _, t := range mb.GetTransactions() {
				out = append(out, newExportTx(t, mb.GetHeight(), mb.GetTimestamp()))
			}
		}
	}
	return out, nil
}

// exportTransactions writes the Transactions CSV:
// time,hash,type,direction,fee,amount,credits,debits.
func exportTransactions(ctx context.Context, cw *csvWriter) error {
	txs, err := fetchMinedExportTxs(ctx)
	if err != nil {
		return err
	}
	cw.writeLine(
		csvQuote("time"), csvQuote("hash"), csvQuote("type"), csvQuote("direction"),
		csvQuote("fee"), csvQuote("amount"), csvQuote("credits"), csvQuote("debits"),
	)
	for _, tx := range txs {
		dir := ""
		if tx.direction != "" {
			dir = csvQuote(tx.direction)
		}
		cw.writeLine(
			csvQuote(csvTimeUTC(tx.timestamp)),
			csvQuote(tx.hash),
			csvQuote(tx.txType),
			dir,
			csvAmount(tx.fee),
			csvAmount(tx.amount),
			csvAmount(tx.creditsSum),
			csvAmount(tx.debitsSum),
		)
	}
	return nil
}

// exportTicket is a normalized ticket for the Tickets/Vote Time exports,
// mirroring Decrediton's ticket normalizer (actions/TransactionActions.js).
type exportTicket struct {
	enterTimestamp     int64
	leaveTimestamp     int64
	hasSpender         bool
	status             string
	ticketHash         string
	spenderHash        string
	ticketInvestment   int64
	ticketReturnAmount int64
}

func ticketStatusLower(s pb.GetTicketsResponse_TicketDetails_TicketStatus) string {
	switch s {
	case pb.GetTicketsResponse_TicketDetails_UNMINED:
		return "unmined"
	case pb.GetTicketsResponse_TicketDetails_IMMATURE:
		return "immature"
	case pb.GetTicketsResponse_TicketDetails_LIVE:
		return "live"
	case pb.GetTicketsResponse_TicketDetails_VOTED:
		return "voted"
	case pb.GetTicketsResponse_TicketDetails_MISSED:
		return "missed"
	case pb.GetTicketsResponse_TicketDetails_EXPIRED:
		return "expired"
	case pb.GetTicketsResponse_TicketDetails_REVOKED:
		return "revoked"
	default:
		return "unknown"
	}
}

func fetchExportTickets(ctx context.Context) ([]exportTicket, error) {
	stream, err := rpc.WalletGrpcClient.GetTickets(ctx, &pb.GetTicketsRequest{})
	if err != nil {
		return nil, err
	}
	var out []exportTicket
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
		ticket := resp.GetTicket()
		if ticket == nil {
			continue
		}
		t := exportTicket{
			status:     ticketStatusLower(ticket.GetTicketStatus()),
			ticketHash: hashHex(ticket.GetTicket().GetHash()),
		}
		if b := resp.GetBlock(); b != nil && b.GetTimestamp() != 0 {
			t.enterTimestamp = b.GetTimestamp()
		} else {
			t.enterTimestamp = ticket.GetTicket().GetTimestamp()
		}
		// ticketInvestment = (sum of ticket debits) - ticket change, where change
		// outputs of an sstx are the even-numbered credits with index > 0.
		td := ticket.GetTicket()
		var change int64
		for _, c := range td.GetCredits() {
			if c.GetIndex() > 0 && c.GetIndex()%2 == 0 {
				change += c.GetAmount()
			}
		}
		var debitsSum int64
		for _, d := range td.GetDebits() {
			debitsSum += d.GetPreviousAmount()
		}
		t.ticketInvestment = debitsSum - change
		if sp := ticket.GetSpender(); sp != nil && len(sp.GetHash()) > 0 {
			t.hasSpender = true
			t.spenderHash = hashHex(sp.GetHash())
			t.leaveTimestamp = sp.GetTimestamp()
			for _, c := range sp.GetCredits() {
				t.ticketReturnAmount += c.GetAmount()
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// exportTickets writes the Tickets CSV:
// time,spenderTimestamp,status,ticketHash,spenderHash,sentAmount,returnedAmount.
func exportTickets(ctx context.Context, cw *csvWriter) error {
	tickets, err := fetchExportTickets(ctx)
	if err != nil {
		return err
	}
	cw.writeLine(
		csvQuote("time"), csvQuote("spenderTimestamp"), csvQuote("status"),
		csvQuote("ticketHash"), csvQuote("spenderHash"),
		csvQuote("sentAmount"), csvQuote("returnedAmount"),
	)
	for _, t := range tickets {
		spenderTs := ""
		spenderHash := ""
		returned := ""
		if t.hasSpender {
			spenderTs = csvQuote(csvTimeUTC(t.leaveTimestamp))
			spenderHash = csvQuote(t.spenderHash)
			returned = csvAmount(t.ticketReturnAmount)
		}
		cw.writeLine(
			csvQuote(csvTimeUTC(t.enterTimestamp)),
			spenderTs,
			csvQuote(t.status),
			csvQuote(t.ticketHash),
			spenderHash,
			csvAmount(t.ticketInvestment),
			returned,
		)
	}
	return nil
}

// exportVoteTime writes the Vote Time CSV: daysToVote,count (no time column).
func exportVoteTime(ctx context.Context, cw *csvWriter) error {
	cp, err := loadExportChainParams(ctx)
	if err != nil {
		return err
	}
	tickets, err := fetchExportTickets(ctx)
	if err != nil {
		return err
	}
	blocksPerDay := (60 * 60 * 24) / cp.targetTimePerBlock
	if blocksPerDay <= 0 {
		blocksPerDay = 1
	}
	expirationDays := int64(cp.ticketExpiry+cp.ticketMaturity)/blocksPerDay + 1
	buckets := make([]int64, expirationDays+1)
	for _, t := range tickets {
		if t.status != "voted" {
			continue
		}
		days := (t.leaveTimestamp - t.enterTimestamp) / (24 * 60 * 60)
		if days >= 0 && days < int64(len(buckets)) {
			buckets[days]++
		}
	}
	cw.writeLine(csvQuote("daysToVote"), csvQuote("count"))
	for days, count := range buckets {
		cw.writeLine(strconv.Itoa(days), strconv.FormatInt(count, 10))
	}
	return nil
}

// exportChainParams holds the chain parameters the balance/vote-time replay
// needs, resolved for the active network.
type exportChainParams struct {
	ticketMaturity     int32
	coinbaseMaturity   int32
	ticketExpiry       int32
	targetTimePerBlock int64
	genesisTimestamp   int64
}

func loadExportChainParams(ctx context.Context) (exportChainParams, error) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return exportChainParams{}, err
	}
	var p *chaincfg.Params
	switch network {
	case "mainnet":
		p = chaincfg.MainNetParams()
	case "testnet":
		p = chaincfg.TestNet3Params()
	case "simnet":
		p = chaincfg.SimNetParams()
	default:
		return exportChainParams{}, fmt.Errorf("unknown network %q", network)
	}
	return exportChainParams{
		ticketMaturity:     int32(p.TicketMaturity),
		coinbaseMaturity:   int32(p.CoinbaseMaturity),
		ticketExpiry:       int32(p.TicketExpiry),
		targetTimePerBlock: int64(p.TargetTimePerBlock / time.Second),
		genesisTimestamp:   p.GenesisBlock.Header.Timestamp.Unix(),
	}, nil
}

// ---- Balance replay engine (ported from Decrediton StatisticsActions.js,
// forwards path). Computes per-event balance snapshots over the full mined
// history, accounting for ticket commitments, vote/revoke stake results, and
// coinbase-maturity of vote/revoke proceeds. ----

type ticketInfoResult struct {
	isWallet     bool
	commitAmount int64
	spentAmount  int64
	purchaseFees int64
}

type voteRevokeResult struct {
	wasWallet          bool
	isVote             bool
	returnAmount       int64
	stakeResult        int64
	ticketCommitAmount int64
}

type maturingEntry struct {
	amount   int64
	isWallet bool
	isTicket bool
}

type balDelta struct {
	spendable         int64
	immature          int64
	immatureNonWallet int64
	locked            int64
	lockedNonWallet   int64
	voted             int64
	revoked           int64
	sent              int64
	received          int64
	ticket            int64
	stakeRewards      int64
	stakeFees         int64
	totalStake        int64
	timestamp         int64
}

type balState struct {
	spendable         int64
	immature          int64
	immatureNonWallet int64
	locked            int64
	lockedNonWallet   int64
	total             int64
	stakeRewards      int64
	stakeFees         int64
	totalStake        int64
	delta             balDelta
}

func ticketInfo(tx exportTx) ticketInfoResult {
	var change int64
	for _, c := range tx.credits {
		if c.GetIndex() > 0 && c.GetIndex()%2 == 0 {
			change += c.GetAmount()
		}
	}
	isWallet := len(tx.credits) > 0 && tx.credits[0].GetIndex() == 0
	var debitsSum int64
	for _, d := range tx.debits {
		debitsSum += d.GetPreviousAmount()
	}
	var poolFee int64
	if len(tx.debits) > 1 && tx.debits[0].GetIndex() == 0 {
		poolFee = tx.debits[0].GetPreviousAmount()
	}
	fee := int64(0)
	if isWallet {
		fee = tx.fee
	}
	commitAmount := debitsSum - change - fee - poolFee
	return ticketInfoResult{
		isWallet:     isWallet,
		commitAmount: commitAmount,
		spentAmount:  commitAmount + fee + poolFee,
		purchaseFees: debitsSum - commitAmount,
	}
}

func voteRevokeInfo(tx exportTx, liveTickets map[string]ticketInfoResult, txByHash map[string]exportTx) voteRevokeResult {
	// The spent ticket is the last input of the vote/revocation.
	ticketHash := ""
	if len(tx.rawTx) > 0 {
		var mtx wire.MsgTx
		if err := mtx.Deserialize(bytes.NewReader(tx.rawTx)); err == nil && len(mtx.TxIn) > 0 {
			ticketHash = mtx.TxIn[len(mtx.TxIn)-1].PreviousOutPoint.Hash.String()
		}
	}
	ticket, ok := liveTickets[ticketHash]
	if !ok {
		if tt, found := txByHash[ticketHash]; found {
			ticket = ticketInfo(tt)
		}
	}
	var returnAmount int64
	for _, c := range tx.credits {
		returnAmount += c.GetAmount()
	}
	return voteRevokeResult{
		wasWallet:          ticket.isWallet,
		isVote:             tx.txType == "vote",
		returnAmount:       returnAmount,
		stakeResult:        returnAmount - ticket.commitAmount,
		ticketCommitAmount: ticket.commitAmount,
	}
}

func txBalancesDelta(tx exportTx, maturingTxs map[int32][]maturingEntry, liveTickets map[string]ticketInfoResult, txByHash map[string]exportTx, cp exportChainParams) balDelta {
	switch tx.txType {
	case "ticket":
		ti := ticketInfo(tx)
		liveTickets[tx.hash] = ti
		d := balDelta{
			spendable:  -ti.spentAmount,
			ticket:     ti.commitAmount,
			stakeFees:  ti.purchaseFees,
			totalStake: ti.spentAmount,
			timestamp:  tx.timestamp,
		}
		if ti.isWallet {
			d.locked = ti.commitAmount
		} else {
			d.lockedNonWallet = ti.commitAmount
		}
		return d
	case "vote", "revocation":
		vr := voteRevokeInfo(tx, liveTickets, txByHash)
		matureHeight := tx.height + cp.coinbaseMaturity
		maturingTxs[matureHeight] = append(maturingTxs[matureHeight], maturingEntry{
			amount:   vr.returnAmount,
			isWallet: vr.wasWallet,
			isTicket: false,
		})
		d := balDelta{timestamp: tx.timestamp}
		if vr.wasWallet {
			d.locked = -vr.ticketCommitAmount
			d.immature = vr.returnAmount
		} else {
			d.lockedNonWallet = -vr.ticketCommitAmount
			d.immatureNonWallet = vr.returnAmount
		}
		if vr.isVote {
			d.voted = vr.returnAmount
			d.stakeRewards = vr.stakeResult
		} else {
			d.revoked = vr.returnAmount
			d.stakeFees = -vr.stakeResult
		}
		return d
	default: // regular, coinbase
		d := balDelta{spendable: tx.amount, timestamp: tx.timestamp}
		if tx.amount < 0 {
			d.sent = tx.amount
		} else if tx.amount > 0 {
			d.received = tx.amount
		}
		return d
	}
}

func findTimestampByBlockHeight(fromHeight, toHeight int32, fromTs, toTs int64, heightDelta int32, cp exportChainParams) int64 {
	if fromHeight == toHeight {
		return toTs
	}
	largeStakeTimeDiff := toHeight-fromHeight > cp.ticketMaturity &&
		toTs-fromTs > int64(toHeight-fromHeight)*cp.targetTimePerBlock
	if largeStakeTimeDiff {
		return fromTs + int64(heightDelta)*cp.targetTimePerBlock
	}
	blockInterval := float64(toTs-fromTs) / float64(toHeight-fromHeight)
	return fromTs + int64(float64(heightDelta)*blockInterval)
}

func findMaturingDeltas(maturingTxs map[int32][]maturingEntry, fromHeight, toHeight int32, fromTs, toTs int64, cp exportChainParams) []balDelta {
	var res []balDelta
	for h := fromHeight; h <= toHeight; h++ {
		entries, ok := maturingTxs[h]
		if !ok {
			continue
		}
		ts := findTimestampByBlockHeight(fromHeight, toHeight, fromTs, toTs, h-fromHeight, cp)
		d := balDelta{timestamp: ts}
		for _, e := range entries {
			if !e.isTicket {
				d.spendable += e.amount
			}
			if e.isWallet {
				d.immature += -e.amount
			} else {
				d.immatureNonWallet += -e.amount
			}
			if e.isTicket {
				if e.isWallet {
					d.locked += e.amount
				} else {
					d.lockedNonWallet += e.amount
				}
			}
		}
		res = append(res, d)
	}
	return res
}

func addDelta(d balDelta, cur balState) balState {
	b := balState{
		spendable:         cur.spendable + d.spendable,
		immature:          cur.immature + d.immature,
		immatureNonWallet: cur.immatureNonWallet + d.immatureNonWallet,
		locked:            cur.locked + d.locked + d.lockedNonWallet,
		lockedNonWallet:   0,
		stakeRewards:      cur.stakeRewards + d.stakeRewards,
		stakeFees:         cur.stakeFees + d.stakeFees,
		totalStake:        cur.totalStake + d.totalStake,
		delta:             d,
	}
	b.total = b.spendable + b.locked + b.immature
	return b
}

// exportBalances writes the Balances CSV, or (when daily) the Daily Balances
// CSV aggregated to one row per UTC day.
func exportBalances(ctx context.Context, cw *csvWriter, daily bool) error {
	cp, err := loadExportChainParams(ctx)
	if err != nil {
		return err
	}
	txs, err := fetchMinedExportTxs(ctx)
	if err != nil {
		return err
	}
	txByHash := make(map[string]exportTx, len(txs))
	for _, tx := range txs {
		txByHash[tx.hash] = tx
	}

	currentBlockHeight := int32(0)
	if len(txs) > 0 {
		currentBlockHeight = txs[len(txs)-1].height
	}
	if rpc.DcrdClient != nil {
		if h, herr := rpc.DcrdClient.GetBlockCount(ctx); herr == nil && int32(h) > currentBlockHeight {
			currentBlockHeight = int32(h)
		}
	}
	recentBlockTs := time.Now().Unix()

	// Header (series order matches StatisticsActions.js balancesStats).
	if daily {
		cw.writeLine(
			csvQuote("time"), csvQuote("spendable"), csvQuote("immature"), csvQuote("locked"),
			csvQuote("immatureNonWallet"), csvQuote("lockedNonWallet"), csvQuote("total"),
			csvQuote("stakeRewards"), csvQuote("stakeFees"), csvQuote("totalStake"),
			csvQuote("sent"), csvQuote("received"), csvQuote("voted"), csvQuote("revoked"), csvQuote("ticket"),
		)
	} else {
		cw.writeLine(
			csvQuote("time"), csvQuote("spendable"), csvQuote("immature"), csvQuote("locked"),
			csvQuote("immatureNonWallet"), csvQuote("lockedNonWallet"), csvQuote("total"),
			csvQuote("stakeRewards"), csvQuote("stakeFees"), csvQuote("totalStake"),
		)
	}

	agg := &dailyAgg{cw: cw}
	emit := func(b balState) {
		if daily {
			agg.add(b)
			return
		}
		cw.writeLine(
			csvQuote(csvTimeUTC(b.delta.timestamp)),
			csvAmount(b.spendable), csvAmount(b.immature), csvAmount(b.locked),
			csvAmount(b.immatureNonWallet), csvAmount(b.lockedNonWallet), csvAmount(b.total),
			csvAmount(b.stakeRewards), csvAmount(b.stakeFees), csvAmount(b.totalStake),
		)
	}

	maturingTxs := map[int32][]maturingEntry{}
	liveTickets := map[string]ticketInfoResult{}
	cur := balState{}
	lastTxHeight := int32(0)
	lastTxTs := cp.genesisTimestamp

	for _, tx := range txs {
		for _, md := range findMaturingDeltas(maturingTxs, lastTxHeight+1, tx.height, lastTxTs, tx.timestamp, cp) {
			cur = addDelta(md, cur)
			emit(cur)
		}
		d := txBalancesDelta(tx, maturingTxs, liveTickets, txByHash, cp)
		cur = addDelta(d, cur)
		emit(cur)
		lastTxHeight = tx.height
		lastTxTs = d.timestamp
	}
	for _, md := range findMaturingDeltas(maturingTxs, lastTxHeight+1, currentBlockHeight, lastTxTs, recentBlockTs, cp) {
		cur = addDelta(md, cur)
		emit(cur)
	}

	if daily {
		agg.finish()
	}
	return nil
}

// dailyAgg collapses per-event balance snapshots into one row per UTC day,
// carrying the day's final balance snapshot plus the summed daily deltas.
type dailyAgg struct {
	cw       *csvWriter
	have     bool
	lastDay  time.Time
	bal      balState
	sent     int64
	received int64
	voted    int64
	revoked  int64
	ticket   int64
}

func (a *dailyAgg) add(b balState) {
	t := time.Unix(b.delta.timestamp, 0).UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if !a.have || !day.Equal(a.lastDay) {
		if a.have {
			a.writeRow()
		}
		a.bal = b
		a.sent = b.delta.sent
		a.received = b.delta.received
		a.voted = b.delta.voted
		a.revoked = b.delta.revoked
		a.ticket = b.delta.ticket
		a.lastDay = day
		a.have = true
		return
	}
	a.bal = b
	a.sent += b.delta.sent
	a.received += b.delta.received
	a.voted += b.delta.voted
	a.revoked += b.delta.revoked
	a.ticket += b.delta.ticket
}

func (a *dailyAgg) finish() {
	if a.have {
		a.writeRow()
	}
}

func (a *dailyAgg) writeRow() {
	t := a.lastDay.Format("2006-01-02") + "T23:59:59.999Z"
	a.cw.writeLine(
		csvQuote(t),
		csvAmount(a.bal.spendable), csvAmount(a.bal.immature), csvAmount(a.bal.locked),
		csvAmount(a.bal.immatureNonWallet), csvAmount(a.bal.lockedNonWallet), csvAmount(a.bal.total),
		csvAmount(a.bal.stakeRewards), csvAmount(a.bal.stakeFees), csvAmount(a.bal.totalStake),
		csvAmount(a.sent), csvAmount(a.received), csvAmount(a.voted), csvAmount(a.revoked), csvAmount(a.ticket),
	)
}
