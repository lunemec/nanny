.PHONY: build docker docker-check lint module-check release release-check release-preflight release-preflight-test release-verify run snapshot test vet vulncheck
SHELL := /bin/bash
export TESTS
export GITHUB_TOKEN
export RELEASE_TAG
header = "  \e[1;34m%-30s\e[m \n"
row = "\e[1mmake %-32s\e[m %-50s \n"
VERSION ?= $(shell git describe --tags --always --dirty)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X nanny/pkg/version.Version=$(VERSION) -X nanny/pkg/version.GitCommit=$(GIT_COMMIT) -X nanny/pkg/version.BuildDate=$(BUILD_DATE)
GORELEASER := go run github.com/goreleaser/goreleaser/v2@v2.18.2

all:
	@printf $(header) "Build"
	@printf $(row) "build" "Build production binary."
	@printf $(row) "docker" "Build a nanny container image using Docker."
	@printf $(row) "snapshot" "Build all release artifacts without publishing."
	@printf $(row) "release-check" "Validate the GoReleaser configuration."
	@printf $(row) "release-preflight" "Verify the release tag matches pushed master."
	@printf $(row) "release-verify" "Run every non-publishing release gate."
	@printf $(row) "release" "Verify, tag, and publish from clean master."
	@printf $(header) "Dev"
	@printf $(row) "run" "Run Nanny in dev mode, all logging and race detector ON."
	@printf $(row) "test" "Run tests."
	@printf $(row) "vet" "Run go vet."
	@printf $(row) "lint" "Run the pinned golangci-lint version."

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o nanny .

docker:
	docker build --no-cache --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t lunemec/nanny:$(VERSION) .
	docker tag lunemec/nanny:$(VERSION) lunemec/nanny:latest

docker-check:
	docker build --tag nanny:verify .
	docker run --rm --entrypoint /bin/sh nanny:verify -ec '\
		test "$$(id -u)" = 1000; \
		test "$$(id -g)" = 1000; \
		test "$$PWD" = /var/lib/nanny; \
		test -x /usr/bin/nanny; \
		test -w /var/lib/nanny; \
		grep -q '\''addr="0.0.0.0:8080"'\'' /etc/nanny/nanny.toml; \
		test -z "$$NANNY_ADDR"; \
		test -z "$$NANNY_STORAGE_DSN"; \
	'

module-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum
	go mod verify

run: 
	LOGXI=* go run -race main.go

test:
	go test -race -shuffle=on -cover ./...

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...

vulncheck:
	go tool govulncheck ./...

snapshot:
	$(GORELEASER) release --snapshot --clean

release-check:
	$(GORELEASER) check

release-preflight:
	@./scripts/release-preflight.sh

release-preflight-test:
	@./scripts/test-release-preflight.sh

release-verify:
	@$(MAKE) module-check
	@$(MAKE) release-preflight-test
	@$(MAKE) build
	@$(MAKE) vet
	@$(MAKE) lint
	@$(MAKE) test
	@$(MAKE) vulncheck
	@$(MAKE) docker-check
	@$(MAKE) release-check
	@$(MAKE) snapshot

release:
	@test -n "$$GITHUB_TOKEN" || (echo "GITHUB_TOKEN is required"; exit 1)
	@./scripts/release.sh $(GORELEASER)
