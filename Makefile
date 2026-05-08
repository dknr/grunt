VERSION  ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DIRTY    ?= $(shell git diff --quiet HEAD 2>/dev/null || echo "+dirty")
TIMESTAMP?= $(shell date -u +"%Y-%m-%dT%H:%M:%S")

LDFLAGS := -s -w \
	-X grunt/cmd.version=$(VERSION)$(DIRTY) \
	-X grunt/cmd.timestamp=$(TIMESTAMP) \
	-X grunt/cmd.commit=$(COMMIT)

BINARY_NAME ?= grunt
BUILD_DIR   ?= dist
CODEGEN_BIN ?= oapi-codegen

GOPATH_BIN ?= $(shell go env GOPATH)/bin

.PHONY: build clean test test-integration test-short version install build-all codegen

build: codegen
	cd cmd && go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME) ./grunt

codegen:
	@if ! command -v oapi-codegen >/dev/null 2>&1; then \
		go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest; \
	fi
	$(GOPATH_BIN)/oapi-codegen -package server -generate std-http,types,spec -o server/openapi.gen.go openapi.yaml

clean:
	rm -rf $(BUILD_DIR)
	rm -f server.json.log

test:
	go test -v -race ./...
	cd cmd && go test -v -race ./...

test-integration:
	cd cmd && go test -v -race ./internal/testutil/...

test-short:
	go test -v ./...
	cd cmd && go test -v ./...

version:
	@echo "Version: $(VERSION)$(DIRTY)"
	@echo "Commit:  $(COMMIT)"
	@echo "Time:    $(TIMESTAMP)"

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

build-all: codegen
	cd cmd && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./grunt
	cd cmd && GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./grunt
	cd cmd && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./grunt
	cd cmd && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./grunt
	cd cmd && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./grunt