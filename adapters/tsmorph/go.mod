// This directory is the ts-morph Node adapter, not Go source for the refute
// module. It is its own (effectively empty) Go module purely to mark a module
// boundary: it stops the root module's `go build ./...` / `go test ./...` from
// descending into adapters/tsmorph — most importantly into third-party Go code
// vendored under node_modules (e.g. node_modules/flatted/golang) when npm
// dependencies are installed. Keeping that subtree out of the root module's
// build, vet, coverage, and govulncheck surface is the whole point of this
// file. See GitHub issue #77.
module github.com/shatterproof-ai/refute/adapters/tsmorph

go 1.26.3
