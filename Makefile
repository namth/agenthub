.PHONY: build test

build:
	CGO_ENABLED=0 go build -o bin/agenthub ./cmd/server

test:
	go test ./...
