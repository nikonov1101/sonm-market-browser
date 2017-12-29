#!/usr/bin/env make
VER = v0.1
BUILD = $(shell git rev-parse --short HEAD)
FULL_VER = $(VER)-$(BUILD)

.PHONY: fmt vet test

all: vet fmt build

build:
	go build -tags "nocgo" -o market-ui main.go

vet:
	@echo "+ $@"
	@go tool vet $(shell ls -1 -d */ | grep -v -e vendor -e contracts)

fmt:
	@echo "+ $@"
	@test -z "$$(gofmt -s -l . 2>&1 | grep -v ^vendor/ | tee /dev/stderr)" || \
		(echo >&2 "+ please format Go code with 'gofmt -s'" && false)

test:
	@echo "+ $@"
	go test $(shell go list ./... | grep -vE 'vendor')
