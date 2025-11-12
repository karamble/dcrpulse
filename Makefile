.PHONY: help start stop restart logs logs-dcrd logs-dashboard build clean status shell-dcrd shell-dashboard backup backup-wallet backup-certs restore restore-wallet

# Force bash shell for bash-specific syntax (needed for clean target)
SHELL := /bin/bash

# Load environment variables from .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Default RPC credentials (override with env vars)
DCRD_RPC_USER ?= decred
DCRD_RPC_PASS ?= decredpass

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

start: ## Start all services (dcrd + dcrwallet + dashboard)
	@echo "Starting dcrpulse stack..."
	docker compose up -d
	@echo "Services started! Dashboard will be available at http://localhost:8080"
	@echo "Note: dcrd needs to sync the blockchain on first run (may take several hours)"

stop: ## Stop all services
	@echo "Stopping all services..."
	docker compose down

restart: ## Restart all services
	@echo "Restarting all services..."
	docker compose restart

logs: ## View logs from all services
	docker compose logs -f

logs-dcrd: ## View dcrd logs
	docker compose logs -f dcrd

logs-dashboard: ## View dashboard logs
	docker compose logs -f dashboard

build: ## Build/rebuild all images
	@echo "Building images..."
	docker compose build --no-cache

status: ## Show status of all services
	@echo "=== Service Status ==="
	docker compose ps
	@echo ""
	@echo "=== dcrd Info ==="
	@docker exec dcrpulse-dcrd dcrctl \
		--rpcuser=$(DCRD_RPC_USER) \
		--rpcpass=$(DCRD_RPC_PASS) \
		--rpcserver=127.0.0.1:9109 \
		--rpccert=/app-data/certs/rpc.cert \
		getinfo 2>/dev/null || echo "dcrd not ready yet..."

clean: ## Stop and remove all containers, networks, and volumes (WARNING: deletes ALL data!)
	@echo "WARNING: This will delete ALL blockchain and wallet data!"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose down -v; \
		echo "Cleaned up!"; \
	fi

shell-dcrd: ## Open shell in dcrd container
	docker exec -it dcrpulse-dcrd /bin/sh

shell-dashboard: ## Open shell in dashboard container
	docker exec -it dcrpulse-dashboard /bin/sh

dcrctl: ## Run dcrctl command for dcrd (usage: make dcrctl CMD="getblockcount")
	@docker exec dcrpulse-dcrd dcrctl \
		--rpcuser=$(DCRD_RPC_USER) \
		--rpcpass=$(DCRD_RPC_PASS) \
		--rpcserver=127.0.0.1:9109 \
		--rpccert=/app-data/certs/rpc.cert \
		$(CMD)

dcrctl-wallet: ## Run dcrctl wallet command (usage: make dcrctl-wallet CMD="getbalance")
	@docker exec dcrpulse-dcrwallet dcrctl \
		--wallet \
		--rpcuser=$(DCRWALLET_RPC_USER) \
		--rpcpass=$(DCRWALLET_RPC_PASS) \
		--rpcserver=127.0.0.1:9110 \
		--rpccert=/app-data/certs/rpc.cert \
		$(CMD)

sync-status: ## Check blockchain sync status
	@echo "=== Blockchain Sync Status ==="
	@docker exec dcrpulse-dcrd dcrctl \
		--rpcuser=$(DCRD_RPC_USER) \
		--rpcpass=$(DCRD_RPC_PASS) \
		--rpcserver=127.0.0.1:9109 \
		--rpccert=/app-data/certs/rpc.cert \
		getblockchaininfo 2>/dev/null || echo "dcrd not ready yet..."

peers: ## Show connected peers
	@echo "=== Connected Peers ==="
	@docker exec dcrpulse-dcrd dcrctl \
		--rpcuser=$(DCRD_RPC_USER) \
		--rpcpass=$(DCRD_RPC_PASS) \
		--rpcserver=127.0.0.1:9109 \
		--rpccert=/app-data/certs/rpc.cert \
		getpeerinfo 2>/dev/null | grep -E '"addr"|"id"' || echo "dcrd not ready yet..."

backup: ## Backup all app data
	@echo "Creating backup of all app data..."
	@mkdir -p backups
	docker run --rm \
		-v dcrpulse_app-data:/app-data \
		-v $(PWD)/backups:/backup \
		alpine tar czf /backup/app-data-backup-$$(date +%Y%m%d-%H%M%S).tar.gz -C /app-data .
	@echo "Backup created in backups/"

backup-wallet: ## Backup wallet data only
	@echo "Creating backup of wallet data..."
	@mkdir -p backups
	docker run --rm \
		-v dcrpulse_app-data:/app-data \
		-v $(PWD)/backups:/backup \
		alpine tar czf /backup/wallet-backup-$$(date +%Y%m%d-%H%M%S).tar.gz -C /app-data/dcrwallet .
	@echo "Wallet backup created in backups/"

backup-certs: ## Backup certificates only
	@echo "Creating backup of certificates..."
	@mkdir -p backups
	docker run --rm \
		-v dcrpulse_app-data:/app-data \
		-v $(PWD)/backups:/backup \
		alpine tar czf /backup/certs-backup-$$(date +%Y%m%d-%H%M%S).tar.gz -C /app-data/certs .
	@echo "Certificates backup created in backups/"

restore: ## Restore all app data from backup (usage: make restore BACKUP=backups/app-data-backup-xxx.tar.gz)
	@if [ -z "$(BACKUP)" ]; then \
		echo "Usage: make restore BACKUP=backups/app-data-backup-xxx.tar.gz"; \
		exit 1; \
	fi
	@echo "Restoring from $(BACKUP)..."
	@echo "WARNING: This will overwrite ALL app data!"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose down; \
		docker run --rm \
			-v dcrpulse_app-data:/app-data \
			-v $(PWD):/backup \
			alpine sh -c "rm -rf /app-data/* && tar xzf /backup/$(BACKUP) -C /app-data"; \
		docker compose up -d; \
		echo "Restored!"; \
	fi

restore-wallet: ## Restore wallet data from backup (usage: make restore-wallet BACKUP=backups/wallet-backup-xxx.tar.gz)
	@if [ -z "$(BACKUP)" ]; then \
		echo "Usage: make restore-wallet BACKUP=backups/wallet-backup-xxx.tar.gz"; \
		exit 1; \
	fi
	@echo "Restoring wallet from $(BACKUP)..."
	@echo "WARNING: This will overwrite existing wallet data!"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker compose stop dcrwallet; \
		docker run --rm \
			-v dcrpulse_app-data:/app-data \
			-v $(PWD):/backup \
			alpine sh -c "rm -rf /app-data/dcrwallet/* && tar xzf /backup/$(BACKUP) -C /app-data/dcrwallet"; \
		docker compose up -d dcrwallet; \
		echo "Wallet restored!"; \
	fi

setup: ## Initial setup (copy env.example to .env)
	@if [ ! -f .env ]; then \
		cp env.example .env; \
		echo "Created .env file. Please edit it with your RPC credentials!"; \
		echo "Then run: make start"; \
	else \
		echo ".env file already exists"; \
	fi

update: ## Update all services to latest
	@echo "Rebuilding all services..."
	docker compose build --no-cache
	@echo "Updated! Run 'make restart' to apply changes"

update-dcrd: ## Update dcrd to latest from source
	@echo "Rebuilding dcrd from latest source..."
	docker compose build --no-cache dcrd
	docker compose up -d dcrd
	@echo "dcrd updated!"

build-dcrd: ## Build dcrd (usage: make build-dcrd VERSION=release-v2.0.6)
	@if [ -n "$(VERSION)" ]; then \
		echo "Building dcrd $(VERSION)..."; \
		DCRD_VERSION=$(VERSION) docker compose build --no-cache dcrd; \
	else \
		echo "Building dcrd from master..."; \
		docker compose build dcrd; \
	fi

dev-backend: ## Run backend in development mode (outside Docker)
	cd dashboard && go run cmd/dcrpulse/main.go

dev-frontend: ## Run frontend in development mode (outside Docker)
	cd dashboard/web && npm run dev

install-frontend: ## Install frontend dependencies
	cd dashboard/web && npm install

install-backend: ## Install backend dependencies
	cd dashboard && go mod download

# Wallet-specific commands

logs-dcrwallet: ## View dcrwallet logs
	docker compose logs -f dcrwallet

shell-dcrwallet: ## Open shell in dcrwallet container
	docker exec -it dcrpulse-dcrwallet /bin/sh

wallet-info: ## Get wallet info
	@docker exec dcrpulse-dcrwallet dcrctl \
		--wallet \
		--rpcuser=$(DCRWALLET_RPC_USER:-dcrwallet) \
		--rpcpass=$(DCRWALLET_RPC_PASS:-dcrwalletpass) \
		--rpcserver=127.0.0.1:9110 \
		--rpccert=/app-data/certs/rpc.cert \
		walletinfo 2>/dev/null || echo "dcrwallet not ready yet..."

wallet-balance: ## Get wallet balance
	@docker exec dcrpulse-dcrwallet dcrctl \
		--wallet \
		--rpcuser=$(DCRWALLET_RPC_USER:-dcrwallet) \
		--rpcpass=$(DCRWALLET_RPC_PASS:-dcrwalletpass) \
		--rpcserver=127.0.0.1:9110 \
		--rpccert=/app-data/certs/rpc.cert \
		getbalance 2>/dev/null || echo "dcrwallet not ready yet..."

wallet-seed: ## View wallet seed from logs (first-time setup only)
	@echo "============================================"
	@echo "WALLET SEED (if found in logs)"
	@echo "============================================"
	@docker logs dcrpulse-dcrwallet 2>&1 | sed -n '/Your wallet generation seed is:/,/Hex:/p' || echo "Seed not found in logs (wallet may have been created before logging was enabled)"
	@echo ""
	@echo "⚠️  IMPORTANT: Store this seed in a safe place!"
	@echo "============================================"

