.PHONY: all build clean test run-agent run-coordinator run-monitor deps

BINARY_NAME=proxy-v6
AGENT_BINARY=bin/agent
COORDINATOR_BINARY=bin/coordinator
MONITOR_BINARY=bin/monitor

all: deps build

deps:
	go mod download
	go mod tidy

build: build-agent build-coordinator build-monitor

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-s -w -X proxy-v6/pkg/version.Version=$(VERSION) -X proxy-v6/pkg/version.GitCommit=$(GIT_COMMIT) -X proxy-v6/pkg/version.BuildDate=$(BUILD_DATE)"

build-agent:
	go build $(LDFLAGS) -o $(AGENT_BINARY) cmd/agent/main.go

build-coordinator:
	go build $(LDFLAGS) -o $(COORDINATOR_BINARY) cmd/coordinator/main.go

build-monitor:
	go build $(LDFLAGS) -o $(MONITOR_BINARY) cmd/monitor/main.go

clean:
	go clean
	rm -f $(AGENT_BINARY) $(COORDINATOR_BINARY) $(MONITOR_BINARY)

test:
	go test -v ./...

run-agent: build-agent
	$(AGENT_BINARY) --coordinator http://localhost:8081

run-coordinator: build-coordinator
	$(COORDINATOR_BINARY)

run-monitor: build-monitor
	$(MONITOR_BINARY) --coordinator http://localhost:8081

install-tinyproxy:
	@echo "Installing tinyproxy..."
	@if [ "$$(uname)" = "Darwin" ]; then \
		brew install tinyproxy; \
	elif [ "$$(uname)" = "Linux" ]; then \
		if [ -f /etc/debian_version ]; then \
			sudo apt-get update && sudo apt-get install -y tinyproxy; \
		elif [ -f /etc/redhat-release ]; then \
			sudo yum install -y tinyproxy; \
		else \
			echo "Unsupported Linux distribution"; \
			exit 1; \
		fi; \
	else \
		echo "Unsupported OS"; \
		exit 1; \
	fi

docker-build:
	docker build -t proxy-v6:latest .

help:
	@echo "Available targets:"
	@echo "  make deps              - Download and tidy dependencies"
	@echo "  make build             - Build all binaries"
	@echo "  make build-agent       - Build agent binary"
	@echo "  make build-coordinator - Build coordinator binary"
	@echo "  make build-monitor     - Build monitor binary"
	@echo "  make clean             - Clean build artifacts"
	@echo "  make test              - Run tests"
	@echo "  make run-agent         - Build and run agent"
	@echo "  make run-coordinator   - Build and run coordinator"
	@echo "  make run-monitor       - Build and run monitor"
	@echo "  make install-tinyproxy - Install tinyproxy dependency"
	@echo "  make help              - Show this help message"