.PHONY: build docker buildah push package run test vet lint clean
SHELL := /bin/bash
export TESTS
header = "  \e[1;34m%-30s\e[m \n"
row = "\e[1mmake %-32s\e[m %-50s \n"
VERSION := $(shell cat VERSION)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X nanny/pkg/version.Version=$(VERSION) -X nanny/pkg/version.GitCommit=$(GIT_COMMIT) -X nanny/pkg/version.BuildDate=$(BUILD_DATE)

all:
	@printf $(header) "Build"
	@printf $(row) "build" "Build production binary."
	@printf $(row) "docker" "Build a nanny container image using Docker."
	@printf $(row) "buildah" "Build a nanny container image using Buildah."
	@printf $(row) "push" "Push the latest and current version tagged container images to Docker Hub and Quay.io."
	@printf $(row) "package" "Build and create .tar.gz."
	@printf $(row) "clean" "Clean from build artefacts."
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

buildah:
	buildah bud --no-cache --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t docker.io/library/lunemec/nanny:$(VERSION) .
	buildah tag docker.io/library/lunemec/nanny:$(VERSION) docker.io/library/lunemec/nanny:latest

push:
	buildah push docker.io/library/lunemec/nanny:$(VERSION) docker://quay.io/nanny/nanny:$(VERSION)
	buildah push docker.io/library/lunemec/nanny:latest docker://quay.io/nanny/nanny:latest
	buildah push docker.io/library/lunemec/nanny:$(VERSION) docker://docker.io/lunemec/nanny:$(VERSION) 
	buildah push docker.io/library/lunemec/nanny:latest docker://docker.io/lunemec/nanny:latest

package: clean build
	scripts/package.sh

run: 
	LOGXI=* go run -race main.go

test: 
	go test -race -cover -v ./...

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...

clean:
	rm nanny || true
	rm *.tar.gz || true
