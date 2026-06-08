# PADS — Predictive Adaptive Download Scheduler

PADS is a Go CLI download manager with segmented downloads, adaptive connection scaling, server probing, resume support, and queue orchestration.

## Status

Phase B (baseline downloader) is complete. `pads get <url>` downloads files with server probing, temp-file writing, atomic merge, and progress display. Segmented parallel downloads arrive in Phase C.

## Requirements

- Go 1.22 or later

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
```

## Configuration

Configuration is stored at `~/.pads/config.json`. State, temp files, and queue data live under `~/.pads/`.

## Development

```bash
make test
make test-race
```

## License

MIT — see [LICENSE](LICENSE).
