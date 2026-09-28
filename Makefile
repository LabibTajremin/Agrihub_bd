# AgriSmart — single entry point for developers and CI (CI runs these exact targets).
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
GO ?= go
FLUTTER ?= flutter
DART ?= dart
GOLANGCI ?= golangci-lint
BACKEND := backend
MOBILE := mobile
QUIET := ./scripts/quiet.sh
COVER_PKGS := ./...

.PHONY: verify verify-backend verify-mobile lint vet arch-test unit integration test coverage-gate \
	run migrate seed docker openapi i18n-sync mobile-deps mobile-analyze mobile-test mobile-coverage-gate \
	security e2e loadtest tools

verify: verify-backend verify-mobile ## everything CI checks

verify-backend: lint vet arch-test unit integration coverage-gate

lint:
	cd $(BACKEND) && $(GOLANGCI) run ./...

vet:
	cd $(BACKEND) && $(GO) vet ./... && $(GO) vet -tags integration ./...

arch-test:
	@if [ -d $(BACKEND)/internal/platform/archtest ]; then \
		cd $(BACKEND) && ../$(QUIET) "arch-test" $(GO) test -count=1 ./internal/platform/archtest/...; \
	else echo "arch-test: not yet present"; fi

unit:
	cd $(BACKEND) && ../$(QUIET) "unit" $(GO) test -race -count=1 ./...

integration:
	cd $(BACKEND) && ../$(QUIET) "integration+coverage" $(GO) test -race -count=1 -tags integration \
		-covermode=atomic -coverpkg=$(COVER_PKGS) -coverprofile=coverage.out ./...

test: unit integration

coverage-gate:
	cd $(BACKEND) && $(GO) run scripts/coverage_gate.go -profile coverage.out -threshold 100 -v

GOBIN := $(shell $(GO) env GOPATH)/bin

tools: ## pinned helper binaries (govulncheck, vegeta)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@v1.8.0
	$(GO) install github.com/tsenart/vegeta/v12@v12.13.0

security: ## govulncheck (gosec runs inside `make lint`) + flutter analyze
	cd $(BACKEND) && $(GOBIN)/govulncheck ./...
	cd $(MOBILE) && $(FLUTTER) analyze --fatal-infos

e2e: ## Flutter client code against the real API + Postgres (needs Docker)
	./scripts/e2e.sh

loadtest: ## vegeta baseline on the top 5 endpoints (needs Docker); RATE=, DURATION=
	./scripts/with-stack.sh ./scripts/loadtest.sh

run:
	cd $(BACKEND) && $(GO) run ./cmd/api -config config.example.yaml

migrate:
	cd $(BACKEND) && $(GO) run ./cmd/migrate up

seed:
	cd $(BACKEND) && $(GO) run ./cmd/seed

openapi:
	cd $(BACKEND) && $(GO) test -tags integration -count=1 ./internal/app -run "TestOpenAPI_UpToDate|TestErrorCatalogue" -update

docker:
	docker build -f deploy/docker/Dockerfile -t agrismart-api:local .

i18n-sync:
	cp $(BACKEND)/internal/modules/localization/seed/*.json $(MOBILE)/assets/i18n/

verify-mobile: mobile-deps mobile-analyze mobile-test mobile-coverage-gate

mobile-deps:
	cd $(MOBILE) && $(FLUTTER) pub get >/dev/null

mobile-analyze:
	cd $(MOBILE) && $(FLUTTER) analyze --fatal-infos

mobile-test:
	cd $(MOBILE) && ../$(QUIET) "flutter test" $(FLUTTER) test --coverage

mobile-coverage-gate:
	cd $(MOBILE) && $(DART) run tool/coverage_gate.dart coverage/lcov.info 100
