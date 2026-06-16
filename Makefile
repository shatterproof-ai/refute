BIN_DIR ?= bin
REFUTE_BIN ?= $(BIN_DIR)/refute
SHATTER_BIN ?= shatter

.PHONY: build test vet fmt shatter shatter-clean
.PHONY: verify

build:
	mkdir -p $(BIN_DIR)
	go build -buildvcs=false -o $(REFUTE_BIN) ./cmd/refute

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

# Unified pre-release verification (issue #97). Runs the full release-candidate
# suite — static analysis, formatting, vulnerability scan, unit tests, build,
# binary smoke test, integration, conformance, and docs checks — with clear
# pass/fail/skip output. Exit 1 on a real failure, 2 on an unsupported
# environment. Pass extra flags via VERIFY_FLAGS, e.g.
# `make verify VERIFY_FLAGS=--no-conformance`.
verify:
	scripts/verify.sh $(VERIFY_FLAGS)

shatter:
	$(SHATTER_BIN) scan --project-dir . --language go --all --resume auto --progress .

shatter-clean:
	rm -rf .shatter-cache .shatter/seeds shatter-artifacts shatter-report
