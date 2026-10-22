.PHONY: build build-sandbox test lint install clean

build:
	go build -o bin/canvas-pp-cli ./cmd/canvas-pp-cli

# The build Canvas Builder bundles. The stamp makes the workspace's settings
# mandatory, so this binary refuses to run anywhere else.
build-sandbox:
	GOOS=darwin GOARCH=arm64 go build \
		-ldflags "-X github.com/johnnyrobot/canvas-pp-cli/internal/sandbox.requiredStamp=canvas-builder" \
		-o bin/canvas-pp-cli-sandbox ./cmd/canvas-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/canvas-pp-cli

clean:
	rm -rf bin/
