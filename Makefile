APP_S=stygai-status
APP_N=notify

VERSION := $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)


.PHONY: run build test fmt

run:
	@go run ./cmd/$(APP_S)

build:
	@go build -ldflags "$(LDFLAGS)" -o bin/$(APP_S) ./cmd/$(APP_S)
	@go build -ldflags "$(LDFLAGS)" -o bin/$(APP_N) ./cmd/$(APP_N)

test:
	@go test ./...

fmt:
	@go fmt ./...
