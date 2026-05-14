SHATTER_BIN ?= $(HOME)/project/shatter/target/release/shatter

.PHONY: build shatter shatter-clean

build:
	go build -buildvcs=false ./cmd/refute

shatter:
	$(SHATTER_BIN) scan --project-dir . --language go --all --resume auto --progress .

shatter-clean:
	rm -rf .shatter-cache shatter-artifacts shatter-report
