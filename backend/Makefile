SHELL := /bin/bash

# ---- Config ----
APP_NAME        := globalconq-service
CMD_DIR         := ./cmd/backend
MAIN_GO         := $(CMD_DIR)/main.go
BIN_DIR         := ./bin
BIN             := $(BIN_DIR)/$(APP_NAME)

SWAG            := swag
SWAG_OUT        := ./docs
SWAG_DIRS       := ./cmd,./internal

GO              := go
GOFLAGS         :=
TESTFLAGS       := -race -count=1

# DB
DB_VOLUME            ?= db
DB_HOST              ?= localhost
DB_PORT              ?= 5432
DB_USER              ?= globalconq
DB_NAME              ?= globalconq
DB_PASSWORD          ?= globalconq
DB_SCHEMA            ?= public
DB_MIXED             ?= false

COMPOSE              ?= docker compose
COMPOSE_FILE         ?= ./docker-compose.yml

SEED_FILE            ?= ./seed.sql

# tdtraindata
TD_GAMES             ?= 200
TD_OUTPUT_DIR         := data/raw/tdtraindata

# Flyway
MIGRATIONS_DIR ?= ./migrations
FLYWAY_IMAGE   ?= flyway/flyway:9

FLYWAY_CMD = docker run --rm \
	--network host \
	-v $(PWD)/$(MIGRATIONS_DIR):/flyway/sql \
	$(FLYWAY_IMAGE) \
	-user=$(DB_USER) \
	-password=$(DB_PASSWORD) \
	-url=jdbc:postgresql://$(DB_HOST):$(DB_PORT)/$(DB_NAME) \
	-schemas=$(DB_SCHEMA) \
	-connectRetries=30

# Default target
.DEFAULT_GOAL := help

# ---- Phony targets ----
.PHONY: help tidy fmt vet lint build run dev test test-short test-integration e2e seed-go \
        swagger swagger-check clean tools \
        db-volume db-up db-down db-logs db-connect db-wait db-migrate \
        build-discord run-discord tdtraindata-diverse

help: ## Show targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Install local dev tools (swag)
	@command -v $(SWAG) >/dev/null 2>&1 || { \
		echo "Installing swag..."; \
		$(GO) install github.com/swaggo/swag/cmd/swag@latest; \
	}

build: ## Build binary to ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN) $(CMD_DIR)

build-discord: ## Build Discord bot binary to ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/global-conquest-discord ./cmd/discord-bot

run: ## Run server (go run)
	$(GO) run $(GOFLAGS) $(CMD_DIR)

run-discord: ## Run Discord bot (go run)
	$(GO) run $(GOFLAGS) ./cmd/discord-bot

dev: swagger ## Update swagger then run server
	$(GO) run $(GOFLAGS) $(CMD_DIR)

swagger: tools ## Regenerate swagger docs (swaggo)
	$(SWAG) init -g $(MAIN_GO)

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

# ----------------------------
# Postgres (Docker Compose)
# ----------------------------

db-volume:
	@echo "==> Creating volume: $(DB_VOLUME)"
	docker volume create $(DB_VOLUME)

db-up:
	@echo "==> Starting only 'db' service (detached)"
	$(COMPOSE) -f $(COMPOSE_FILE) up -d postgres

db-down:
	@echo "==> Stopping DB stack"
	$(COMPOSE) -f $(COMPOSE_FILE) down

db-logs:
	@echo "==> Tail logs for 'db'"
	$(COMPOSE) -f $(COMPOSE_FILE) logs -f db

db-create:
	@echo "==> Ensuring database $(DB_NAME) exists on $(DB_HOST):$(DB_PORT)"
    	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d postgres \
    		-c "SELECT 'CREATE DATABASE $(DB_NAME)' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$(DB_NAME)')\gexec"

db-connect:
	@echo "==> Connecting to postgres ($(DB_HOST):$(DB_PORT)) db=$(DB_NAME) user=$(DB_USER)"
	@echo "(Note: password is '$(DB_PASSWORD)')"
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME)

# Wait until Postgres is ready (uses Flyway's connectRetries)
db-wait:
	@echo "==> Waiting for Postgres at $(DB_HOST):$(DB_PORT)/$(DB_NAME)"
	@$(FLYWAY_CMD) info >/dev/null

	# Apply migrations (V*.sql)
db-migrate: db-wait
	@echo "==> Running Flyway migrate from $(MIGRATIONS_DIR) (mixed=$(DB_MIXED))"
	@$(FLYWAY_CMD) migrate -mixed=$(DB_MIXED)

db-seed:
	@echo "==> Running SQL file $(SEED_FILE) against $(DB_NAME)"
	PGPASSWORD=$(DB_PASSWORD) psql \
		-h $(DB_HOST) -p $(DB_PORT) \
		-U $(DB_USER) -d $(DB_NAME) \
		-f "$(SEED_FILE)"

e2e: ## Run DB-backed end-to-end tests (requires E2E_ADMIN_DSN)
	@if [ -z "$(E2E_ADMIN_DSN)" ]; then \
		echo "E2E_ADMIN_DSN is required, e.g. postgres://globalconq:globalconq@localhost:5432/postgres?sslmode=disable"; \
		exit 1; \
	fi
	$(GO) test -tags=e2e ./internal/e2e -count=1 -v

seed-go: ## Seed DB with test users/games (hashes password='password' via auth module)
	$(GO) run ./cmd/seed

tdtraindata-diverse: ## Generate a TD(lambda) training set using the paper's exact 6-Lux-persona opponent pool (override TD_GAMES to change games-per-lineup, default 200)
	@mkdir -p $(TD_OUTPUT_DIR)
	@echo "==> Generating Lux-6 tdtraindata set ($(TD_GAMES) games per lineup) into $(TD_OUTPUT_DIR)"
	# Restricted to exactly the 6 personas Carr (2020) Section 3.4.3 used to
	# generate the paper's own training data -- "the Angry, Pixie, Cluster,
	# Quo, Killbot and Boscoe AIs inbuilt with Lux... chosen due to their
	# varying skill and playstyles". Previously this target also mixed in
	# basic-v1/scored-v1/turtle-v1 (none of which are Lux personas) across
	# 11 of its 12 lineups, diluting that curated pool across most of the
	# data. Player count is fixed per lineup (= len(--strategies)), and
	# encode.go's enemySeatCount=5 owner-slot/fraction features only get
	# populated up to numPlayers-1 enemies -- so seat coverage across the
	# whole set depends on how many lineups exist at each player count.
	# Kept at 3 lineups per count for 3/4/5 players, each rotating which of
	# the 6 personas gets excluded so every persona gets balanced exposure
	# (impossible to make perfectly even since 9/12/15 slots don't divide
	# evenly by 6, but no persona is excluded from more than one tier's
	# rotation). 6 players only has one possible composition (all 6, and
	# also the paper's own game size -- Section 3.2, "All games will
	# feature 6 players") -- run across 3 non-overlapping seed ranges
	# instead of 3 distinct lineups, to match the other tiers' total volume.
	#
	# -- 3 players (seats 1-2 populated) --
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,cluster-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux3_a_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies killbot-v1,quo-v1,boscoe-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux3_b_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,killbot-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux3_c_train.jsonl
	# -- 4 players (seats 1-3 populated) --
	$(GO) run ./cmd/tdtraindata --strategies cluster-v1,quo-v1,killbot-v1,boscoe-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux4_a_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,killbot-v1,boscoe-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux4_b_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,cluster-v1,quo-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux4_c_train.jsonl
	# -- 5 players (seats 1-4 populated) --
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,cluster-v1,quo-v1,boscoe-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux5_a_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,cluster-v1,quo-v1,killbot-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux5_b_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,pixie-v1,quo-v1,killbot-v1,boscoe-v1 \
		--games $(TD_GAMES) --output $(TD_OUTPUT_DIR)/lux5_c_train.jsonl
	# -- 6 players (seats 1-5 populated) -- the paper's exact opponent pool
	# and game size, run 3x across disjoint seed ranges for volume parity
	# with the other tiers.
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,cluster-v1,pixie-v1,quo-v1,boscoe-v1,killbot-v1 \
		--games $(TD_GAMES) --seed-start 1 --output $(TD_OUTPUT_DIR)/lux6_a_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,cluster-v1,pixie-v1,quo-v1,boscoe-v1,killbot-v1 \
		--games $(TD_GAMES) --seed-start $$(($(TD_GAMES)+1)) --output $(TD_OUTPUT_DIR)/lux6_b_train.jsonl
	$(GO) run ./cmd/tdtraindata --strategies angry-v1,cluster-v1,pixie-v1,quo-v1,boscoe-v1,killbot-v1 \
		--games $(TD_GAMES) --seed-start $$((2*$(TD_GAMES)+1)) --output $(TD_OUTPUT_DIR)/lux6_c_train.jsonl
	@echo "==> Done. Fit against the whole set from analytics/ with: poetry run fit-gcn --output /tmp/gcn.json"
