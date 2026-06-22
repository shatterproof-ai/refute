BIN_DIR ?= bin
REFUTE_BIN ?= $(BIN_DIR)/refute
SHATTER_BIN ?= shatter

.PHONY: build test vet fmt shatter shatter-clean
.PHONY: verify verify-report corpus corpus-clean

build:
	mkdir -p $(BIN_DIR)
	go build -buildvcs=false -o $(REFUTE_BIN) ./cmd/refute

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

# Unified pre-release verification (issues #97, #120). Runs the full
# release-candidate suite — static analysis, formatting, vulnerability scan,
# unit tests, build, binary smoke test, integration, conformance, and docs
# checks — with clear PASS/FAIL/SKIP/UNAVAIL output. Exit 1 on a real failure,
# 2 on an unsupported environment. Pass extra flags via VERIFY_FLAGS, e.g.
# `make verify VERIFY_FLAGS=--no-conformance`.
#
# `verify` stops at the first failing gate (fast local feedback). `verify-report`
# keeps going through every gate and reports a summary — use it for a full audit
# so one failing gate never hides later checks.
verify:
	scripts/verify.sh --fail-fast $(VERIFY_FLAGS)

verify-report:
	scripts/verify.sh --keep-going $(VERIFY_FLAGS)

shatter:
	$(SHATTER_BIN) scan --project-dir . --language go --all --resume auto --progress .

shatter-clean:
	rm -rf .shatter-cache .shatter/seeds shatter-artifacts shatter-report

# Pinned real-world refactoring corpus (issue #96). Materializes upstream repos
# at fixed commits and runs refute renames against them, then verifies the
# project still builds/tests. Network-dependent and separated from `make test`;
# targets whose backend or toolchain is absent skip with a reason. Set
# REFUTE_CORPUS_ALLOW_NETWORK_VERIFY=1 to also run dependency-installing verify
# steps (npm ci, mvn compile).
corpus:
	scripts/corpus-fetch.sh
	go test -tags corpus ./internal/corpus/ -v

corpus-clean:
	rm -rf .corpus-cache
