COMPOSE ?= $(shell if docker compose version >/dev/null 2>&1; then printf 'docker compose'; elif command -v docker-compose >/dev/null 2>&1; then printf 'docker-compose'; fi)
OPENAPI ?= packages/openapi/openapi.yaml
OPENAPI_BUNDLE ?= build/openapi/openapi.bundle.yaml
OPENAPI_GO_CONFIG ?= packages/openapi/oapi-codegen.server.yaml
OPENAPI_GO_OUTPUT ?= apps/api/internal/openapi/openapi.gen.go
OPENAPI_COPILOT_SPEC ?= apps/api/internal/modules/copilot/openapi.spec.yaml
OPENAPI_TS_OUTPUT ?= packages/sdk/typescript/src
OAPI_CODEGEN ?= go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0
OPENAPI_TS ?= npx --yes @hey-api/openapi-ts@0.97.2
TSC ?= npx --yes -p typescript@5.9.3 tsc
SECRETLINT ?= npx --yes -p secretlint@13.0.2 -p @secretlint/secretlint-rule-preset-recommend@13.0.2 secretlint
ENT ?= go run entgo.io/ent/cmd/ent@v0.14.6
ATLAS ?= npx --yes @ariga/atlas@1.2.0
EXAMPLES_CHECK ?= node tools/examples-check.mjs
OBSERVABILITY_RULES_CHECK ?= node tools/observability-rules-check.mjs
ADMIN_OPENAPI_COVERAGE_CHECK ?= node tools/admin-openapi-coverage-check.mjs
WEB_ADMIN_SDK_ROUTE_CHECK ?= node tools/web-admin-sdk-route-check.mjs
BOOTSTRAP_ENV ?= node tools/bootstrap-env.mjs
ENV_CHECK ?= node tools/env-check.mjs
DEPLOY_PREFLIGHT ?= node tools/deploy-preflight.mjs
WEB_CHECK ?= node tools/web-check.mjs
WEB_CHECK_E2E ?= node tools/web-check-e2e.mjs
WEB_DIR ?= apps/web
API_DIR ?= apps/api
MIGRATION_NAME ?=
RATE_LIMIT_BENCH_REDIS_ADDR ?=
RATE_LIMIT_BENCH_REDIS_PASSWORD ?=
RATE_LIMIT_BENCH_REDIS_DB ?= 15
RATE_LIMIT_BENCH_SAMPLES ?= 2000
RATE_LIMIT_BENCH_BUDGET_MS ?= 2
BALANCE_CHARGER_PRESSURE_DSN ?=
BALANCE_CHARGER_PRESSURE_TIMEOUT ?= 120s
OTEL_OVERHEAD_SAMPLES ?= 2000
OTEL_OVERHEAD_WARMUP ?= 200
OTEL_OVERHEAD_BUDGET_MS ?= 5
OTEL_OVERHEAD_TIMEOUT ?= 60s
JAEGER_IMAGE ?= jaegertracing/all-in-one:1.76.0
JAEGER_CONTAINER ?= srapi-jaeger-smoke
JAEGER_OTLP_PORT ?= 4317
JAEGER_QUERY_PORT ?= 16686
JAEGER_QUERY_TIMEOUT_SECONDS ?= 20
JAEGER_SMOKE_TIMEOUT ?= 90s
TEMPO_IMAGE ?= grafana/tempo:2.9.0
TEMPO_CONTAINER ?= srapi-tempo-smoke
TEMPO_CONFIG ?= $(CURDIR)/deploy/tempo-smoke.yaml
TEMPO_OTLP_PORT ?= 14318
TEMPO_QUERY_PORT ?= 13201
TEMPO_QUERY_TIMEOUT_SECONDS ?= 20
TEMPO_SMOKE_TIMEOUT ?= 90s

.PHONY: help bootstrap-env env-check deploy-preflight openapi-lint openapi-bundle openapi-codegen openapi-codegen-check openapi-admin-coverage-check openapi-ts-codegen openapi-ts-codegen-check sdk-ts-typecheck ent-generate ent-generate-check migration-diff migration-hash migration-check api-test api-run dev-up dev-down dev-logs smoke-health smoke-gateway smoke-rate-limit smoke-failover smoke-quality-eval smoke-payment-stripe smoke-payment-alipay smoke-payment-wechat smoke-release smoke-jaeger-trace smoke-tempo-trace rate-limit-bench balance-charger-pressure otel-overhead-bench backup-postgres restore-postgres examples-check observability-rules-check secret-scan architecture-check code-quality-check diff-check web-install web-install-ci web-dev web-admin-sdk-route-check web-check web-check-e2e check

help:
	@printf '%s\n' \
		'SRapi development targets:' \
		'  make bootstrap-env   Create .env from .env.example if missing' \
		'  make env-check       Verify .env has strong local secrets and private permissions' \
		'  make deploy-preflight  Verify local env, deploy files, alerts, and host tooling before Compose' \
		'  make openapi-lint    Validate OpenAPI contract with Redocly' \
		'  make openapi-bundle  Bundle OpenAPI contract into build/openapi/' \
		'  make openapi-codegen Generate Go OpenAPI types/server interfaces' \
		'  make openapi-codegen-check Check generated Go OpenAPI code is current' \
		'  make openapi-admin-coverage-check  Check registered admin routes are in OpenAPI' \
		'  make openapi-ts-codegen Generate TypeScript SDK from OpenAPI' \
		'  make openapi-ts-codegen-check Check generated TypeScript SDK is current' \
		'  make sdk-ts-typecheck Typecheck generated TypeScript SDK' \
		'  make ent-generate    Generate Ent client from schema' \
		'  make ent-generate-check Check Ent generated code is current' \
		'  make migration-diff MIGRATION_NAME=... Generate a PostgreSQL migration from Ent schema' \
		'  make migration-hash  Refresh Atlas migration integrity hash for PostgreSQL up migrations' \
		'  make migration-check Check versioned PostgreSQL migrations against Ent schema' \
		'  make api-test        Run Go API tests' \
		'  make api-run         Run the API locally with go run' \
		'  make dev-up          Start API, PostgreSQL, and Redis with Docker Compose' \
		'  make dev-down        Stop local Docker Compose services' \
		'  make smoke-health    Curl /api/v1/health on localhost' \
		'  make smoke-gateway   Login, create an API key, and smoke test local gateway endpoints' \
		'  make smoke-rate-limit  Verify Gateway API key RPM limiting returns 429 + Retry-After' \
		'  make smoke-failover  Verify Gateway retries from a 503 upstream to a fallback provider' \
		'  make smoke-quality-eval  Verify QualityEval capture, worker judge, and Scheduler quality evidence' \
		'  make smoke-payment-stripe  Verify Stripe test-mode checkout + webhook + balance credit on a running API' \
		'  make smoke-payment-alipay  Verify Alipay Page Pay checkout URL and optional local webhook on a running API' \
		'  make smoke-payment-wechat  Verify WeChat Pay prepay and optional local APIv3 webhook on a running API' \
		'  make smoke-release   Validate health, readiness, metrics, and gateway smoke on localhost' \
		'  make smoke-jaeger-trace  Verify OTLP traces are visible through Jaeger query API' \
		'  make smoke-tempo-trace  Verify OTLP traces are visible through Tempo query API' \
		'  make rate-limit-bench RATE_LIMIT_BENCH_REDIS_ADDR=host:port  Check Redis rate limiter p99 budget' \
		'  make balance-charger-pressure BALANCE_CHARGER_PRESSURE_DSN=postgres://...  Run PostgreSQL balance_charger pressure test' \
		'  make otel-overhead-bench  Check OTel HTTP tracing p99 overhead budget' \
		'  make backup-postgres BACKUP_FILE=...   Create a PostgreSQL custom-format backup' \
		'  make restore-postgres BACKUP_FILE=...  Restore a PostgreSQL custom-format backup' \
		'  make examples-check  Validate public examples and 2api migration guide' \
		'  make observability-rules-check  Validate SRapi Prometheus alert rule hygiene' \
		'  make architecture-check  Run architecture and startup harness tests' \
		'  make code-quality-check  Run repository code-quality harness tests' \
		'  make diff-check     Check staged and unstaged diff whitespace' \
		'  make web-admin-sdk-route-check  Check managed admin frontend routes use generated SDK' \
		'  make secret-scan     Scan source files for committed secrets' \
		'  make check           Run current contract and API checks'

bootstrap-env:
	$(BOOTSTRAP_ENV)

env-check:
	$(ENV_CHECK)

deploy-preflight:
	$(DEPLOY_PREFLIGHT)

openapi-lint:
	npx --yes @redocly/cli lint $(OPENAPI)

openapi-bundle:
	@mkdir -p $(dir $(OPENAPI_BUNDLE))
	npx --yes @redocly/cli bundle $(OPENAPI) --output $(OPENAPI_BUNDLE)

openapi-codegen:
	@mkdir -p $(dir $(OPENAPI_GO_OUTPUT))
	cd $(API_DIR) && $(OAPI_CODEGEN) -generate types,std-http -package openapi -o internal/openapi/openapi.gen.go ../../$(OPENAPI)
	cp $(OPENAPI) $(OPENAPI_COPILOT_SPEC)

openapi-codegen-check:
	@set -e; \
	tmp="$$(mktemp)"; \
	(cd $(API_DIR) && $(OAPI_CODEGEN) -generate types,std-http -package openapi -o "$$tmp" ../../$(OPENAPI)); \
	cmp -s "$$tmp" "$(OPENAPI_GO_OUTPUT)" || (echo "$(OPENAPI_GO_OUTPUT) is out of date; run make openapi-codegen" >&2; rm -f "$$tmp"; exit 1); \
	rm -f "$$tmp"
	@cmp -s "$(OPENAPI)" "$(OPENAPI_COPILOT_SPEC)" || (echo "$(OPENAPI_COPILOT_SPEC) is out of date; run make openapi-codegen" >&2; exit 1)

openapi-admin-coverage-check:
	$(ADMIN_OPENAPI_COVERAGE_CHECK)

openapi-ts-codegen:
	$(OPENAPI_TS) -i $(OPENAPI) -o $(OPENAPI_TS_OUTPUT) -c @hey-api/client-fetch -p @hey-api/typescript @hey-api/sdk --no-log-file

openapi-ts-codegen-check:
	@set -e; \
	tmp="$$(mktemp -d)"; \
	$(OPENAPI_TS) -i $(OPENAPI) -o "$$tmp/typescript" -c @hey-api/client-fetch -p @hey-api/typescript @hey-api/sdk --no-log-file; \
	diff -qr "$$tmp/typescript" "$(OPENAPI_TS_OUTPUT)" >/dev/null || (echo "$(OPENAPI_TS_OUTPUT) is out of date; run make openapi-ts-codegen" >&2; rm -rf "$$tmp"; exit 1); \
	rm -rf "$$tmp"

sdk-ts-typecheck:
	$(TSC) -p packages/sdk/typescript/tsconfig.json --noEmit

ent-generate:
	cd $(API_DIR) && $(ENT) generate ./ent/schema

ent-generate-check:
	@set -e; \
	tmp="$$(mktemp -d)"; \
	cp -a "$(API_DIR)/ent" "$$tmp/ent.before"; \
	(cd $(API_DIR) && $(ENT) generate ./ent/schema); \
	diff -qr "$$tmp/ent.before" "$(API_DIR)/ent" >/dev/null || (echo 'Ent generated code changed; run make ent-generate' >&2; rm -rf "$$tmp"; exit 1); \
	rm -rf "$$tmp"

migration-diff:
	@test -n "$(MIGRATION_NAME)" || (echo 'MIGRATION_NAME is required, for example: make migration-diff MIGRATION_NAME=000002_auth_sessions' >&2; exit 2)
	@name="$(MIGRATION_NAME)"; \
	printf '%s' "$$name" | grep -Eq '^[0-9]{6}_[a-z0-9_]+$$' || (echo 'MIGRATION_NAME must look like 000002_auth_sessions' >&2; exit 2); \
	number="$${name%%_*}"; \
	test "$$number" != "000001" || (echo '000001 is reserved for the initial schema; use 000002 or later' >&2; exit 2)
	@set -e; \
	before="$$(mktemp)"; \
	after="$$(mktemp)"; \
	find "$(API_DIR)/migrations/postgres/up" -maxdepth 1 -type f -name '*.sql' -printf '%f\n' | sort > "$$before"; \
	(cd $(API_DIR) && $(ATLAS) migrate diff "$(MIGRATION_NAME)" --env local); \
	find "$(API_DIR)/migrations/postgres/up" -maxdepth 1 -type f -name '*.sql' -printf '%f\n' | sort > "$$after"; \
	new="$$(comm -13 "$$before" "$$after" | sed '/^$$/d')"; \
	rm -f "$$before" "$$after"; \
	count="$$(printf '%s\n' "$$new" | sed '/^$$/d' | wc -l | tr -d ' ')"; \
	test "$$count" -le 1 || (echo "Atlas generated multiple migration files; review apps/api/migrations/postgres/up manually" >&2; exit 1); \
	if test "$$count" -eq 0; then \
		printf '%s\n' 'No migration generated; Ent schema already matches the up migration directory.'; \
		exit 0; \
	fi; \
	if test "$$new" != "$(MIGRATION_NAME).sql"; then \
		mv "$(API_DIR)/migrations/postgres/up/$$new" "$(API_DIR)/migrations/postgres/up/$(MIGRATION_NAME).sql"; \
	fi; \
	$(MAKE) migration-hash; \
	printf '%s\n' 'Review the generated up migration and add the matching apps/api/migrations/postgres/down/$(MIGRATION_NAME).sql before commit.'

migration-hash:
	cd $(API_DIR) && $(ATLAS) migrate hash --dir file://migrations/postgres/up

migration-check:
	cd $(API_DIR) && go test ./internal/platform/db -run 'Test(EntSchemaAppliesToEmptyDatabase|PostgresVersionedUpMigrationsMatchEntSchema|PostgresInitialDownMigrationCoversInitialTables|PostgresDownMigrationsCoverCreatedTables|PostgresIncrementalMigrationsArePairedAndContiguous)'

api-test:
	cd $(API_DIR) && packages="$$(go list ./... | grep -v '/internal/codequality$$')" && go test $$packages

api-run:
	cd $(API_DIR) && go run ./cmd/srapi

architecture-check:
	cd $(API_DIR) && go test ./internal/config ./internal/architecture ./internal/app ./internal/platform/crypto ./internal/platform/db ./internal/platform/logger ./internal/platform/redis ./internal/modules/providers/preset ./internal/persistence/entstore/... ./internal/persistence/redisstore/... ./internal/workers/... ./internal/httpserver

code-quality-check:
	cd $(API_DIR) && go test -tags=codequality ./internal/codequality -count=1

diff-check:
	git diff --check

dev-up: bootstrap-env
	@test -n "$(COMPOSE)" || (echo 'Docker Compose is required: install the docker compose plugin or docker-compose.' >&2; exit 127)
	$(COMPOSE) --env-file .env -f deploy/docker-compose.yml up --build

dev-down:
	@test -n "$(COMPOSE)" || (echo 'Docker Compose is required: install the docker compose plugin or docker-compose.' >&2; exit 127)
	$(COMPOSE) --env-file .env -f deploy/docker-compose.yml down

dev-logs:
	@test -n "$(COMPOSE)" || (echo 'Docker Compose is required: install the docker compose plugin or docker-compose.' >&2; exit 127)
	$(COMPOSE) --env-file .env -f deploy/docker-compose.yml logs -f

smoke-health:
	curl -fsS "http://localhost:$${SERVER_PORT:-8080}/api/v1/health"

smoke-gateway:
	node tools/smoke-local.mjs

smoke-rate-limit:
	node tools/smoke-local.mjs --rate-limit

smoke-failover:
	node tools/smoke-local.mjs --failover

smoke-quality-eval:
	cd $(API_DIR) && go test ./internal/httpserver -run TestQualityEvalSmokeCapturesEvaluatesAndFeedsScheduler -count=1 -v

smoke-payment-stripe:
	node tools/smoke-payment-stripe.mjs

smoke-payment-alipay:
	node tools/smoke-payment-alipay.mjs

smoke-payment-wechat:
	node tools/smoke-payment-wechat.mjs

smoke-release:
	node tools/smoke-local.mjs --release

smoke-jaeger-trace:
	@command -v docker >/dev/null 2>&1 || (echo 'docker is required for smoke-jaeger-trace' >&2; exit 127)
	@set -e; \
	cleanup() { docker rm -f "$(JAEGER_CONTAINER)" >/dev/null 2>&1 || true; }; \
	trap cleanup EXIT INT TERM; \
	docker rm -f "$(JAEGER_CONTAINER)" >/dev/null 2>&1 || true; \
	docker run -d --rm --name "$(JAEGER_CONTAINER)" \
		-e COLLECTOR_OTLP_ENABLED=true \
		-p "127.0.0.1:$(JAEGER_QUERY_PORT):16686" \
		-p "127.0.0.1:$(JAEGER_OTLP_PORT):4317" \
		"$(JAEGER_IMAGE)" >/dev/null; \
	ready=0; \
	for _ in $$(seq 1 30); do \
		if curl -fsS "http://127.0.0.1:$(JAEGER_QUERY_PORT)/api/services" >/dev/null 2>&1; then ready=1; break; fi; \
		sleep 1; \
	done; \
	test "$$ready" = "1" || (docker logs "$(JAEGER_CONTAINER)" >&2; echo 'Jaeger query API did not become ready' >&2; exit 1); \
	cd $(API_DIR) && \
		SRAPI_OTEL_JAEGER_SMOKE=1 \
		SRAPI_OTEL_JAEGER_OTLP_ENDPOINT="127.0.0.1:$(JAEGER_OTLP_PORT)" \
		SRAPI_OTEL_JAEGER_QUERY_URL="http://127.0.0.1:$(JAEGER_QUERY_PORT)" \
		SRAPI_OTEL_JAEGER_QUERY_TIMEOUT_SECONDS="$(JAEGER_QUERY_TIMEOUT_SECONDS)" \
		go test ./internal/platform/otel -run TestNewTracerProviderExportsSpansToJaegerQuery -count=1 -timeout "$(JAEGER_SMOKE_TIMEOUT)" -v

smoke-tempo-trace:
	@command -v docker >/dev/null 2>&1 || (echo 'docker is required for smoke-tempo-trace' >&2; exit 127)
	@test -f "$(TEMPO_CONFIG)" || (echo 'TEMPO_CONFIG does not exist: $(TEMPO_CONFIG)' >&2; exit 2)
	@set -e; \
	cleanup() { docker rm -f "$(TEMPO_CONTAINER)" >/dev/null 2>&1 || true; }; \
	trap cleanup EXIT INT TERM; \
	docker rm -f "$(TEMPO_CONTAINER)" >/dev/null 2>&1 || true; \
	docker run -d --rm --name "$(TEMPO_CONTAINER)" \
		-v "$(TEMPO_CONFIG):/etc/tempo.yaml:ro" \
		-p "127.0.0.1:$(TEMPO_QUERY_PORT):3200" \
		-p "127.0.0.1:$(TEMPO_OTLP_PORT):4317" \
		"$(TEMPO_IMAGE)" -config.file=/etc/tempo.yaml >/dev/null; \
	ready=0; \
	for _ in $$(seq 1 30); do \
		if curl -fsS "http://127.0.0.1:$(TEMPO_QUERY_PORT)/ready" >/dev/null 2>&1; then ready=1; break; fi; \
		sleep 1; \
	done; \
	test "$$ready" = "1" || (docker logs "$(TEMPO_CONTAINER)" >&2; echo 'Tempo query API did not become ready' >&2; exit 1); \
	cd $(API_DIR) && \
		SRAPI_OTEL_TEMPO_SMOKE=1 \
		SRAPI_OTEL_TEMPO_OTLP_ENDPOINT="127.0.0.1:$(TEMPO_OTLP_PORT)" \
		SRAPI_OTEL_TEMPO_QUERY_URL="http://127.0.0.1:$(TEMPO_QUERY_PORT)" \
		SRAPI_OTEL_TEMPO_QUERY_TIMEOUT_SECONDS="$(TEMPO_QUERY_TIMEOUT_SECONDS)" \
		go test ./internal/platform/otel -run TestNewTracerProviderExportsSpansToTempoQuery -count=1 -timeout "$(TEMPO_SMOKE_TIMEOUT)" -v

rate-limit-bench:
	@test -n "$(RATE_LIMIT_BENCH_REDIS_ADDR)" || (echo 'RATE_LIMIT_BENCH_REDIS_ADDR is required, for example: make rate-limit-bench RATE_LIMIT_BENCH_REDIS_ADDR=127.0.0.1:6379' >&2; exit 2)
	cd $(API_DIR) && \
		SRAPI_RATE_LIMIT_P99_GUARD=1 \
		SRAPI_RATE_LIMIT_P99_REDIS_ADDR="$(RATE_LIMIT_BENCH_REDIS_ADDR)" \
		SRAPI_RATE_LIMIT_P99_REDIS_PASSWORD="$(RATE_LIMIT_BENCH_REDIS_PASSWORD)" \
		SRAPI_RATE_LIMIT_P99_REDIS_DB="$(RATE_LIMIT_BENCH_REDIS_DB)" \
		SRAPI_RATE_LIMIT_P99_SAMPLES="$(RATE_LIMIT_BENCH_SAMPLES)" \
		SRAPI_RATE_LIMIT_P99_BUDGET_MS="$(RATE_LIMIT_BENCH_BUDGET_MS)" \
		go test ./internal/platform/ratelimit -run TestLimiterP99Budget -count=1 -v

balance-charger-pressure:
	@test -n "$(BALANCE_CHARGER_PRESSURE_DSN)" || (echo 'BALANCE_CHARGER_PRESSURE_DSN is required, for example: make balance-charger-pressure BALANCE_CHARGER_PRESSURE_DSN=postgres://user:pass@127.0.0.1:5432/srapi?sslmode=disable' >&2; exit 2)
	cd $(API_DIR) && \
		SRAPI_BALANCE_CHARGER_PRESSURE_DSN="$(BALANCE_CHARGER_PRESSURE_DSN)" \
		go test ./internal/workers/balance_charger -run TestBalanceChargerPostgresPressureDrainsDefaultBacklog -count=1 -timeout "$(BALANCE_CHARGER_PRESSURE_TIMEOUT)" -v

otel-overhead-bench:
	cd $(API_DIR) && \
		SRAPI_OTEL_P99_GUARD=1 \
		SRAPI_OTEL_P99_SAMPLES="$(OTEL_OVERHEAD_SAMPLES)" \
		SRAPI_OTEL_P99_WARMUP="$(OTEL_OVERHEAD_WARMUP)" \
		SRAPI_OTEL_P99_BUDGET_MS="$(OTEL_OVERHEAD_BUDGET_MS)" \
		go test ./internal/httpserver -run TestTracingMiddlewareP99OverheadBudget -count=1 -timeout "$(OTEL_OVERHEAD_TIMEOUT)" -v

backup-postgres:
	@test -n "$(BACKUP_FILE)" || (echo 'BACKUP_FILE is required' >&2; exit 2)
	@mkdir -p "$$(dirname "$(BACKUP_FILE)")"
	PGPASSWORD="$${DATABASE_PASSWORD:-srapi_dev_password_change_me}" pg_dump \
		--host "$${DATABASE_HOST:-localhost}" \
		--port "$${DATABASE_PORT:-5432}" \
		--username "$${DATABASE_USER:-srapi}" \
		--dbname "$${DATABASE_DBNAME:-srapi}" \
		--format custom \
		--file "$(BACKUP_FILE)"
	sha256sum "$(BACKUP_FILE)" > "$(BACKUP_FILE).sha256"

restore-postgres:
	@test -n "$(BACKUP_FILE)" || (echo 'BACKUP_FILE is required' >&2; exit 2)
	@test -f "$(BACKUP_FILE)" || (echo 'BACKUP_FILE does not exist: $(BACKUP_FILE)' >&2; exit 2)
	@if test -f "$(BACKUP_FILE).sha256"; then sha256sum -c "$(BACKUP_FILE).sha256"; else echo 'No checksum file found; skipping checksum verification.'; fi
	PGPASSWORD="$${DATABASE_PASSWORD:-srapi_dev_password_change_me}" pg_restore \
		--host "$${DATABASE_HOST:-localhost}" \
		--port "$${DATABASE_PORT:-5432}" \
		--username "$${DATABASE_USER:-srapi}" \
		--dbname "$${DATABASE_DBNAME:-srapi}" \
		--clean \
		--if-exists \
		"$(BACKUP_FILE)"

secret-scan:
	$(SECRETLINT) "**/*"

examples-check:
	$(EXAMPLES_CHECK)

observability-rules-check:
	$(OBSERVABILITY_RULES_CHECK)

web-install:
	npm --prefix $(WEB_DIR) install

web-install-ci:
	npm --prefix $(WEB_DIR) ci

web-dev:
	npm --prefix $(WEB_DIR) run dev

web-admin-sdk-route-check:
	$(WEB_ADMIN_SDK_ROUTE_CHECK)

web-check: web-admin-sdk-route-check
	$(WEB_CHECK)

web-check-e2e:
	$(WEB_CHECK_E2E)

check: diff-check openapi-lint openapi-bundle openapi-codegen-check openapi-admin-coverage-check openapi-ts-codegen-check sdk-ts-typecheck ent-generate-check migration-check architecture-check code-quality-check examples-check observability-rules-check api-test secret-scan web-check
