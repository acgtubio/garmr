# garmr

garmr blocks distracting websites during the hours you choose. It works by
adding a managed section to `/etc/hosts` that points blocked domains at
`0.0.0.0`, and a small background service keeps that section in sync with your
schedule.

- One default blocking window (e.g. `09:00-17:00`), plus optional per-site windows.
- Runs as a systemd service; config edits take effect within a minute.
- Stopping or crashing the service does **not** lift the block.
- Linux only. No dependencies beyond Go to build.

## Install

Requires Go 1.26+ and systemd.

```sh
make build                 # as your normal user
sudo make install          # binary -> /usr/local/bin, unit -> /etc/systemd/system
```

Set up your blocklist as your normal user (the config lives in your home directory):

```sh
garmr schedule set 09:00-17:00
garmr block add reddit.com youtube.com
garmr status
```

Or start from the sample config (YouTube, Reddit and Instagram, blocked 09:00–17:00):

```sh
mkdir -p ~/.config/garmr
cp config.example.json ~/.config/garmr/config.json
```

Then start the service for your user:

```sh
sudo systemctl enable --now garmr@$USER
```

`garmr status` should now show `runner: active`.

## Usage

```
garmr [flags] <command> [args]
```

| Command | What it does |
| --- | --- |
| `block add [-window HH:MM-HH:MM]... <site>...` | Add sites. URLs are accepted (`https://www.youtube.com/watch` → `youtube.com`). With `-window`, the site uses its own windows instead of the default; repeat `-window` for several. Running it again with `-window` on an existing site replaces its windows. |
| `block remove <site>...` | Remove sites. |
| `block list` | Show sites and their windows (`(default)` means the default schedule). |
| `schedule set HH:MM-HH:MM` | Set the default blocking window. |
| `schedule clear` | Remove the default window: sites without their own windows are then blocked all day. |
| `status` | Show the current time, the default schedule, whether the service is running, and which sites are blocked right now. |
| `run` | Enforce the schedule in the foreground, re-checking every minute. This is what the service runs. Needs root. |
| `apply` | Apply the schedule once for the current time. Needs root; refused while the service is running. |
| `clear` | Remove all garmr entries from `/etc/hosts`. Needs root; refused while the service is running. |

Global flags (put them before the command):

| Flag | Default | |
| --- | --- | --- |
| `-config` | `~/.config/garmr/config.json` | Config file. Under `sudo`, defaults to the invoking user's config. |
| `-hosts` | `/etc/hosts` | Hosts file to manage. Point it at a copy to experiment safely. |
| `-lock` | `/run/garmr.lock` | Lock file held by `run`. |
| `-version` | | Print the version. |

### Examples

```sh
# Block during work hours
garmr schedule set 09:00-17:00
garmr block add reddit.com news.ycombinator.com

# YouTube only in the morning and afternoon, with a free lunch hour
garmr block add -window 09:00-12:00 -window 13:00-17:00 youtube.com

# Block a site late at night (wraps past midnight)
garmr block add -window 22:00-06:00 twitter.com

# Block a site all day, regardless of the default
garmr block add -window 00:00-00:00 example.com

# Put a site back on the default schedule
garmr block remove youtube.com && garmr block add youtube.com
```

## Configuration

`~/.config/garmr/config.json` is written by the commands above, but you can
edit it by hand:

```json
{
  "schedule": { "start": "09:00", "end": "17:00" },
  "sites": [
    "reddit.com",
    {
      "domain": "youtube.com",
      "windows": [
        { "start": "09:00", "end": "12:00" },
        { "start": "13:00", "end": "17:00" }
      ]
    }
  ]
}
```

- A site written as a plain string follows the default `schedule`.
- A site with `windows` follows only its own windows.
- If there is no `schedule`, plain sites are blocked all day.

Window rules (local time, 24-hour `HH:MM`):

- The start is included, the end is not: `09:00-17:00` blocks from 09:00 up to 16:59.
- If the end is earlier than the start, the window runs past midnight: `22:00-06:00`.
- If start equals end, the window covers the whole day: `00:00-00:00`.

Blocking a domain also blocks its `www.` form. Subdomains are not covered:
add `m.youtube.com` etc. separately if needed.

## Running as a service

The unit is a template keyed by username, `garmr@<user>.service`, which runs
`garmr -config /home/<user>/.config/garmr/config.json run` as root.

```sh
sudo systemctl enable --now garmr@$USER   # start now and at boot
systemctl status garmr@$USER
journalctl -u garmr@$USER -f              # logs each time the blocked set changes
sudo systemctl stop garmr@$USER           # stop enforcing (current blocks remain)
```

- The service re-reads the config every minute, so no restart is needed after
  `block` or `schedule` changes.
- While it runs, `apply` and `clear` refuse to touch the hosts file. To remove
  all blocks: `sudo systemctl stop garmr@$USER && sudo garmr clear`.
- If the config file is invalid, the service logs an error and leaves the hosts
  file as it was.
- If your home directory is not `/home/<user>`, edit the `ExecStart` line with
  `sudo systemctl edit --full garmr@.service`.

## Troubleshooting

**A blocked site still loads.**

1. Check garmr's view: `garmr status` should show it `BLOCKED`.
2. Check the system's view: `getent hosts youtube.com` should print `0.0.0.0` / `::`.
3. If both look right, the browser is bypassing `/etc/hosts`:
   - Disable secure DNS / DNS-over-HTTPS (Chrome: *Settings → Privacy and security → Security → Use secure DNS*; Firefox: *Settings → Privacy & Security → DNS over HTTPS → Off*).
   - Restart the browser to drop its DNS cache.
4. The site may be served from a subdomain (e.g. `m.`, `old.`); add it too.

**`permission denied (try running with sudo)`.** `run`, `apply` and `clear`
write `/etc/hosts` and need root. `block`, `schedule` and `status` do not.

**`'garmr run' is active and manages the hosts file`.** The service is running.
Change the config instead, or stop the service first.

## Uninstall

```sh
sudo make uninstall
```

This stops and disables all `garmr@` services, removes garmr's entries from
`/etc/hosts`, and deletes the installed files. Your config in
`~/.config/garmr/` is left in place.
