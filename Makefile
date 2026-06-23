.PHONY: build test lint install clean

build:
	go build -o bin/canvas-pp-cli ./cmd/canvas-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/canvas-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/canvas-pp-mcp ./cmd/canvas-pp-mcp

install-mcp:
	go install ./cmd/canvas-pp-mcp

build-all: build build-mcp
