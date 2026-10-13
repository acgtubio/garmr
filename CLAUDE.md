# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make build                                            # go build with version from git describe
make test                                             # go vet + go test
go test -run TestWindowContains ./internal/schedule/  # single test
gofmt -l .                                            # formatting check
make install DESTDIR=/tmp/stage                       # stage install without root to inspect output
systemd-analyze verify <staged>/garmr@<user>.service  # check the unit (verify an instance, not the template)
```

`sudo make install` / `sudo make uninstall` change the real system; leave those to the user. `install` refuses to build as root, so `make build` must come first.

## Architecture

Go CLI that blocks websites on a daily schedule by managing a section of `/etc/hosts`. Module `github.com/acgtubio/garmr`, standard library only (no CLI framework). Linux-only (`syscall.Flock`, `/etc/hosts`, `/run`).

The packages split along what / how / when, plus a small runner layer:

- **What** to block: `internal/config`. A JSON file at `~/.config/garmr/config.json` with an optional default `schedule` window and `sites`. A site is either a bare domain string (uses the default) or `{"domain", "windows": [...]}`; `Site` has custom JSON (un)marshalling for this, and old string-only configs still load. `Load` re-validates every domain with `NormalizeDomain` because domains are written verbatim into the hosts file. `ActiveSites(now)` is the single source of truth for which sites are blocked: the site's own windows, else the default, else always. Under sudo, `DefaultPath` resolves `SUDO_USER`'s config; a root service with no `SUDO_USER` needs `-config`.
- **When**: `internal/schedule`. `Clock` (minutes since midnight, JSON `"HH:MM"`) and `Window` (start inclusive, end exclusive; `end < start` wraps midnight; `start == end` is all day), in local time. Pure functions only.
- **How**: `internal/hosts`. Owns the block between `# BEGIN garmr` and `# END garmr`. For each domain it writes `0.0.0.0` and `::` entries plus a `www.` variant. `Apply`/`Clear` are idempotent, skip the write when nothing changed, and write atomically (temp file + rename). Content outside the markers must stay byte-for-byte unchanged.
- **Runner**: `internal/runner`. `Tick(cfg, hosts, now)` = load config → `ActiveSites` → `hosts.Apply`. `Run` ticks at each minute boundary, re-reading the config each time. On a config/hosts error it logs and leaves the hosts file untouched, and on exit it does **not** clear blocks. Both are deliberate so stopping or breaking garmr is not an easy unblock.
- **Lock**: `runner.Acquire` takes an exclusive flock on `-lock` (default `/run/garmr.lock`) for `garmr run`. `apply` and `clear` check `runner.Held` and refuse while a runner is active, so manual commands can't fight the loop.

Service: `contrib/systemd/garmr@.service` is a template keyed by username (`garmr@eyd`) that runs `garmr -config /home/%i/.config/garmr/config.json run` as root. `make install` substitutes `@BINDIR@`/`@DOCDIR@`. Its hardening (`ProtectSystem=strict`, `ReadWritePaths=/etc /run`, `ProtectHome=read-only`, `PrivateNetwork`) means garmr may only write `/etc` and `/run`; if new code needs to write elsewhere or use the network, update the unit too. Keep scheduling logic in `runner`/`schedule`, not in the unit.

`README.md` is the user-facing docs (installed to `/usr/local/share/doc/garmr`); update it when commands, flags or config format change.

CLI (`main.go`):
- `main()` wraps `run(args, stdout)`, which parses global flags (`-config`, `-hosts`, `-lock`, `-version`) and dispatches to `app` methods: `block add [-window ...]|remove|list`, `schedule set|clear`, `status`, `run`, `apply`, `clear`.
- `block add` with `-window` replaces an existing site's windows; without it an existing site is left unchanged.
- The package var `now` stands in for `time.Now` so tests can fix the clock.
- Writing the real `/etc/hosts` needs root. Permission errors get a "try running with sudo" hint.
- `version` is set at build time via `-ldflags "-X main.version=..."`.

Testing: tests call `run` directly, always passing `-config`, `-hosts` and `-lock` paths under `t.TempDir()`. Never let tests or manual checks touch the real `/etc/hosts` or `/run`; use copies.

Known limitations of hosts-file blocking: there are no wildcards, so `m.example.com` must be added separately. Browsers may serve cached DNS results or use DNS-over-HTTPS and bypass the block.
