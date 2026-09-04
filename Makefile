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
