BINARY=cloudflare-exporter

.PHONY: build test run fmt tidy

build:
	go build -o bin/$(BINARY) ./cmd/cloudflare-exporter

test:
	go test ./...

run:
	go run ./cmd/cloudflare-exporter

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
