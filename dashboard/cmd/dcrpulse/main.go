// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"dcrpulse/internal/auth"
	"dcrpulse/internal/config"
	"dcrpulse/internal/handlers"
	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

//go:embed web/dist
var embeddedFiles embed.FS

func main() {
	// Load dcrd configuration from environment variables
	dcrdConfig := rpc.Config{
		RPCHost:     getEnv("DCRD_RPC_HOST", "localhost"),
		RPCPort:     getEnv("DCRD_RPC_PORT", "9109"),
		RPCUser:     getEnv("DCRD_RPC_USER", ""),
		RPCPassword: getEnv("DCRD_RPC_PASS", ""),
		RPCCert:     getEnv("DCRD_RPC_CERT", ""),
	}

	// Try to initialize dcrd RPC client if credentials are provided
	if dcrdConfig.RPCUser != "" && dcrdConfig.RPCPassword != "" {
		if err := rpc.InitDcrdClient(dcrdConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrd on startup: %v", err)
			log.Println("RPC connection can be configured via API")
		} else {
			// Seed + push dcrd sync progress, refreshed on block-connected
			// notifications (websocket) instead of a fixed poll interval.
			services.StartNodeSync(context.Background())
			if err := rpc.InitDcrdNotifyClient(dcrdConfig, services.TriggerNodeSyncRefresh); err != nil {
				log.Printf("Warning: dcrd notification client unavailable (progress falls back to timer): %v", err)
			}
		}
	} else {
		log.Println("No dcrd RPC credentials provided. Use /api/connect endpoint to configure.")
	}

	// Load dcrwallet configuration from environment variables
	walletConfig := rpc.Config{
		RPCHost:     getEnv("DCRWALLET_RPC_HOST", "localhost"),
		RPCPort:     getEnv("DCRWALLET_RPC_PORT", "9110"),
		RPCUser:     getEnv("DCRWALLET_RPC_USER", ""),
		RPCPassword: getEnv("DCRWALLET_RPC_PASS", ""),
		RPCCert:     getEnv("DCRWALLET_RPC_CERT", ""),
	}

	// Try to initialize wallet RPC client if credentials are provided
	if walletConfig.RPCUser != "" && walletConfig.RPCPassword != "" {
		if err := rpc.InitWalletClient(walletConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrwallet on startup: %v", err)
			log.Println("Wallet features will be unavailable")
		}
	} else {
		log.Println("No dcrwallet RPC credentials provided. Wallet features disabled.")
	}

	// Initialize wallet gRPC client for streaming
	grpcConfig := rpc.GrpcConfig{
		GrpcHost: getEnv("DCRWALLET_RPC_HOST", "localhost"),
		GrpcPort: getEnv("DCRWALLET_GRPC_PORT", "9111"),
		GrpcCert: getEnv("DCRWALLET_RPC_CERT", ""),
	}

	// Restore the active wallet selection (or default to the legacy wallet on
	// upgraded deployments) before the sync supervisor starts.
	services.SeedActiveWallet()

	if grpcConfig.GrpcCert != "" {
		if err := rpc.InitWalletGrpcClient(grpcConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrwallet gRPC on startup: %v", err)
			log.Println("Streaming features will be unavailable")
		} else {
			// Supervise RpcSync from dcrd. Resumes automatically when the
			// wallet is loaded, reconnects with backoff if the stream dies.
			go superviseRpcSync(context.Background())
		}
	} else {
		log.Println("No gRPC certificate provided. Streaming features disabled.")
	}

	// Best-effort dcrlnd connection. dcrlnd may still be locked or
	// uninitialised at this point; failures are logged and the
	// dashboard re-tries on demand via ReinitDcrlndClient.
	// Dial each downstream service with the ACTIVE wallet's per-wallet cert
	// paths (the default wallet resolves to the legacy paths).
	activeWallet := services.CurrentWalletName()
	dcrlndCfg := rpc.DcrlndConfig{
		GrpcHost:     getEnv("DCRLND_HOST", "dcrlnd"),
		GrpcPort:     getEnv("DCRLND_GRPC_PORT", "10009"),
		TLSCertPath:  config.DcrlndTLSCert(activeWallet),
		MacaroonPath: config.DcrlndMacaroon(activeWallet),
	}
	if err := rpc.InitDcrlndClient(dcrlndCfg); err != nil {
		log.Printf("Warning: dcrlnd init: %v", err)
	}

	// brclientd clientrpc config. The cert pair is owned by brclientd and
	// mounted read-only into this container; lazy init means the dashboard
	// starts even before brclientd has provisioned its identity and certs.
	brServerCert, brClientCert, brClientKey := services.BrclientdDaemonCertPaths(context.Background())
	rpc.InitBrclientdConfig(rpc.BrclientdConfig{
		Host:           getEnv("BRCLIENTD_HOST", "brclientd"),
		Port:           getEnv("BRCLIENTD_PORT", "7676"),
		StatusPort:     getEnv("BRCLIENTD_STATUS_PORT", "7677"),
		ServerCertPath: brServerCert,
		ClientCertPath: brClientCert,
		ClientKeyPath:  brClientKey,
	})

	// dcrdex (bisonw) backend-only RPC config. The RPC cert is owned by the
	// dcrdex container and mounted read-only here; the client is built lazily
	// because bisonw generates the cert on first run.
	rpc.InitDcrdexConfig(rpc.DcrdexConfig{
		Host:       getEnv("DCRDEX_RPC_HOST", "dcrdex"),
		Port:       getEnv("DCRDEX_RPC_PORT", "5757"),
		User:       getEnv("DCRDEX_RPC_USER", "dcrdex"),
		Pass:       getEnv("DCRDEX_RPC_PASS", "dcrdexpass"),
		CertPath:   config.DcrdexCert(activeWallet),
		WSPort:     getEnv("DCRDEX_WS_PORT", "5758"),
		WSCertPath: config.DcrdexWSCert(activeWallet),
	})

	// Tail dcrwallet's log file for mixer-relevant entries; pushes them into
	// the same ring buffer the /wallet/privacy/events WebSocket reads from.
	services.StartWalletLogTail()

	// Background refresh of the Bison Relay peer-preset cache used by the
	// Channels tab's open-channel form.
	services.StartBrseederRefresh()

	// Persistent WS subscriptions to brclientd for chat / KX / GC events.
	services.StartBisonrelayStreams(context.Background())
	services.StartBrclientdNotifs(context.Background())

	// Load the optional dashboard app-password gate (off unless configured).
	if err := auth.Init(); err != nil {
		log.Printf("Warning: could not load app-password config (auth disabled): %v", err)
	}

	// Setup router
	r := mux.NewRouter()
	r.Use(middleware.SecurityHeaders)

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.Use(middleware.RequireSameOrigin, middleware.LimitJSONBody(1<<20), auth.RequireAuth)

	// Dashboard app-password (optional). /auth/status + /auth/login are exempt
	// from RequireAuth so the user can reach the login handshake; every other
	// /api route is gated once the password is enabled.
	api.HandleFunc("/auth/status", handlers.AuthStatusHandler).Methods("GET")
	api.Handle("/auth/login",
		middleware.RateLimit("auth-login", time.Second, 5)(
			http.HandlerFunc(handlers.AuthLoginHandler))).Methods("POST")
	api.HandleFunc("/auth/setup", handlers.AuthSetupHandler).Methods("POST")
	api.HandleFunc("/auth/skip-setup", handlers.AuthSkipSetupHandler).Methods("POST")
	api.HandleFunc("/auth/logout", handlers.AuthLogoutHandler).Methods("POST")
	api.HandleFunc("/auth/change", handlers.AuthChangeHandler).Methods("POST")
	api.HandleFunc("/auth/disable", handlers.AuthDisableHandler).Methods("POST")

	// DCRDEX routes
	api.HandleFunc("/dcrdex/status", handlers.GetDcrdexStatusHandler).Methods("GET")
	api.HandleFunc("/dcrdex/init", handlers.InitDcrdexHandler).Methods("POST")
	api.HandleFunc("/dcrdex/unlock", handlers.UnlockDcrdexHandler).Methods("POST")
	api.HandleFunc("/dcrdex/lock", handlers.LockDcrdexHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet", handlers.CreateDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet", handlers.GetDcrdexWalletHandler).Methods("GET")
	api.HandleFunc("/dcrdex/exchanges", handlers.GetDcrdexExchangesHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallets", handlers.GetDcrdexWalletsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/account", handlers.GetDcrdexAccountHandler).Methods("GET")
	api.HandleFunc("/dcrdex/bondopts", handlers.SetDcrdexBondOptionsHandler).Methods("POST")
	api.HandleFunc("/dcrdex/dexconfig", handlers.GetDcrdexConfigHandler).Methods("GET")
	api.HandleFunc("/dcrdex/postbond", handlers.PostDcrdexBondHandler).Methods("POST")
	api.HandleFunc("/dcrdex/ws", handlers.DcrdexWSHandler).Methods("GET")
	api.HandleFunc("/dcrdex/notify", handlers.DcrdexNotifyWSHandler).Methods("GET")
	api.HandleFunc("/dcrdex/myorders", handlers.GetDcrdexMyOrdersHandler).Methods("GET")
	api.HandleFunc("/dcrdex/orders", handlers.GetDcrdexOrdersHandler).Methods("POST")
	api.HandleFunc("/dcrdex/order", handlers.GetDcrdexSingleOrderHandler).Methods("POST")
	api.HandleFunc("/dcrdex/cancel", handlers.CancelDcrdexOrderHandler).Methods("POST")
	api.HandleFunc("/dcrdex/trade", handlers.PlaceDcrdexOrderHandler).Methods("POST")
	api.HandleFunc("/dcrdex/preorder", handlers.PreDcrdexOrderHandler).Methods("POST")
	api.HandleFunc("/dcrdex/maxbuy", handlers.MaxDcrdexBuyHandler).Methods("POST")
	api.HandleFunc("/dcrdex/maxsell", handlers.MaxDcrdexSellHandler).Methods("POST")
	api.HandleFunc("/dcrdex/assets", handlers.GetDcrdexAssetsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallet/create", handlers.CreateDcrdexAssetWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/txs", handlers.GetDcrdexWalletTxsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallet/tx", handlers.GetDcrdexWalletTxHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallet/send", handlers.SendDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/txfee", handlers.EstimateDcrdexSendFeeHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/open", handlers.OpenDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/close", handlers.CloseDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/toggle", handlers.ToggleDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/rescan", handlers.RescanDcrdexWalletHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/new-address", handlers.NewDexDepositAddressHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/address-used", handlers.DexAddressUsedHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallet/peers", handlers.GetDcrdexWalletPeersHandler).Methods("GET")
	api.HandleFunc("/dcrdex/wallet/peers", handlers.AddDcrdexWalletPeerHandler).Methods("POST")
	api.HandleFunc("/dcrdex/wallet/peers", handlers.RemoveDcrdexWalletPeerHandler).Methods("DELETE")
	api.HandleFunc("/dcrdex/notifications", handlers.GetDcrdexNotificationsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/rates", handlers.GetDcrdexRatesHandler).Methods("GET")
	api.HandleFunc("/dcrdex/seed", handlers.ExportDcrdexSeedHandler).Methods("POST")
	api.HandleFunc("/dcrdex/seed/backed-up", handlers.MarkDcrdexSeedBackedUpHandler).Methods("POST")
	api.HandleFunc("/dcrdex/discover-account", handlers.DiscoverDcrdexAccountHandler).Methods("POST")
	api.HandleFunc("/dcrdex/mm/status", handlers.GetDcrdexMMStatusHandler).Methods("GET")
	api.HandleFunc("/dcrdex/mm/marketreport", handlers.GetDcrdexMMMarketReportHandler).Methods("GET")
	api.HandleFunc("/dcrdex/mm/runlogs", handlers.GetDcrdexMMRunLogsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/mm/archivedruns", handlers.GetDcrdexMMArchivedRunsHandler).Methods("GET")
	api.HandleFunc("/dcrdex/mm/config", handlers.UpdateDcrdexMMBotConfigHandler).Methods("POST")
	api.HandleFunc("/dcrdex/mm/config/remove", handlers.RemoveDcrdexMMBotConfigHandler).Methods("POST")
	api.HandleFunc("/dcrdex/mm/cexconfig", handlers.UpdateDcrdexMMCexConfigHandler).Methods("POST")
	api.HandleFunc("/dcrdex/mm/start", handlers.StartDcrdexMMBotHandler).Methods("POST")
	api.HandleFunc("/dcrdex/mm/stop", handlers.StopDcrdexMMBotHandler).Methods("POST")

	// Node/dcrd routes
	api.HandleFunc("/health", handlers.HealthCheckHandler).Methods("GET")
	api.HandleFunc("/dashboard", handlers.GetDashboardDataHandler).Methods("GET")
	api.HandleFunc("/node/status", handlers.GetNodeStatusHandler).Methods("GET")
	api.HandleFunc("/node/sync/stream", handlers.StreamNodeSyncHandler).Methods("GET")
	api.HandleFunc("/blockchain/info", handlers.GetBlockchainInfoHandler).Methods("GET")
	api.HandleFunc("/network/peers", handlers.GetPeersHandler).Methods("GET")

	// Multi-wallet routes. select/create/delete relaunch the dcrwallet daemon,
	// so they are rate limited like other daemon-cycling endpoints.
	api.HandleFunc("/wallets", handlers.ListWalletsHandler).Methods("GET")
	api.Handle("/wallets/select",
		middleware.RateLimit("wallet-select", 5*time.Second, 1)(
			http.HandlerFunc(handlers.SelectWalletHandler))).Methods("POST")
	api.Handle("/wallets/create",
		middleware.RateLimit("wallet-create", 5*time.Second, 1)(
			http.HandlerFunc(handlers.CreateNamedWalletHandler))).Methods("POST")
	api.Handle("/wallets/rename",
		middleware.RateLimit("wallet-rename", 5*time.Second, 1)(
			http.HandlerFunc(handlers.RenameWalletHandler))).Methods("POST")
	api.Handle("/wallets/delete",
		middleware.RateLimit("wallet-delete", 5*time.Second, 1)(
			http.HandlerFunc(handlers.DeleteWalletHandler))).Methods("POST")
	api.HandleFunc("/wallet/close", handlers.CloseWalletHandler).Methods("POST")

	// Wallet routes
	api.HandleFunc("/wallet/exists", handlers.WalletExistsHandler).Methods("GET")
	api.HandleFunc("/wallet/loaded", handlers.WalletLoadedHandler).Methods("GET")
	api.HandleFunc("/wallet/generate-seed", handlers.GenerateSeedHandler).Methods("POST")
	api.HandleFunc("/wallet/decode-seed", handlers.DecodeSeedHandler).Methods("POST")
	api.HandleFunc("/wallet/seed-words", handlers.SeedWordsHandler).Methods("GET")
	api.HandleFunc("/wallet/create", handlers.CreateWalletHandler).Methods("POST")
	api.HandleFunc("/wallet/open", handlers.OpenWalletHandler).Methods("POST")
	api.HandleFunc("/wallet/status", handlers.GetWalletStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/dashboard", handlers.GetWalletDashboardHandler).Methods("GET")
	api.HandleFunc("/wallet/transactions", handlers.ListTransactionsHandler).Methods("GET")
	api.Handle("/wallet/importxpub",
		middleware.RateLimit("importxpub", 30*time.Second, 1)(
			http.HandlerFunc(handlers.ImportXpubHandler))).Methods("POST")
	api.HandleFunc("/wallet/accounts", handlers.GetAccountsHandler).Methods("GET")
	api.HandleFunc("/wallet/create-account", handlers.CreateAccountHandler).Methods("POST")
	api.HandleFunc("/wallet/rename-account", handlers.RenameAccountHandler).Methods("POST")
	api.HandleFunc("/wallet/account-extended-pubkey", handlers.GetAccountExtendedPubKeyHandler).Methods("GET")
	api.HandleFunc("/wallet/privacy/status", handlers.PrivacyStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/privacy/setup", handlers.PrivacySetupHandler).Methods("POST")
	api.HandleFunc("/wallet/privacy/start", handlers.PrivacyStartHandler).Methods("POST")
	api.HandleFunc("/wallet/privacy/stop", handlers.PrivacyStopHandler).Methods("POST")
	api.HandleFunc("/wallet/privacy/events", handlers.StreamMixerEventsHandler).Methods("GET")
	api.HandleFunc("/wallet/mixer/debug", handlers.MixerDebugHandler).Methods("GET", "POST")
	api.HandleFunc("/wallet/staking/vsps", handlers.ListVSPsHandler).Methods("GET")
	api.HandleFunc("/wallet/staking/vsp-info", handlers.VSPInfoHandler).Methods("GET")
	api.HandleFunc("/wallet/staking/purchase", handlers.PurchaseTicketsHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/tickets", handlers.ListTicketsHandler).Methods("GET")
	api.HandleFunc("/wallet/staking/sync-failed-vsp-tickets", handlers.SyncFailedVSPTicketsHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/process-unmanaged-vsp-tickets", handlers.ProcessUnmanagedVSPTicketsHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/autobuyer/status", handlers.AutobuyerStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/staking/autobuyer/settings", handlers.GetAutobuyerSettingsHandler).Methods("GET")
	api.HandleFunc("/wallet/staking/autobuyer/settings", handlers.SaveAutobuyerSettingsHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/autobuyer/start", handlers.StartAutobuyerHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/autobuyer/stop", handlers.StopAutobuyerHandler).Methods("POST")
	api.HandleFunc("/wallet/staking/autobuyer/events", handlers.StreamAutobuyerEventsHandler).Methods("GET")
	api.HandleFunc("/wallet/settings", handlers.GetSettingsHandler).Methods("GET")
	api.HandleFunc("/wallet/settings", handlers.SaveSettingsHandler).Methods("POST")
	api.HandleFunc("/wallet/settings/change-passphrase", handlers.ChangePassphraseHandler).Methods("POST")
	api.Handle("/wallet/settings/discover-addresses",
		middleware.RateLimit("discover-addresses", 30*time.Second, 1)(
			http.HandlerFunc(handlers.DiscoverAddressesHandler))).Methods("POST")
	api.HandleFunc("/wallet/settings/logs", handlers.GetLogsHandler).Methods("GET")
	api.HandleFunc("/themes", handlers.GetThemesHandler).Methods("GET")
	api.HandleFunc("/themes", handlers.SaveThemesHandler).Methods("POST")
	api.HandleFunc("/tor", handlers.GetTorHandler).Methods("GET")
	api.HandleFunc("/tor", handlers.SetTorHandler).Methods("POST")
	api.HandleFunc("/tor/status", handlers.GetTorStatusHandler).Methods("GET")
	api.HandleFunc("/tor/control", handlers.GetTorControlHandler).Methods("GET")
	api.HandleFunc("/tor/newidentity", handlers.TorNewIdentityHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/agendas", handlers.GetAgendasHandler).Methods("GET")
	api.HandleFunc("/wallet/governance/agendas/set", handlers.SetAgendaChoiceHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/treasury/keys", handlers.GetTreasuryKeyPoliciesHandler).Methods("GET")
	api.HandleFunc("/wallet/governance/treasury/keys/set", handlers.SetTreasuryKeyPolicyHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/treasury/tspends", handlers.GetTSpendPoliciesHandler).Methods("GET")
	api.HandleFunc("/wallet/governance/treasury/tspends/set", handlers.SetTSpendPolicyHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/proposals", handlers.GetProposalsHandler).Methods("GET")
	api.HandleFunc("/wallet/governance/proposals/{token}", handlers.GetProposalDetailHandler).Methods("GET")
	api.HandleFunc("/wallet/governance/proposals/cast-vote", handlers.CastPoliteiaVoteHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/proposals/refresh", handlers.RefreshProposalsHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/proposals/{token}/refresh", handlers.RefreshProposalDetailHandler).Methods("POST")
	api.HandleFunc("/wallet/governance/proposals/{token}/vote-eligibility", handlers.PrepareProposalVoteHandler).Methods("POST")
	api.HandleFunc("/br/version", handlers.BisonrelayVersionHandler).Methods("GET")
	api.HandleFunc("/br/status", handlers.BisonrelayStatusHandler).Methods("GET")
	api.HandleFunc("/br/setup", handlers.BisonrelaySetupHandler).Methods("POST")
	api.HandleFunc("/br/backup", handlers.BisonrelayBackupHandler).Methods("GET")
	api.HandleFunc("/br/backup/prepare", handlers.BisonrelayBackupPrepareHandler).Methods("POST")
	api.HandleFunc("/br/backup/status", handlers.BisonrelayBackupStatusHandler).Methods("GET")
	api.HandleFunc("/br/backup/restore", handlers.BisonrelayRestoreBackupHandler).Methods("POST")
	api.HandleFunc("/br/identity", handlers.BisonrelayIdentityHandler).Methods("GET")
	api.HandleFunc("/br/avatar", handlers.BisonrelaySetAvatarHandler).Methods("POST")
	api.HandleFunc("/br/messages", handlers.BisonrelayMessagesHandler).Methods("GET")
	api.HandleFunc("/br/messages/clear", handlers.BisonrelayClearHistoryHandler).Methods("POST")
	api.HandleFunc("/br/contacts", handlers.BisonrelayContactsHandler).Methods("GET")
	api.HandleFunc("/br/contacts/rename", handlers.BisonrelayContactRenameHandler).Methods("POST")
	api.HandleFunc("/br/contacts/kx-reset", handlers.BisonrelayContactKXResetHandler).Methods("POST")
	api.HandleFunc("/br/contacts/reset-all", handlers.BisonrelayContactResetAllHandler).Methods("POST")
	api.HandleFunc("/br/connection", handlers.BisonrelayConnectionHandler).Methods("GET", "POST")
	api.HandleFunc("/br/settings/receivereceipts", handlers.BisonrelayReceiveReceiptsHandler).Methods("GET", "POST")
	api.HandleFunc("/br/notifications/recent", handlers.BisonrelayRecentNotificationsHandler).Methods("GET")
	api.HandleFunc("/br/payments/tips", handlers.BisonrelayTipAttemptsHandler).Methods("GET")
	api.HandleFunc("/br/payments/tips/running", handlers.BisonrelayRunningTipsHandler).Methods("GET")
	api.HandleFunc("/br/filters", handlers.BisonrelayFiltersHandler).Methods("GET", "POST")
	api.HandleFunc("/br/filters/delete", handlers.BisonrelayFilterDeleteHandler).Methods("POST")
	api.HandleFunc("/br/posts/subscribe-all", handlers.BisonrelaySubscribeAllPostsHandler).Methods("POST")
	api.HandleFunc("/br/kx/list", handlers.BisonrelayKXListHandler).Methods("GET")
	api.HandleFunc("/br/kx/searches", handlers.BisonrelayKXSearchesHandler).Methods("GET")
	api.HandleFunc("/br/kx/mediateids", handlers.BisonrelayMediateIDsHandler).Methods("GET", "POST")
	api.HandleFunc("/br/contacts/block", handlers.BisonrelayContactBlockHandler).Methods("POST")
	api.HandleFunc("/br/contacts/ignore", handlers.BisonrelayContactIgnoreHandler).Methods("POST")
	api.HandleFunc("/br/contacts/handshake", handlers.BisonrelayContactHandshakeHandler).Methods("POST")
	api.HandleFunc("/br/contacts/suggest-kx", handlers.BisonrelayContactSuggestKXHandler).Methods("POST")
	api.HandleFunc("/br/contacts/trans-reset", handlers.BisonrelayContactTransResetHandler).Methods("POST")
	api.HandleFunc("/br/contacts/accept-suggestion", handlers.BisonrelayContactAcceptSuggestionHandler).Methods("POST")
	api.HandleFunc("/br/contacts/tip", handlers.BisonrelayContactTipHandler).Methods("POST")
	api.HandleFunc("/br/contacts/subscribe-posts", handlers.BisonrelayContactSubscribePostsHandler).Methods("POST")
	api.HandleFunc("/br/contacts/unsubscribe-posts", handlers.BisonrelayContactUnsubscribePostsHandler).Methods("POST")
	api.HandleFunc("/br/contacts/list-posts", handlers.BisonrelayContactListPostsHandler).Methods("POST")
	api.HandleFunc("/br/contacts/list-content", handlers.BisonrelayContactListContentHandler).Methods("POST")
	api.HandleFunc("/br/contacts/fetch-post", handlers.BisonrelayContactFetchPostHandler).Methods("POST")
	api.HandleFunc("/br/posts", handlers.BisonrelayPostsFeedHandler).Methods("GET")
	api.HandleFunc("/br/posts/body", handlers.BisonrelayPostBodyHandler).Methods("GET")
	api.HandleFunc("/br/posts/embed-data", handlers.BisonrelayPostsEmbedDataHandler).Methods("GET")
	api.HandleFunc("/br/posts/comments", handlers.BisonrelayPostCommentsHandler).Methods("GET")
	api.HandleFunc("/br/posts/comment", handlers.BisonrelayPostCommentHandler).Methods("POST")
	api.HandleFunc("/br/posts/hearts", handlers.BisonrelayPostHeartsHandler).Methods("GET")
	api.HandleFunc("/br/posts/heart", handlers.BisonrelayPostHeartHandler).Methods("POST")
	api.HandleFunc("/br/posts/receivereceipts", handlers.BisonrelayPostReceiveReceiptsHandler).Methods("GET")
	api.HandleFunc("/br/posts/comment-receivereceipts", handlers.BisonrelayPostCommentReceiptsHandler).Methods("GET")
	api.HandleFunc("/br/posts/relay", handlers.BisonrelayPostRelayHandler).Methods("POST")
	api.HandleFunc("/br/posts/new", handlers.BisonrelayPostsNewHandler).Methods("POST")
	api.HandleFunc("/br/posts/render", handlers.BisonrelayPostsRenderHandler).Methods("POST")
	api.HandleFunc("/br/pages/render", handlers.BisonrelayPagesRenderHandler).Methods("POST")
	api.HandleFunc("/br/shared-files", handlers.BisonrelaySharedFilesHandler).Methods("GET")
	api.HandleFunc("/br/embeds/{contact}/{filename}", handlers.BisonrelayEmbedHandler).Methods("GET")
	api.HandleFunc("/br/downloads/{contact}", handlers.BisonrelayDownloadsListHandler).Methods("GET")
	api.HandleFunc("/br/downloads/{contact}/{filename}", handlers.BisonrelayDownloadHandler).Methods("GET")
	api.HandleFunc("/br/files/send", handlers.BisonrelayFileSendHandler).Methods("POST")
	api.HandleFunc("/br/files/add", handlers.BisonrelayManageAddHandler).Methods("POST")
	api.HandleFunc("/br/files/shared/remove", handlers.BisonrelayManageUnshareHandler).Methods("POST")
	api.HandleFunc("/br/files/downloads", handlers.BisonrelayManageDownloadsHandler).Methods("GET")
	api.HandleFunc("/br/files/downloads/cancel", handlers.BisonrelayManageCancelDownloadHandler).Methods("POST")
	api.HandleFunc("/br/content/get", handlers.BisonrelayContentGetHandler).Methods("POST")
	api.HandleFunc("/br/content/file", handlers.BisonrelayContentFileHandler).Methods("GET")
	api.HandleFunc("/br/rates", handlers.BisonrelayRatesHandler).Methods("GET")
	api.HandleFunc("/br/store/mode", handlers.BisonrelayStoreModeHandler).Methods("GET", "POST")
	api.HandleFunc("/br/store/products", handlers.BisonrelayStoreProductsHandler).Methods("GET", "POST")
	api.HandleFunc("/br/store/products/delete", handlers.BisonrelayStoreProductDeleteHandler).Methods("POST")
	api.HandleFunc("/br/store/orders", handlers.BisonrelayStoreOrdersHandler).Methods("GET")
	api.HandleFunc("/br/store/orders/status", handlers.BisonrelayStoreOrderStatusHandler).Methods("POST")
	api.HandleFunc("/br/store/orders/comment", handlers.BisonrelayStoreOrderCommentHandler).Methods("POST")
	api.HandleFunc("/br/store/files/upload", handlers.BisonrelayStoreFileUploadHandler).Methods("POST")
	api.HandleFunc("/br/store/templates", handlers.BisonrelayStoreTemplatesHandler).Methods("GET")
	api.HandleFunc("/br/store/templates/file", handlers.BisonrelayStoreTemplateFileHandler).Methods("GET")
	api.HandleFunc("/br/store/templates/save", handlers.BisonrelayStoreTemplateSaveHandler).Methods("POST")
	api.HandleFunc("/br/store/templates/delete", handlers.BisonrelayStoreTemplateDeleteHandler).Methods("POST")
	api.HandleFunc("/br/pages/fetch", handlers.BisonrelayPagesFetchHandler).Methods("POST")
	api.HandleFunc("/br/pages/local", handlers.BisonrelayPagesLocalListHandler).Methods("GET")
	api.HandleFunc("/br/pages/local/file", handlers.BisonrelayPagesLocalFileHandler).Methods("GET")
	api.HandleFunc("/br/pages/local/save", handlers.BisonrelayPagesLocalSaveHandler).Methods("POST")
	api.HandleFunc("/br/pages/local/delete", handlers.BisonrelayPagesLocalDeleteHandler).Methods("POST")
	api.HandleFunc("/br/stats/overview", handlers.BisonrelayStatsOverviewHandler).Methods("GET")
	api.HandleFunc("/br/stats/payments", handlers.BisonrelayStatsPaymentsHandler).Methods("GET")
	api.HandleFunc("/br/stats/network", handlers.BisonrelayStatsNetworkHandler).Methods("GET")
	api.HandleFunc("/br/stats/contacts", handlers.BisonrelayStatsContactsHandler).Methods("GET")
	api.HandleFunc("/br/stats/posts", handlers.BisonrelayStatsPostsHandler).Methods("GET")
	api.HandleFunc("/br/rtdt/sessions", handlers.BisonrelayRTDTListHandler).Methods("GET")
	api.HandleFunc("/br/rtdt/sessions/create", handlers.BisonrelayRTDTCreateHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/create-instant", handlers.BisonrelayRTDTCreateInstantHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/invite", handlers.BisonrelayRTDTInviteHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/accept", handlers.BisonrelayRTDTAcceptHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/join", handlers.BisonrelayRTDTJoinHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/leave", handlers.BisonrelayRTDTLeaveHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/dissolve", handlers.BisonrelayRTDTDissolveHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/kick", handlers.BisonrelayRTDTKickHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/remove", handlers.BisonrelayRTDTRemoveHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/rotate-cookies", handlers.BisonrelayRTDTRotateCookiesHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/messages", handlers.BisonrelayRTDTMessagesHandler).Methods("GET")
	api.HandleFunc("/br/rtdt/sessions/{rv}/chat", handlers.BisonrelayRTDTChatHandler).Methods("POST")
	api.HandleFunc("/br/rtdt/sessions/{rv}/audio", handlers.BisonrelayRTDTAudioHandler).Methods("GET")
	api.HandleFunc("/br/gc", handlers.BisonrelayGCListHandler).Methods("GET")
	api.HandleFunc("/br/gc/create", handlers.BisonrelayGCCreateHandler).Methods("POST")
	api.HandleFunc("/br/gc/invites", handlers.BisonrelayGCInvitesListHandler).Methods("GET")
	api.HandleFunc("/br/gc/invites/accept", handlers.BisonrelayGCInvitesAcceptHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}", handlers.BisonrelayGCDetailHandler).Methods("GET")
	api.HandleFunc("/br/gc/{gcid}/invite", handlers.BisonrelayGCInviteHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/message", handlers.BisonrelayGCMessageHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/history", handlers.BisonrelayGCHistoryHandler).Methods("GET")
	api.HandleFunc("/br/gc/{gcid}/part", handlers.BisonrelayGCPartHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/kill", handlers.BisonrelayGCKillHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/kick", handlers.BisonrelayGCKickHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/block", handlers.BisonrelayGCBlockHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/unblock", handlers.BisonrelayGCUnblockHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/admins", handlers.BisonrelayGCAdminsHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/owner", handlers.BisonrelayGCOwnerHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/upgrade", handlers.BisonrelayGCUpgradeHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/alias", handlers.BisonrelayGCAliasHandler).Methods("POST")
	api.HandleFunc("/br/gc/{gcid}/resend-list", handlers.BisonrelayGCResendListHandler).Methods("POST")
	api.HandleFunc("/br/pm", handlers.BisonrelayPMHandler).Methods("POST")
	api.HandleFunc("/br/invites/write", handlers.BisonrelayInviteWriteHandler).Methods("POST")
	api.HandleFunc("/br/invites/accept", handlers.BisonrelayInviteAcceptHandler).Methods("POST")
	api.HandleFunc("/br/join-decred-pulse", handlers.JoinDecredPulseHandler).Methods("POST")
	api.HandleFunc("/br/events", handlers.BisonrelayEventsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/status", handlers.LightningStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/setup", handlers.LightningSetupHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/unlock", handlers.LightningUnlockHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/info", handlers.LightningInfoHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/balance", handlers.LightningBalanceHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/activity", handlers.LightningActivityHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/channels", handlers.LightningChannelsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/channels/open", handlers.LightningOpenChannelHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/channels/close", handlers.LightningCloseChannelHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/peer-presets", handlers.LightningPeerPresetsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/liquidity/defaults", handlers.LightningLiquidityDefaultsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/liquidity/estimate", handlers.LightningLiquidityEstimateHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/liquidity/request", handlers.LightningLiquidityRequestHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/autopilot", handlers.LightningAutopilotStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/autopilot", handlers.LightningAutopilotSetHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/graph/search", handlers.LightningGraphSearchHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/channel-events", handlers.LightningChannelEventsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/network", handlers.LightningNetworkHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/send/decode", handlers.LightningDecodePayReqHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/send", handlers.LightningSendPaymentHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/payments", handlers.LightningPaymentsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/invoices", handlers.LightningInvoicesHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/invoices/add", handlers.LightningAddInvoiceHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/invoices/cancel", handlers.LightningCancelInvoiceHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/invoice-events", handlers.LightningInvoiceEventsHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/backup", handlers.LightningBackupExportHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/backup/verify", handlers.LightningBackupVerifyHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/watchtowers", handlers.LightningWatchtowersHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/watchtowers/add", handlers.LightningWatchtowerAddHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/watchtowers/remove", handlers.LightningWatchtowerRemoveHandler).Methods("POST")
	api.HandleFunc("/wallet/ln/graph/node", handlers.LightningGraphNodeHandler).Methods("GET")
	api.HandleFunc("/wallet/ln/graph/routes", handlers.LightningGraphRoutesHandler).Methods("POST")
	api.HandleFunc("/wallet/next-address", handlers.NextAddressHandler).Methods("GET")
	api.HandleFunc("/wallet/validate-address", handlers.ValidateAddressHandler).Methods("GET")
	api.HandleFunc("/wallet/construct-transaction", handlers.ConstructTransactionHandler).Methods("POST")
	api.HandleFunc("/wallet/sign-publish-transaction", handlers.SignPublishTransactionHandler).Methods("POST")
	api.Handle("/wallet/rescan",
		middleware.RateLimit("rescan", 60*time.Second, 1)(
			http.HandlerFunc(handlers.RescanWalletHandler))).Methods("POST")
	api.HandleFunc("/wallet/sync-progress", handlers.GetSyncProgressHandler).Methods("GET")

	// WebSocket streaming routes (log-based monitoring, does not start rescans)
	api.HandleFunc("/wallet/stream-rescan-progress", handlers.StreamRescanProgressHandler).Methods("GET")
	api.HandleFunc("/wallet/grpc/stream-rescan", handlers.StreamRescanGrpcHandler).Methods("GET")

	// Explorer routes
	api.HandleFunc("/explorer/search", handlers.SearchHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/recent", handlers.GetRecentBlocksHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/{height:[0-9]+}", handlers.GetBlockByHeightHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/hash/{hash}", handlers.GetBlockByHashHandler).Methods("GET")
	api.HandleFunc("/explorer/transactions/{txhash}", handlers.GetTransactionHandler).Methods("GET")
	api.HandleFunc("/explorer/address/{address}", handlers.GetAddressHandler).Methods("GET")
	api.HandleFunc("/explorer/mempool", handlers.GetMempoolTransactionsHandler).Methods("GET")

	// Treasury/Governance routes
	api.HandleFunc("/treasury/info", handlers.GetTreasuryInfoHandler).Methods("GET")
	api.HandleFunc("/treasury/balance-history", handlers.GetTreasuryBalanceHistoryHandler).Methods("GET")
	api.Handle("/treasury/scan-history",
		middleware.RateLimit("treasury-scan", 60*time.Second, 1)(
			http.HandlerFunc(handlers.TriggerTSpendScanHandler))).Methods("POST")
	api.HandleFunc("/treasury/scan-progress", handlers.GetTSpendScanProgressHandler).Methods("GET")
	api.HandleFunc("/treasury/scan-results", handlers.GetTSpendScanResultsHandler).Methods("GET")
	api.HandleFunc("/treasury/mempool", handlers.GetMempoolTSpendsHandler).Methods("GET")
	api.HandleFunc("/treasury/votes/{txhash}/progress", handlers.GetVoteParsingProgressHandler).Methods("GET")

	// Serve embedded static files for frontend
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		log.Printf("Warning: Could not load embedded frontend files: %v", err)
		log.Println("Frontend will not be available. This is expected in development mode.")
	} else {
		// Serve static files with SPA fallback
		r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			path := req.URL.Path

			// Skip API routes
			if strings.HasPrefix(path, "/api") {
				http.NotFound(w, req)
				return
			}

			// Try to serve the requested file
			if path != "/" {
				filePath := strings.TrimPrefix(path, "/")
				if _, err := distFS.Open(filePath); err == nil {
					http.FileServer(http.FS(distFS)).ServeHTTP(w, req)
					return
				}
			}

			// Fallback to index.html for SPA routing
			req.URL.Path = "/"
			http.FileServer(http.FS(distFS)).ServeHTTP(w, req)
		})
	}

	// Start server
	port := getEnv("PORT", "8080")
	address := fmt.Sprintf(":%s", port)

	log.Printf("Starting dcrpulse Dashboard server on %s", address)
	log.Println("Node endpoints: /api/dashboard, /api/node/*, /api/blockchain/*, /api/network/*")
	log.Println("Wallet endpoints: /api/wallet/status, /api/wallet/dashboard, /api/wallet/importxpub")
	log.Println("Wallet gRPC endpoints: /api/wallet/grpc/stream-rescan (real-time streaming)")
	log.Println("Explorer endpoints: /api/explorer/search, /api/explorer/blocks/*, /api/explorer/transactions/*")
	log.Println("Treasury endpoints: /api/treasury/info, /api/treasury/scan-history, /api/treasury/scan-progress")
	log.Println("Frontend: Embedded static files served at /")
	// ReadHeaderTimeout bounds the header-read phase to defeat Slowloris. Read
	// and Write timeouts are intentionally left unset so long-lived streams
	// (WebSocket rescan/events, SSE progress) and large BR file uploads are not
	// cut off mid-transfer.
	srv := &http.Server{
		Addr:              address,
		Handler:           r,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// superviseRpcSync keeps an RpcSync stream open whenever the wallet is
// loaded; reconnects with backoff on failure.
func superviseRpcSync(ctx context.Context) {
	firstStart := true
	backoff := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		// While a wallet switch tears the daemon down and brings a different
		// wallet up, park here so we neither touch the gRPC clients being
		// reconnected nor race for dcrwallet's single RpcSync slot.
		for services.SyncPaused() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}

		// Wait for wallet to be loaded (user must open via UI on a fresh
		// dashboard restart, OR auto-loaded if dcrwallet kept state).
		if !waitForWalletLoaded(ctx) {
			return
		}

		// During a restore, the dedicated account-discovery sync owns
		// dcrwallet's single RpcSync slot (it carries the private passphrase to
		// keep the wallet unlocked for discovery). Opening our passphrase-less
		// sync now would steal that slot, leaving accounts undiscovered and
		// corrupting per-account keys written during restore. Wait it out.
		for services.RestoreDiscoveryActive() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}

		if firstStart {
			log.Println("RPC sync resumed on startup")
			firstStart = false
		} else {
			log.Println("Reconnecting RPC sync to dcrd")
		}

		// Run on a per-attempt cancellable context so PauseSync can interrupt
		// the otherwise-blocking RpcSync stream the moment a switch begins.
		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		services.RegisterSyncCancel(cancelAttempt)
		err := services.EnsureRpcSync(attemptCtx)
		cancelAttempt()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			services.MarkSyncDisconnected(err.Error())
			log.Printf("RPC sync error (will retry in %v): %v", backoff, err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

func waitForWalletLoaded(ctx context.Context) bool {
	for {
		// Don't touch the gRPC clients while a switch is reconnecting them.
		if services.SyncPaused() {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(1 * time.Second):
			}
			continue
		}
		loadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		loaded, _ := services.CheckWalletLoaded(loadCtx)
		cancel()
		if loaded {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
}
