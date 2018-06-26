SHELL := /bin/bash
export TESTS
header = "  \e[1;34m%-30s\e[m \n"
row = "\e[1mmake %-32s\e[m %-50s \n"

all:
	@printf $(header) "Build"
	@printf $(row) "build" "Build production binary."
	@printf $(header) "Dev"
	@printf $(row) "run" "Run Nanny in dev mode, all logging and race detector ON."
	@printf $(row) "test" "Run tests."
	@printf $(row) "vet" "Run go vet."
	@printf $(row) "lint" "Run gometalinter (you have to install it)."

build:
	go build

run: 
	LOGXI=* go run -race main.go

test: 
	go test ./...

vet:
	go vet ./...

lint:
	gometalinter.v2 --vendor ./...