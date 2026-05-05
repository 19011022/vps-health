# vps-health

Modern TUI health check for Ubuntu VPSes running Docker workloads. Single
static binary, no runtime deps. Built with [Bubble Tea] + [Lip Gloss].

```
$ vps-health
```

Tells you in one screen: CPU/load, memory, swap, disk, inodes, top mounts and
top dirs, Docker (containers + reclaimable space + top-by-CPU/mem), top
processes, zombies, journal size, OOM kills (24h), failed systemd units,
reboot-required flag, FD usage. Then a clear OK / WARN / BAD verdict with
reasons and suggested actions.

## Install

### One-liner (recommended)

```bash
curl -fsSL https://get.ottomind.ai/vh | sudo bash
```

Pin a specific version:

```bash
curl -fsSL https://get.ottomind.ai/vh/0.1.0 | sudo bash
# or
VPS_HEALTH_VERSION=0.1.0 curl -fsSL https://get.ottomind.ai/vh | sudo bash
```

### Direct from GitHub

```bash
curl -fsSL https://raw.githubusercontent.com/19011022/vps-health/main/install.sh | sudo bash
```

### Build from source (no GitHub Release needed)

```bash
git clone https://github.com/19011022/vps-health
cd vps-health
./install-from-source.sh
```

## Usage

```
vps-health             # interactive TUI: q quit, r refresh, ↑↓/j/k scroll, space pgdn
vps-health --plain     # styled but non-interactive (auto-selected when piped)
vps-health --no-color
vps-health --version
vh                     # alias
```

When stdout is **not** a TTY (cron, pipes, redirects), the binary
auto-falls-back to `--plain`.

### Exit codes (plain mode)

- `0` everything OK
- `1` at least one WARN
- `2` at least one BAD

```bash
vps-health --plain >/var/log/vps-health.log || \
  curl -fsS https://hooks.slack.com/... -d "vps unhealthy on $(hostname)"
```

## Thresholds

All in one place at the top of `collect.go`:

| Resource | OK | WARN | BAD |
|---|---|---|---|
| CPU load (% of cores, 1m) | <70 | 70–100 | ≥100 |
| RAM available | >20% | 10–20% | ≤10% |
| Swap in use | <512 MB | ≥512 MB | ≥1 GB |
| Disk root | <75% | 75–90% | ≥90% |
| Inodes | <85% | 85–95% | ≥95% |
| File descriptors | <70% | 70–90% | ≥90% |
| OOM kills (24h) | 0 | ≥1 | ≥1 + tight RAM |
| Failed systemd units | 0 | ≥1 | – |

## Distribution flow

```
   developer push tag v1.2.3
            │
            ▼
   ┌───────────────────┐    builds linux/amd64,
   │ GitHub Actions    │    linux/arm64, darwin/amd64,
   │ release.yml       │    darwin/arm64 + SHA256SUMS
   └────────┬──────────┘
            │ uploads
            ▼
   ┌───────────────────┐
   │ GitHub Releases   │  ← canonical binary host (CDN-backed)
   └────────┬──────────┘
            │ raw url to install.sh
            ▼
   ┌───────────────────┐
   │ install.sh        │  detects OS/arch, downloads asset,
   │ (in repo, main)   │  verifies sha256, drops in /usr/local/bin
   └────────┬──────────┘
            │ proxied by
            ▼
   ┌───────────────────┐
   │ get.ottomind.ai/vh    │  Cloudflare Worker → install.sh
   │ (Cloudflare WK)   │  with optional version pin /vh/0.1.0
   └───────────────────┘
            │
            ▼
        sudo bash
```

## Cutting a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds the four binaries, generates `SHA256SUMS`, and publishes
a Release. After ~2 minutes, `curl -fsSL https://get.ottomind.ai/vh | sudo bash`
on any VPS pulls v0.1.0.

For the `get.ottomind.ai/vh` route to work the first time:

```bash
cd cloudflare
wrangler deploy
```

(See [`cloudflare/README.md`](cloudflare/README.md).)

## Layout

```
.
├── .github/workflows/
│   ├── ci.yml              # vet + gofmt + build on every push/PR
│   └── release.yml         # cross-arch build + GitHub Release on v* tag
├── cloudflare/
│   ├── README.md           # how to deploy the Worker
│   ├── worker.js           # get.ottomind.ai/vh → install.sh proxy
│   └── wrangler.toml
├── go.mod
├── install.sh              # curl|bash entrypoint (downloads from Releases)
├── install-from-source.sh  # git clone + go build (no Release needed)
├── README.md
├── main.go                 # entry, flags, plain/TUI dispatch
├── types.go                # Report data types
├── collect.go              # /proc, df, ps, docker, journalctl probes
├── decide.go               # OK/WARN/BAD logic
├── styles.go               # lipgloss styles + bars + badges
├── render.go               # Report → string (used by both modes)
└── model.go                # Bubble Tea Model/Update/View
```

[Bubble Tea]: https://github.com/charmbracelet/bubbletea
[Lip Gloss]: https://github.com/charmbracelet/lipgloss
