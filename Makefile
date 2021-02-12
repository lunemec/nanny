.PHONY: build
SHELL := /bin/bash
export TESTS
header = "  \e[1;34m%-30s\e[m \n"
row = "\e[1mmake %-32s\e[m %-50s \n"
VERSION := $(shell cat VERSION)

all:
	@printf $(header) "Build"
	@printf $(row) "build" "Build production binary."
	@printf $(row) "docker" "Build a nanny Docker image."
	@printf $(row) "package" "Build and create .tar.gz."
	@printf $(row) "clean" "Clean from build artefacts."
	@printf $(header) "Dev"
	@printf $(row) "run" "Run Nanny in dev mode, all logging and race detector ON."
	@printf $(row) "test" "Run tests."
	@printf $(row) "vet" "Run go vet."
	@printf $(row) "lint" "Run gometalinter (you have to install it)."

build:
	go get github.com/ahmetb/govvv
	govvv build -pkg nanny/pkg/version

docker:
	docker build -t nanny:$(VERSION) .
	docker tag nanny:$(VERSION) nanny:latest

buildah:
	buildah bud -t docker.io/library/nanny:$(VERSION) .
	buildah tag docker.io/library/nanny:$(VERSION) docker.io/library/nanny:latest

package: clean build
	scripts/package.sh

run: 
	LOGXI=* go run -race main.go

test: 
	go test -race -cover -v ./...

vet:
	go vet ./...

lint:
	gometalinter.v2 --deadline 120s --vendor ./...

clean:
	rm nanny || true
	rm *.tar.gz || true
