COMPOSE ?= $(shell if docker compose version >/dev/null 2>&1; then printf 'docker compose'; elif command -v docker-compose >/dev/null 2>&1; then printf 'docker-compose'; fi)
OPENAPI ?= packages/openapi/openapi.yaml
OPENAPI_BUNDLE ?= build/openapi/openapi.bundle.yaml
OPENAPI_GO_CONFIG ?= packages/openapi/oapi-codegen.server.yaml
OPENAPI_GO_OUTPUT ?= apps/api/internal/openapi/openapi.gen.go
OPENAPI_TS_OUTPUT ?= packages/sdk/typescript/src
OAPI_CODEGEN ?= go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.0
OPENAPI_TS ?= npx --yes @hey-api/openapi-ts@0.97.2
TSC ?= npx --yes -p typescript@5.9.3 tsc
SECRETLINT ?= npx --yes -p secretlint@13.0.2 -p @secretlint/secretlint-rule-preset-recommend@13.0.2 secretlint
ENT ?= go run entgo.io/ent/cmd/ent@v0.14.6
EXAMPLES_CHECK ?= node tools/examples-check.mjs
API_DIR ?= apps/api

.PHONY: help bootstrap-env openapi-lint openapi-bundle openapi-codegen openapi-codegen-check openapi-ts-codegen openapi-ts-codegen-check sdk-ts-typecheck ent-generate ent-generate-check migration-check api-test api-run dev-up dev-down dev-logs smoke-health smoke-gateway smoke-release backup-postgres restore-postgres examples-check secret-scan architecture-check code-quality-check diff-check check

help:
	@printf '%s\n' \
		'SRapi development targets:' \
		'  make bootstrap-env   Create .env from .env.example if missing' \
		'  make openapi-lint    Validate OpenAPI contract with Redocly' \
		'  make openapi-bundle  Bundle OpenAPI contract into build/openapi/' \
		'  make openapi-codegen Generate Go OpenAPI types/server interfaces' \
		'  make openapi-codegen-check Check generated Go OpenAPI code is current' \
		'  make openapi-ts-codegen Generate TypeScript SDK from OpenAPI' \
		'  make openapi-ts-codegen-check Check generated TypeScript SDK is current' \
		'  make sdk-ts-typecheck Typecheck generated TypeScript SDK' \
		'  make ent-generate    Generate Ent client from schema' \
		'  make ent-generate-check Check Ent generated code is current' \
		'  make migration-check Check versioned PostgreSQL migrations against Ent schema' \
		'  make api-test        Run Go API tests' \
		'  make api-run         Run the API locally with go run' \
		'  make dev-up          Start API, PostgreSQL, and Redis with Docker Compose' \
		'  make dev-down        Stop local Docker Compose services' \
		'  make smoke-health    Curl /api/v1/health on localhost' \
		'  make smoke-gateway   Login, create an API key, and smoke test local gateway endpoints' \
		'  make smoke-release   Validate health, readiness, metrics, and gateway smoke on localhost' \
		'  make backup-postgres BACKUP_FILE=...   Create a PostgreSQL custom-format backup' \
		'  make restore-postgres BACKUP_FILE=...  Restore a PostgreSQL custom-format backup' \
		'  make examples-check  Validate public examples and 2api migration guide' \
		'  make architecture-check  Run architecture and startup harness tests' \
		'  make code-quality-check  Run repository code-quality harness tests' \
		'  make diff-check     Check staged and unstaged diff whitespace' \
		'  make secret-scan     Scan source files for committed secrets' \
		'  make check           Run current contract and API checks'

bootstrap-env:
	@test -f .env || cp .env.example .env

openapi-lint:
	npx --yes @redocly/cli lint $(OPENAPI)

openapi-bundle:
	@mkdir -p $(dir $(OPENAPI_BUNDLE))
	npx --yes @redocly/cli bundle $(OPENAPI) --output $(OPENAPI_BUNDLE)

openapi-codegen:
	@mkdir -p $(dir $(OPENAPI_GO_OUTPUT))
	$(OAPI_CODEGEN) -config $(OPENAPI_GO_CONFIG) $(OPENAPI)

openapi-codegen-check:
	@set -e; \
	tmp="$$(mktemp)"; \
	$(OAPI_CODEGEN) -generate types,std-http -package openapi -o "$$tmp" $(OPENAPI); \
	cmp -s "$$tmp" "$(OPENAPI_GO_OUTPUT)" || (echo "$(OPENAPI_GO_OUTPUT) is out of date; run make openapi-codegen" >&2; rm -f "$$tmp"; exit 1); \
	rm -f "$$tmp"

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

migration-check:
	cd $(API_DIR) && go test ./internal/platform/db -run 'Test(EntSchemaAppliesToEmptyDatabase|PostgresInitialMigrationMatchesEntSchema|PostgresInitialDownMigrationCoversAllTables)'

api-test:
	cd $(API_DIR) && go test ./...

api-run:
	cd $(API_DIR) && go run ./cmd/srapi

architecture-check:
	cd $(API_DIR) && go test ./internal/config ./internal/architecture ./internal/app ./internal/platform/crypto ./internal/platform/db ./internal/platform/logger ./internal/platform/redis ./internal/modules/providers/preset ./internal/persistence/entstore/... ./internal/persistence/redisstore/... ./internal/workers/... ./internal/httpserver

code-quality-check:
	cd $(API_DIR) && go test ./internal/codequality

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

smoke-release:
	node tools/smoke-local.mjs --release

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

check: diff-check openapi-lint openapi-bundle openapi-codegen-check openapi-ts-codegen-check sdk-ts-typecheck ent-generate-check migration-check architecture-check code-quality-check examples-check api-test secret-scan
