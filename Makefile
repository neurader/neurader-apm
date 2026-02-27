BINARY    := neurader
BUILD_DIR := dist
CMD       := ./cmd/neurader
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -s -w -X main.version=$(VERSION)

.PHONY: all build build-amd64 build-arm64 build-arm clean test install tidy help

## all: Cross-compile for all supported Linux architectures
all: clean build-amd64 build-arm64 build-arm checksums

build-amd64:
	@echo "→ Building linux/amd64"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 $(CMD)

build-arm64:
	@echo "→ Building linux/arm64"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-arm64 $(CMD)

build-arm:
	@echo "→ Building linux/arm (v7)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-arm $(CMD)

## build: Build for the current machine only (fast, for development)
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(CMD)

checksums:
	@cd $(BUILD_DIR) && sha256sum $(BINARY)-linux-amd64 $(BINARY)-linux-arm64 $(BINARY)-linux-arm > checksums.sha256
	@cat $(BUILD_DIR)/checksums.sha256

## install: Build and install to /usr/local/bin (requires root)
install: build
	install -m 0755 $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "✓ Installed. Run: sudo neurader init"

## test: Run all tests
test:
	go test -v -race ./...

## tidy: Tidy go.mod and go.sum
tidy:
	go mod tidy

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## help: Show available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'