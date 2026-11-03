APP_S=stygai-status
APP_N=notify


.PHONY: run build test fmt

run:
	@go run ./cmd/$(APP_S)

build:
	@go build -o bin/$(APP_S) ./cmd/$(APP_S)
	@go build -o bin/$(APP_N) ./cmd/$(APP_N)

test:
	@go test ./...

fmt:
	@go fmt ./...
