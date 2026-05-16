GOCACHE ?= $(CURDIR)/.cache/go-build
export GOCACHE

.PHONY: test build lint bench perf

test:
	go test ./...

build:
	go build -o tpc ./cmd/tfplanctx

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; skipping"; \
	fi

bench:
	go run ./cmd/tpcbench

perf:
	go test -run '^$$' -bench . ./...
