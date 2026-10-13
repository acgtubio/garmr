# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build -o garmr .                                   # build binary (ignored by .gitignore)
go build -ldflags "-X main.version=v0.1.0" -o garmr . # build with a release version
go run . -version                                     # run without building
go test ./...                                         # all tests
go test -run 'TestRun/version' .                      # single test / subtest
go vet ./... && gofmt -l .                            # lint
```

## Architecture

Minimal Go CLI, module `github.com/acgtubio/garmr`, standard library only (no CLI framework).

- Single `main` package at the repo root.
- `main()` is a thin wrapper: it calls `run(os.Args[1:], os.Stdout)`, prints any error to stderr prefixed with `garmr:`, and exits 1.
- `run(args []string, stdout io.Writer) error` holds all logic. It parses flags with its own `flag.FlagSet` (`ContinueOnError`) rather than the global `flag` package, so it can be called repeatedly from tests. `-h` returns nil, not an error.
- `version` is a package var defaulting to `"dev"`, set at build time via `-ldflags "-X main.version=..."`.
- Tests call `run` directly with a `bytes.Buffer` as stdout (table-driven, see `main_test.go`). Keep new behavior reachable through `run` so it stays testable this way.
