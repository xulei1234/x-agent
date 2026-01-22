# Makefile for Go project (Windows/Linux compatible)
# - version inject: module/common.{GitCommit,GoVersion,BuildTime,BuildHost}
# - build output: ./bin/x-agent(.exe)

APP_NAME := x-agent
MAIN_PKG := ./main.go
OUT_DIR  := bin
GO       := go

# Detect OS
ifeq ($(OS),Windows_NT)
	EXE         := .exe
	NULLDEV     := NUL
	SHELL       := cmd
	.SHELLFLAGS := /C
else
	EXE     :=
	NULLDEV := /dev/null
endif

BIN := $(OUT_DIR)/$(APP_NAME)$(EXE)

# Git / build metadata (redirect to OS-specific null device)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>$(NULLDEV) || echo unknown)
GO_VERSION := $(shell $(GO) env GOVERSION 2>$(NULLDEV) || echo unknown)
BUILD_HOST := $(shell hostname 2>$(NULLDEV) || echo unknown)

# Build time: prefer PowerShell on Windows; date on Unix
ifeq ($(OS),Windows_NT)
	BUILD_TIME := $(shell powershell -NoProfile -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssK'" 2>$(NULLDEV) || echo unknown)
else
	BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>$(NULLDEV) || echo unknown)
endif

LDFLAGS := -s -w \
	-X github.com/xulei1234/x-agent/module/common.GitCommit=$(GIT_COMMIT) \
	-X github.com/xulei1234/x-agent/module/common.GoVersion=$(GO_VERSION) \
	-X github.com/xulei1234/x-agent/module/common.BuildTime=$(BUILD_TIME) \
	-X github.com/xulei1234/x-agent/module/common.BuildHost=$(BUILD_HOST)

.PHONY: all build run test cover fmt vet lint tidy clean version

all: build

$(OUT_DIR):
ifeq ($(OS),Windows_NT)
	@if not exist $(OUT_DIR) mkdir $(OUT_DIR)
else
	@mkdir -p $(OUT_DIR)
endif

build: $(OUT_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(MAIN_PKG)

run: build
	$(BIN) run

version: build
	$(BIN) version

test:
	$(GO) test ./... -count=1

cover: $(OUT_DIR)
	$(GO) test ./... -count=1 -coverprofile=$(OUT_DIR)/coverage.out
	$(GO) tool cover -func=$(OUT_DIR)/coverage.out

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# optional: only runs if golangci-lint is installed
lint:
ifeq ($(OS),Windows_NT)
	@where golangci-lint > $(NULLDEV) 2> $(NULLDEV) && golangci-lint run ./... || echo golangci-lint not found, skip
else
	@command -v golangci-lint > $(NULLDEV) 2> $(NULLDEV) && golangci-lint run ./... || echo golangci-lint not found, skip
endif

tidy:
	$(GO) mod tidy

clean:
ifeq ($(OS),Windows_NT)
	@if exist $(OUT_DIR) rmdir /S /Q $(OUT_DIR)
else
	@rm -rf $(OUT_DIR)
endif
