.PHONY: build test vet fmt fmt-check lint no-network ci release-snapshot release-check clean

BINARY  := standin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

# Packages that can open a network connection. Standin must never link any of them.
NET_PKGS := ^(net|net/http|net/http/.*|net/rpc|net/rpc/.*|net/smtp|crypto/tls)$$

build:
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BINARY) ./cmd/standin

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "These files need gofmt (run: make fmt):"; echo "$$out"; exit 1; fi

# Runs golangci-lint only if it is installed.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; \
	else echo "golangci-lint not installed; skipping lint"; fi

# Fails if the standin binary links any network package.
no-network:
	@deps="$$(go list -deps ./cmd/standin)" || { echo "no-network: go list failed; cannot check"; exit 1; }; \
	bad="$$(printf '%s\n' "$$deps" | grep -E '$(NET_PKGS)' || true)"; \
	if [ -n "$$bad" ]; then echo "standin links network packages:"; echo "$$bad"; exit 1; fi; \
	echo "no-network: OK"

ci: fmt-check vet test no-network lint build

# Builds every platform into dist/ without publishing. Needs goreleaser.
release-snapshot:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check

clean:
	rm -rf $(BINARY) $(BINARY).exe dist/
