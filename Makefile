BINARY=cloudflare-exporter

.PHONY: build test test-ci run fmt tidy

build:
	go build -o bin/$(BINARY) ./cmd/cloudflare-exporter

test:
	go test ./...

test-ci:
	go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...

run:
	go run ./cmd/cloudflare-exporter

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
