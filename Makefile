.DEFAULT_GOAL := help

GO ?= go
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

.PHONY: help setup build run install fmt fmt-check vet test test-race coverage check clean generate generate-check dart-check emulator-test integration-test dist setup-integration check-all schema-format schema-format-check audit-test release release-check installer-test typescript-check

help: ## Show available development commands
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup: ## Check Go and download declared module dependencies
	$(GO) version
	$(GO) mod download
	$(GO) mod verify

build: ## Build bin/fql (override VERSION for version metadata)
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/fql ./cmd/fql

run: ## Run the CLI (example: make run ARGS=version)
	$(GO) run ./cmd/fql $(ARGS)

install: ## Install the CLI into GOBIN or GOPATH/bin
	$(GO) install -trimpath -ldflags '$(LDFLAGS)' ./cmd/fql

fmt: ## Format Go source
	$(GO) fmt ./...

fmt-check: ## Fail if Go source needs formatting
	@files="$$(gofmt -l cmd internal tests tools)"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi

vet: ## Run Go static checks
	$(GO) vet ./...

test: ## Run tests from dedicated test directories
	$(GO) test ./...

test-race: ## Run tests with the race detector (requires a C toolchain)
	$(GO) test -race ./...

coverage: ## Write compiler coverage to coverage/coverage.out
	@mkdir -p coverage
	$(GO) test -coverpkg=./internal/... -coverprofile=coverage/coverage.out ./tests/...
	$(GO) tool cover -func=coverage/coverage.out

check: fmt-check vet test build generate-check schema-format-check ## Run required local checks

clean: ## Remove only known local build and coverage artifacts
	rm -f bin/fql coverage/coverage.out

generate: ## Generate artifacts for the Flutter example
	$(GO) run ./cmd/fql generate --project examples/flutter
	$(GO) run ./cmd/fql generate --project tests/dart
	$(GO) run ./cmd/fql generate --project tests/typescript

generate-check: ## Verify committed example artifacts are current
	$(GO) run ./cmd/fql generate --project examples/flutter --check
	$(GO) run ./cmd/fql generate --project tests/dart --check
	$(GO) run ./cmd/fql generate --project tests/typescript --check

dart-check: ## Analyze and test generated Dart (requires Flutter)
	cd examples/flutter && flutter pub get && flutter analyze
	cd tests/dart && flutter pub get && flutter analyze && flutter test test/runtime_test.dart test/defaults_test.dart test/extensions_test.dart

emulator-test: generate-check ## Test generated Security Rules (requires Node 24.12+, Java 21+, npm ci)
	npm run test:rules

integration-test: generate-check ## Test Flutter web SDK integration against emulators
	npm run test:integration

dist: ## Build checksummed cross-platform CLI archives (set VERSION=0.1.0-dev.3)
	$(GO) run ./tools/release --version '$(VERSION)'
	python3 tests/release/verify_archives.py --version '$(VERSION)'

setup-integration: ## Install locked test dependencies and Chromium (requires Node/Flutter/Java)
	npm ci --no-fund
	cd examples/flutter && flutter pub get
	cd tests/dart && flutter pub get
	npx playwright install chromium

check-all: check dart-check typescript-check emulator-test integration-test audit-test ## Run the full local verification suite

schema-format: ## Validate and format example/test schemas
	$(GO) run ./cmd/fql format --project examples/flutter
	$(GO) run ./cmd/fql format --project tests/dart

schema-format-check: ## Check schema validity and canonical formatting without writes
	$(GO) run ./cmd/fql format --check --project examples/flutter
	$(GO) run ./cmd/fql format --check --project tests/dart

audit-test: generate-check ## Verify read-only audit against a local emulator
	npm run test:audit

installer-test: ## Test installer platform selection, integrity checks, and safe replacement
	python3 -m unittest discover -s tests/install -p '*_test.py'

release: ## Verify, build and publish GitHub Release assets (set VERSION=0.1.0-dev.3; needs gh auth)
	$(GO) run ./tools/publish --version '$(VERSION)'

release-check: installer-test ## Check only release tooling and executable packaging prerequisites
	$(GO) test ./tests/release
	$(GO) vet ./internal/release ./tools/release ./tools/publish
	sh -n install.sh

typescript-check: ## Compile generated TypeScript and run focused runtime checks
	npm run test:typescript
