BUILD_DIR := build

# NOTE: no `go tool` targets here on purpose. A go.mod `tool` block requires
# Go 1.24+, and this module deliberately targets an older release so it builds
# under pre-commit's GOTOOLCHAIN=local. Formatting uses the toolchain's own
# gofmt; golangci-lint runs from CI (or from PATH if you have it installed).

.PHONY: all build clean test test-cover lint fmt vet tidy check check-versions sync

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

lint:
	@command -v golangci-lint >/dev/null 2>&1 \
		&& golangci-lint run ./... \
		|| echo "golangci-lint not on PATH — skipping (CI runs it)"

tidy:
	go mod tidy

# Dogfooding: this repo's own toolchain pin is checked by its own hook.
check-versions:
	go run ./cmd/check-go-version-sync -mode=min

sync:
	go run ./cmd/check-go-version-sync -fix || true

check: fmt vet lint test check-versions
