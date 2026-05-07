# DiscEcho

Self-hosted optical-disc archival service for the homelab. Watches
optical drives, classifies inserted discs, runs per-disc-type rip →
transcode → tag → move pipelines, and exposes a mobile-first web UI for
live status and history.

> Status: **M1** — audio CD pipeline shipping (M1.1 daemon, M1.2 mobile
> UI). See [`ROADMAP.md`](./ROADMAP.md).

## Quick start

```bash
git clone https://github.com/jumpingmushroom/DiscEcho.git
cd DiscEcho
cp .env.example .env
# edit DISCECHO_LIBRARY_PATH and CDROM_GID for your host
docker compose up -d --build
curl http://localhost:8088/api/health   # → {"ok":true}
```

Open `http://localhost:8088/` on your phone (or laptop in mobile
viewport) for the dashboard.

### Auth

By default the daemon generates a bearer token on first start and writes
it to `${DISCECHO_DATA}/token`. The mobile UI does **not** send this
token, so for production deployment put DiscEcho behind a reverse proxy
that handles auth (Tailscale Funnel, Caddy basic auth, etc.) and set
`DISCECHO_AUTH_DISABLED=true` in your `.env` to skip the daemon-side
token entirely.

For `curl`-only automation, leave the env unset and pass the token from
`${DISCECHO_DATA}/token` as `Authorization: Bearer <token>`.

## Dev setup

You need:

- Go 1.24+
- Node 20 LTS, pnpm 9
- Docker with BuildKit
- Linux host with at least one optical drive at `/dev/sr0` (manual
  testing of disc detection is Linux-only; macOS / Windows can build and
  unit-test)

Local loop:

```bash
# 1. Build the UI (run once, then re-run after UI changes)
cd webui && pnpm install && pnpm build && cd ..
rm -rf daemon/embed/webui_build && cp -r webui/build daemon/embed/webui_build

# 2. Run the daemon
cd daemon && go run ./cmd/discecho

# 3. (separate shell) UI dev server with hot reload, proxies /api → :8088
cd webui && pnpm dev    # opens http://localhost:5173
```

Tests:

```bash
cd daemon && go test ./... -race
cd webui  && pnpm check
```

## Layout

| Path                  | Purpose                                             |
|-----------------------|-----------------------------------------------------|
| `daemon/`             | Go service: HTTP API, udev, pipelines (M1+)         |
| `webui/`              | SvelteKit dark-only PWA + desktop dashboard         |
| `shared/`             | Wire-format types (filled in at M1)                 |
| `Dockerfile`          | Multi-stage build → `python:3.12-slim` runtime      |
| `docker-compose.yml`  | Single-service homelab deploy                       |

See [`ARCHITECTURE.md`](./ARCHITECTURE.md) for the full system shape and
[`ROADMAP.md`](./ROADMAP.md) for milestone planning.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md). Open opinions on
[`OPEN_QUESTIONS.md`](./OPEN_QUESTIONS.md) are welcome — answers in PRs
count as contributions.

## License

> TODO: pin a license. Tracked in `OPEN_QUESTIONS.md`.
