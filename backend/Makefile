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
.PHONY: help tidy fmt vet lint build run dev test test-short test-integration \
        swagger swagger-check clean tools \
        db-volume db-up db-down db-logs db-connect db-wait db-migrate

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

run: ## Run server (go run)
	$(GO) run $(GOFLAGS) $(CMD_DIR)

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

