# PADS — Predictive Adaptive Download Scheduler

PADS is a Go CLI download manager with segmented downloads, adaptive connection scaling, server probing, resume support, and queue orchestration.

## Status

Phase E (persistence) is complete, and a background daemon now owns long-running
downloads so `status`, `pause`, and `resume` work across processes. Remaining
Phase F work: ADRs, benchmarks, `pads schedule`, and raising test coverage.

A browser extension in `extension/` hands downloads to the daemon through a
native-messaging bridge; it runs in Firefox, Chrome, Brave, Chromium, and Edge.

## Requirements

- Go 1.26 or later

## Build

```bash
go build -o pads .
```

Or:

```bash
make build
```

## Usage

```bash
pads get <url>
pads queue add <url>
pads queue list
pads queue start
pads queue clear
pads queue remove <id>
pads resume <id>
pads pause <id>
pads status
pads schedule <url> --at "HH:MM"
pads config set <key> <value>
pads config show
pads config reset

pads daemon run [--addr 127.0.0.1:0]
pads daemon status
pads daemon stop

pads nativehost install [--browser chrome,brave,firefox,...]
pads nativehost uninstall
```

### Daemon

Without a daemon, a download lives and dies with the `pads get` process that
started it, so `pads status` in another terminal has nothing to report and
`pads pause` can only mark a saved state as paused.

Run `pads daemon run` and the daemon owns downloads instead: `status`, `pause`,
and `resume` detect it automatically and act on the running transfers. The
daemon listens on loopback only, authenticates every request with a bearer token
written to `~/.pads/daemon.json` (mode 0600), and refuses requests that carry a
browser `Origin` header. It holds `~/.pads/daemon.lock` while running; a leftover
lock is never cleared automatically, because doing so would mean guessing that
another daemon has died.

`pads get` still downloads in the foreground so it can draw a progress bar.

### Browser extension

`extension/` is a WebExtension that hands the browser's downloads to the daemon.
It runs in Firefox and in the Chromium family: Chrome, Brave, Chromium, and
Edge. It reaches the daemon through `pads nativehost`, a stdio bridge the
browser launches itself, so the bearer token never enters the browser and the
daemon's refusal of browser-origin requests stays intact.

```bash
make extension
pads nativehost install
pads daemon run
```

Then load `dist/extension/chrome` as an unpacked extension, or
`dist/extension/firefox/manifest.json` as a temporary add-on. See
[extension/README.md](extension/README.md) for per-browser setup and the opt-in
permission that lets downloads behind a login carry your browser session.

## Configuration

Configuration is stored at `~/.pads/config.json`. State, temp files, and queue
data live under `~/.pads/`.

Downloads started from the browser land in `download_dir`, which defaults to
`~/Downloads`:

```bash
pads config set download_dir /data/downloads
```

## Development

```bash
make test
make test-race
make extension        # build dist/extension/{chrome,firefox}
make extension-check  # load each background entry under stub APIs
```

## License

MIT — see [LICENSE](LICENSE).
