.PHONY: db migrate migrate-one demo-load run test snapshot

SHELL_APP_PORT := $(APP_PORT)
SHELL_DATABASE_URL := $(DATABASE_URL)
SHELL_CORS_ALLOWED_ORIGINS := $(CORS_ALLOWED_ORIGINS)
SHELL_COLLECT_INTERVAL := $(COLLECT_INTERVAL)
SHELL_RISK_FREE_RATE := $(RISK_FREE_RATE)
SHELL_TSETMC_BASE_URL := $(TSETMC_BASE_URL)
SHELL_TSETMC_USER_AGENT := $(TSETMC_USER_AGENT)
SHELL_TSETMC_TIMEOUT := $(TSETMC_TIMEOUT)
SHELL_TSETMC_MAX_ATTEMPTS := $(TSETMC_MAX_ATTEMPTS)
SHELL_TSETMC_RATE_LIMIT := $(TSETMC_RATE_LIMIT)
SHELL_SNAPSHOT_TIME := $(SNAPSHOT_TIME)
SHELL_SNAPSHOT_TRIES := $(SNAPSHOT_TRIES)
SHELL_SNAPSHOT_RETRY_DELAY := $(SNAPSHOT_RETRY_DELAY)

-include .env

DATABASE_URL ?= postgres://Finova:Finova@localhost:5432/Finova?sslmode=disable
ifneq ($(SHELL_DATABASE_URL),)
DATABASE_URL := $(SHELL_DATABASE_URL)
endif
ifneq ($(SHELL_APP_PORT),)
APP_PORT := $(SHELL_APP_PORT)
endif
ifneq ($(SHELL_CORS_ALLOWED_ORIGINS),)
CORS_ALLOWED_ORIGINS := $(SHELL_CORS_ALLOWED_ORIGINS)
endif
ifneq ($(SHELL_COLLECT_INTERVAL),)
COLLECT_INTERVAL := $(SHELL_COLLECT_INTERVAL)
endif
ifneq ($(SHELL_RISK_FREE_RATE),)
RISK_FREE_RATE := $(SHELL_RISK_FREE_RATE)
endif
ifneq ($(SHELL_TSETMC_BASE_URL),)
TSETMC_BASE_URL := $(SHELL_TSETMC_BASE_URL)
endif
ifneq ($(SHELL_TSETMC_USER_AGENT),)
TSETMC_USER_AGENT := $(SHELL_TSETMC_USER_AGENT)
endif
ifneq ($(SHELL_TSETMC_TIMEOUT),)
TSETMC_TIMEOUT := $(SHELL_TSETMC_TIMEOUT)
endif
ifneq ($(SHELL_TSETMC_MAX_ATTEMPTS),)
TSETMC_MAX_ATTEMPTS := $(SHELL_TSETMC_MAX_ATTEMPTS)
endif
ifneq ($(SHELL_TSETMC_RATE_LIMIT),)
TSETMC_RATE_LIMIT := $(SHELL_TSETMC_RATE_LIMIT)
endif
ifneq ($(SHELL_SNAPSHOT_TIME),)
SNAPSHOT_TIME := $(SHELL_SNAPSHOT_TIME)
endif
ifneq ($(SHELL_SNAPSHOT_TRIES),)
SNAPSHOT_TRIES := $(SHELL_SNAPSHOT_TRIES)
endif
ifneq ($(SHELL_SNAPSHOT_RETRY_DELAY),)
SNAPSHOT_RETRY_DELAY := $(SHELL_SNAPSHOT_RETRY_DELAY)
endif

export APP_PORT DATABASE_URL CORS_ALLOWED_ORIGINS COLLECT_INTERVAL RISK_FREE_RATE
export TSETMC_BASE_URL TSETMC_USER_AGENT TSETMC_TIMEOUT TSETMC_MAX_ATTEMPTS TSETMC_RATE_LIMIT
export SNAPSHOT_TIME SNAPSHOT_TRIES SNAPSHOT_RETRY_DELAY

db:
	docker compose -f ../docker-compose.yml up -d db

migrate:
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "$$files=Get-ChildItem 'migrations' -Filter '*.up.sql' | Sort-Object Name; if ([string]::IsNullOrWhiteSpace('$(DATABASE_URL)')) { Write-Error 'DATABASE_URL is required. Copy .env.example to .env or set DATABASE_URL in your shell.'; exit 2 }; foreach ($$file in $$files) { Write-Host \"Applying $$($$file.Name)\"; psql '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -f $$file.FullName; if ($$LASTEXITCODE -ne 0) { Write-Error 'Migration failed. For a database with earlier migrations, use migrate-one FILE=migrations/<migration>.up.sql.'; exit 1 } }"
else
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "DATABASE_URL is required. Copy .env.example to .env or set DATABASE_URL in your shell." >&2; \
		exit 2; \
	fi
	@set -e; \
	for file in migrations/*.up.sql; do \
		echo "Applying $$file"; \
		psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$file" || { \
			echo "Migration failed." >&2; \
			echo "For migrations on a database that already has the market tables, use 'make migrate-one FILE=migrations/<migration>.up.sql'." >&2; \
			echo "Apply migrations only once; rerunning migration 000001 on an existing schema may report that tables already exist." >&2; \
			exit 1; \
		}; \
	done
endif

migrate-one:
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "if ('$(FILE)' -eq '') { Write-Error 'FILE is required, for example: make migrate-one FILE=migrations/000012_backtest_run_details.up.sql'; exit 2 }; if ([string]::IsNullOrWhiteSpace('$(DATABASE_URL)')) { Write-Error 'DATABASE_URL is required. Copy .env.example to .env or set DATABASE_URL in your shell.'; exit 2 }; psql '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -f '$(FILE)'"
else
	@if [ -z "$(FILE)" ]; then \
		echo "FILE is required, for example: make migrate-one FILE=migrations/000012_backtest_run_details.up.sql" >&2; \
		exit 2; \
	fi
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "DATABASE_URL is required. Copy .env.example to .env or set DATABASE_URL in your shell." >&2; \
		exit 2; \
	fi
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$(FILE)"
endif

demo-load:
	go run ./cmd/backfill import testdata/demo
	go run ./cmd/metrics recompute

run:
	go run ./cmd/api

test:
	go test ./...

snapshot:
	go run ./cmd/snapshot daemon