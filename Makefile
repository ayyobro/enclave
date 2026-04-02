APP_NAME := enclave
BUILD_DIR := ./build
VERSION := 0.1.0

.PHONY: build run test clean serve

build:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/enclave/

run: build
	$(BUILD_DIR)/$(APP_NAME) chat

serve: build
	$(BUILD_DIR)/$(APP_NAME) serve

test:
	go test ./... -count=1

test-verbose:
	go test ./... -v -count=1

clean:
	rm -rf $(BUILD_DIR)

# Cross compilation
build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/enclave/

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 ./cmd/enclave/

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/enclave/

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/enclave/

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/enclave/

build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows
