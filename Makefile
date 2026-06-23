.PHONY: build test lint install clean

build:
	go build -o bin/canvas-cli ./cmd/canvas-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/canvas-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/canvas-mcp ./cmd/canvas-mcp

install-mcp:
	go install ./cmd/canvas-mcp

build-all: build build-mcp
