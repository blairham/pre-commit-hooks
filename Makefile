BUILD_DIR := build

# NOTE: no `go tool` targets here on purpose. A go.mod `tool` block requires
# Go 1.24+, and this module deliberately targets an older release so it builds
# under pre-commit's GOTOOLCHAIN=local. Formatting uses the toolchain's own
# gofmt; golangci-lint runs as a pre-commit hook and in CI.

.PHONY: all build clean test test-cover fmt vet tidy check check-versions sync

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/ ./cmd/...

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	go clean

test:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

vet:
	go vet ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

# Dogfooding: this repo's own toolchain pin is checked by its own hook.
check-versions:
	go run ./cmd/check-go-version-sync -mode=min

sync:
	go run ./cmd/check-go-version-sync -fix || true

# There is no lint target: golangci-lint runs as a pre-commit hook and in CI.
check: fmt vet test check-versions
