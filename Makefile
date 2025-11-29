# Makefile for syncnorris

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)
BUILD_DIR := dist

.PHONY: all
all: clean build-all

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

.PHONY: build
build:
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris cmd/syncnorris/main.go

.PHONY: build-all
build-all:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-linux-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-linux-arm64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-windows-amd64.exe cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-darwin-amd64 cmd/syncnorris/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/syncnorris-darwin-arm64 cmd/syncnorris/main.go

.PHONY: test
test:
	go test ./... -v -race -coverprofile=coverage.out

.PHONY: test-short
test-short:
	go test ./... -short

.PHONY: test-coverage
test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-unit
test-unit:
	go test ./pkg/... -v -race

.PHONY: test-integration
test-integration:
	go test ./tests/... -v -race

.PHONY: lint
lint:
	go vet ./...
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run

.PHONY: run
run:
	go run cmd/syncnorris/main.go

.PHONY: install
install:
	go install cmd/syncnorris/main.go
