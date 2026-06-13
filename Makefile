BIN_DIR ?= bin
REFUTE_BIN ?= $(BIN_DIR)/refute
SHATTER_BIN ?= shatter

.PHONY: build test vet fmt shatter shatter-clean

build:
	mkdir -p $(BIN_DIR)
	go build -buildvcs=false -o $(REFUTE_BIN) ./cmd/refute

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)

shatter:
	$(SHATTER_BIN) scan --project-dir . --language go --all --resume auto --progress .

shatter-clean:
	rm -rf .shatter-cache .shatter/seeds shatter-artifacts shatter-report
