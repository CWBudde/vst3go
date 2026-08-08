set shell := ["zsh", "-cu"]

default:
  @just --list

fmt:
  env XDG_CACHE_HOME=/tmp/treefmt-cache treefmt --allow-missing-formatter

fmt-check:
  env XDG_CACHE_HOME=/tmp/treefmt-cache treefmt --allow-missing-formatter --fail-on-change

check-formatted: fmt-check

lint:
  @if ! command -v golangci-lint >/dev/null 2>&1; then echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; fi
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache golangci-lint run ./pkg/...

lint-fix:
  @if ! command -v golangci-lint >/dev/null 2>&1; then echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; fi
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache golangci-lint run --fix ./pkg/...

test-go:
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./pkg/...

# Also cross-builds the Windows DLL and checks its export table when a
# MinGW-w64 toolchain is installed; without one that part skips.
#
# Build a real loadable module and drive it from C
test-smoke:
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags smoke -count=1 ./test/smoke/...

# Needs a validator binary: put one on PATH or set VST3_VALIDATOR. Skips when
# neither is present.
#
# Run Steinberg's own conformance suite
test-validator:
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags validator -count=1 ./test/smoke/...

test: fmt-check test-go

# The cheapest check that the cgo bridge still builds there; test-smoke
# additionally verifies the DLL exports what a host looks for.
#
# Cross-compile the smoke plugin for Windows
build-windows arch="amd64":
  @if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then echo "x86_64-w64-mingw32-gcc not found. Install MinGW-w64 (Debian/Ubuntu: apt install gcc-mingw-w64-x86-64)."; exit 1; fi
  env GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
    GOOS=windows GOARCH={{arch}} CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
    go build -buildvcs=false -buildmode=c-shared -o /tmp/vst3go-smoke.dll ./test/smoke/plugin

fix:
    just lint-fix
    just fmt
