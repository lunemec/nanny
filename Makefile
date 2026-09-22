.PHONY: build docker run test vet lint snapshot release-check release-preflight release-preflight-test release
SHELL := /bin/bash
export TESTS
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
	@printf $(row) "release" "Publish from a clean, tagged checkout."
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

run: 
	LOGXI=* go run -race main.go

test: 
	go test -race -cover -v ./...

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...

snapshot:
	$(GORELEASER) release --snapshot --clean

release-check:
	$(GORELEASER) check

release-preflight:
	@./scripts/release-preflight.sh

release-preflight-test:
	@./scripts/test-release-preflight.sh

release: release-preflight
	@test -n "$(GITHUB_TOKEN)" || (echo "GITHUB_TOKEN is required"; exit 1)
	$(GORELEASER) release --clean
