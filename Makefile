.PHONY: build shatter-clean

build:
	go build -buildvcs=false ./cmd/refute

shatter-clean:
	rm -rf .shatter-cache shatter-artifacts shatter-report
