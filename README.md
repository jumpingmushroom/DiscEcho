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

### TMDB

DVD identification queries TMDB (https://www.themoviedb.org/). To enable
auto-identification:

1. Create a free TMDB account → settings → API → request a v3 key
2. Set `DISCECHO_TMDB_KEY=<your-key>` in your `.env`
3. Optional: `DISCECHO_TMDB_LANG=en-US` (or any TMDB-supported locale)

If the key is unset, identification returns empty candidates and the UI
prompts for manual title entry — the daemon still starts and other
pipelines (audio CD) work normally.

### BDMV / UHD setup

DiscEcho's Blu-ray (BDMV) and Ultra HD Blu-ray pipelines decrypt and
demux discs with [MakeMKV](https://www.makemkv.com/). Audio CD and DVD
work without any of the setup below; the rest is opt-in.

**MakeMKV beta key (BDMV + UHD).** MakeMKV needs a registration key.
While the project is in beta the author posts a public key that
refreshes roughly monthly:

- Forum thread: <https://forum.makemkv.com/forum/viewtopic.php?t=1053>

Set the env var on the daemon:

```env
DISCECHO_MAKEMKV_BETA_KEY=T-<rest-of-key>
```

DiscEcho writes `${DISCECHO_DATA}/MakeMKV/settings.conf` on each start.
Refresh the env var (and restart the container) when the public key
rotates. Symptom of an expired key: BDMV/UHD jobs fail at the rip step
with "registration key expired" in the logs. If the env var is unset
the daemon still starts; only BDMV/UHD jobs will fail.

**AACS2 keys (UHD only).** UHD-Blu-ray discs are encrypted with AACS2.
MakeMKV needs a `KEYDB.cfg` to decrypt. **DiscEcho does not ship
`KEYDB.cfg` and does not link to sources for it.** Sourcing one is
your responsibility and may be restricted in your jurisdiction.

Drop your `KEYDB.cfg` at:

```
${DISCECHO_DATA}/MakeMKV/KEYDB.cfg
```

If a UHD disc is inserted and `KEYDB.cfg` is missing, the job fails
fast at the identify step with a clear error before any disc read.
Regular BDMV (Blu-ray) discs do not need this file.

### Game-disc setup (PSX / PS2 / Saturn / Dreamcast / Xbox)

DiscEcho's game-disc pipelines use
[redumper](https://github.com/superg/redumper) for ripping and
[chdman](https://github.com/mamedev/mame) (from MAME tools) for CHD
compression where applicable. They match each disc against
[Redump](http://redump.org/) for identification.

Drop Redump dat files under per-system subdirectories of
`${DISCECHO_DATA}/redump/`:

```
${DISCECHO_DATA}/redump/psx/Sony - PlayStation - Datfile (*.dat)
${DISCECHO_DATA}/redump/ps2/Sony - PlayStation 2 - Datfile (*.dat)
${DISCECHO_DATA}/redump/saturn/Sega - Saturn - Datfile (*.dat)
${DISCECHO_DATA}/redump/dreamcast/Sega - Dreamcast - Datfile (*.dat)
${DISCECHO_DATA}/redump/xbox/Microsoft - Xbox - Datfile (*.dat)
```

Sourced from <http://redump.org/downloads/>. Refresh manually as
Redump adds new dumps. **DiscEcho does not auto-download or
redistribute these files.**

The daemon walks `${DISCECHO_DATA}/redump/<system>/*.dat` at startup
and merges every dat-file into one in-memory index. Subdirectories
are optional — a Saturn user without a Dreamcast dat just doesn't
get DC matches. **NOTE:** dat files placed directly under
`${DISCECHO_DATA}/redump/` (without a subdirectory) are no longer
loaded; move them into the right per-system subfolder.

If a dat is missing, that disc-type's auto-identification falls back
to a placeholder title via the new-disc sheet (Dreamcast is
post-rip-identified by MD5; the others fall back to manual search).
The daemon still starts; only auto-ID is affected.

Disc detection is automatic:

- PSX/PS2: classifier reads `/SYSTEM.CNF` and parses the `BOOT[2]=`
  line to distinguish them.
- Saturn: raw sector 0 magic `SEGA SEGASATURN` + product number
  from IP.BIN.
- Dreamcast: multi-session TOC heuristic (two sessions with session
  2 starting at LBA ≥ 45000); identification is post-rip via
  MD5 against the DC dat.
- Xbox: `/default.xbe` at the disc root + XBE certificate title ID.
  Original Xbox only — Xbox 360 (XGD2/3) requires Kreon-flashed
  drive firmware and is out of scope.

### Raw-data discs

Anything the classifier doesn't recognise (data CDs, data DVDs,
unrecognised game discs) routes to the `Data` pipeline: a straight
`dd` rip to ISO with `conv=noerror,sync` (bad sectors are zero-filled
rather than aborting). The disc filesystem's volume label becomes
the title; falls back to `data-disc-YYYYMMDD-HHMMSS` when no label
is present. SHA-256 of the produced ISO and total byte count are
stored on the disc record for verification later.

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
