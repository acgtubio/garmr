# TODO

## Fail loudly on a misconfigured service instance

**Problem seen (2026-10-13):** `garmr@zorya` (the hostname, not a user) was
started by mistake alongside `garmr@eyd`. It took the lock, found no config at
`/home/zorya/.config/garmr/config.json`, treated that as an empty blocklist and
silently blocked nothing. The real `garmr@eyd` then crash-looped on
`garmr run is already active` (32+ restarts) and nothing was blocked.

**Changes:**

1. **Missing config is an error for `run`/`apply`.** Add
   `config.LoadExisting` (returns `config.ErrNotFound`) and use it in
   `runner.Tick`. `Run` should exit immediately if the first tick can't load
   the config; later tick errors keep the current log-and-leave-hosts behavior.
   `block`/`schedule` keep using `Load` (missing file = new list). Log a
   warning when the config has zero sites.
2. **No restart loop on permanent errors.** `main` exits with code 3 for
   config-not-found and lock-held (`errors.Is` on `config.ErrNotFound` /
   `runner.ErrLocked`), 1 otherwise. Add `RestartPreventExitStatus=3` to
   `contrib/systemd/garmr@.service`.
3. **Say who holds the lock.** `runner.Acquire` writes PID and config path
   into the lock file. Show them in the `ErrLocked` message, the `apply`/`clear`
   refusal, and `garmr status` (`runner: active (pid N, config …)`).
4. **Small fixes.**
   - The unit's header comment contains the literal `@BINDIR@`/`@DOCDIR@`
     placeholders, which `make install`'s sed also rewrites; reword it.
   - README: the instance name must be your username (`garmr@$USER`, not
     `$HOST`) and only one instance can run at a time; add a "service running
     but nothing blocked" troubleshooting entry (`systemctl list-units
     'garmr@*'`, `garmr status`).
   - CLAUDE.md: document the exit-code-3 rule and the single-instance limit.

**Tests:** `LoadExisting` on a missing file; `Run` with a missing config
returns immediately and leaves hosts untouched; `ErrLocked` message includes
the holder PID; `run` with missing config maps to exit code 3; `status` shows
the runner PID.

**Verify:** `sudo systemctl start garmr@nosuchuser` ends `failed` with exit
status 3 and a "config not found" message, with no restart loop and no change
to `/etc/hosts`.
