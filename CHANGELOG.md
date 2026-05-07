# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial repo skeleton: `daemon/`, `webui/`, `shared/`.
- Go daemon serving `GET /api/health` and `GET /api/version` on `:8088`.
- Linux udev subscription that logs disc-insert / disc-remove events on
  `/dev/sr*`; non-Linux builds compile with a no-op stub.
- SvelteKit 2 web UI scaffold (Tailwind 3, dark-only design tokens,
  static adapter) embedded into the daemon binary via `//go:embed`.
- Multi-stage Dockerfile (Go + Node build → `python:3.12-slim` runtime
  with Apprise pre-installed for M1).
- `docker-compose.yml` with `/dev/sr0` passthrough and bind-mounted
  library + data directories.
- GitHub Actions CI: gofmt, go vet, golangci-lint, `go test -race`,
  Prettier check, `svelte-check`, `pnpm build`, Docker image build.
- GitHub Actions release: container build and push to GHCR on `v*` tags.
- PR template enforcing conventional-commit prefixes and verification
  checklist.
- **M1.1 daemon-side audio CD pipeline (backend only).**
  - `state` package: SQLite (modernc.org/sqlite, pure-Go, WAL) + 8-table
    schema (drives, discs, profiles, jobs, job_steps, log_lines,
    notifications, settings) with hand-rolled migration runner; typed
    Store CRUD; in-process Broadcaster with slow-subscriber drop.
  - `identify` package: TOC reader via `cdparanoia -Q`; MusicBrainz
    disc-ID hashing (verified against the official algorithm);
    MusicBrainz REST client with 1 req/sec rate limit; disc-type
    classifier via `cd-info`.
  - `tools` package: Tool/Sink/Registry interfaces; Whipper subprocess
    wrapper with stdout progress parser; Apprise CLI wrapper (failures
    are warned, never fail the job); Eject wrapper; MockTool for tests.
  - `pipelines` package: Handler interface, Registry, output templating
    (Go templates with path-traversal sanitization), atomic move with
    cross-filesystem fallback, write probe, RecordingSink for tests.
  - `pipelines/audiocd`: Handler implementation that ties it all
    together — Identify (TOC + MB lookup) → Plan (8-step with
    transcode+compress skipped) → Run (whipper rip → atomic move →
    Apprise → eject).
  - `jobs` package: PersistentSink (bridges pipeline events to SQLite +
    Broadcaster, with 1Hz progress coalescing); Orchestrator with
    per-drive serialization, ctx cancellation, and crash recovery
    (interrupted state on startup).
  - `api` package: bearer-token middleware (auto-generated to
    `${DISCECHO_DATA}/token` on first start); REST endpoints for
    `/api/state`, `/api/drives`, `/api/jobs`, `/api/discs/:id/start`,
    `/api/profiles`, `/api/settings`; SSE stream at `/api/events` with
    `state.snapshot` bootstrap and 9 event types.
  - `settings` package: env loader, token bootstrap, profile + Apprise
    URL seeding from `${DISCECHO_APPRISE_URLS}`.
  - `drive.InitialScan`: enumerates `/dev/sr*` on startup and upserts
    drives.
  - `discflow` (in main): wires udev events → classifier → handler.Identify
    → store + broadcaster.
  - Runtime image now includes `whipper`, `cdparanoia`, `libcdio-utils`
    (image grows from 156 MB to ~291 MB).
  - `shared/wire.ts`: TypeScript types mirroring the JSON wire format
    for the M1.2 mobile UI to consume.
- Go module bumped to 1.25 (transitive requirement of
  `modernc.org/sqlite`); Dockerfile and CI workflow aligned.
- **M1.2 mobile UI (audio CD pipeline).**
  - SvelteKit 2 mobile-first PWA: dashboard, full-screen job detail with
    vertical pipeline stepper, new-disc bottom sheet with 8s
    auto-confirm.
  - Stub pages for History (M2) and Settings (M6); tab bar always
    visible.
  - Global Svelte store hydrating from `GET /api/state` and merging
    SSE deltas (`drive.changed`, `disc.detected/identified`, `job.*`,
    `state.snapshot` reconnect). Per-job log ring buffer capped at 50
    lines.
  - vitest + @testing-library/svelte unit tests for the store, fetch
    helpers, SSE wrapper, time helpers, and the disc-id sheet's
    countdown logic.
  - PWA manifest + 192/512 icons (offline shell + push deferred to M6).
  - Daemon: new `DISCECHO_AUTH_DISABLED=true` env to skip the bearer
    token bootstrap, intended for use behind a reverse proxy.
- **M2.1 daemon-side DVD-Video pipeline + history endpoint.**
  - `pipelines/dvdvideo`: handler that probes DVD volume label, queries
    TMDB (movie + tv search merged), runs HandBrake scan + per-title
    encode, atomic-moves outputs to library, fires Apprise, ejects.
  - DVD-Movie profile (MP4, x264 RF 20, longest title only) and
    DVD-Series profile (MKV, x264 RF 20 per-title, titles ≥ 5 min)
    seeded on first start.
  - `identify/tmdb.go`: TMDB JSON client with parallel movie+tv search,
    confidence from `popularity / 10`, capped at 5 candidates.
  - `identify/dvdlabel.go`: volume label normaliser (replaces `_` and
    `.`, title-cases via `golang.org/x/text/cases`, rejects `DVD_VIDEO`
    / ≤ 3 chars).
  - `identify/dvdprober.go`: shells out to `isoinfo -d` to read the
    primary volume descriptor.
  - `tools/handbrake.go`: HandBrakeCLI wrapper with `--scan` title
    parser and encode-progress parser (computes overall progress
    across multiple titles via `HB_TITLE_IDX` / `HB_TOTAL_TITLES` env).
  - `state.Candidate` gains `tmdb_id` and `media_type` fields
    (additive; no migration).
  - `state.Store.ListHistory` + `CountHistory` + `HistoryFilter` /
    `HistoryRow` types; `GET /api/history` endpoint paginated by
    finished_at DESC, filterable by type/from/to.
  - `POST /api/discs/:id/identify` accepts `{query, media_type}` body
    for manual TMDB lookup; persists candidates back onto the disc.
  - New env vars: `DISCECHO_TMDB_KEY`, `DISCECHO_TMDB_LANG` (default
    `en-US`), `DISCECHO_SUBS_LANG` (default `eng`),
    `DISCECHO_HANDBRAKE_BIN`, `DISCECHO_ISOINFO_BIN`.
  - Runtime image now includes `handbrake-cli`, `libdvd-pkg` (CSS
    bypass), and `genisoimage` (`isoinfo`).
- **M2.2 mobile UI for DVD pipeline + history.**
  - `/history` page replaces the M1.2 stub: day-grouped list
    (Today / Yesterday / N days ago / N weeks ago / absolute date past
    30 days), disc-type filter chips (All / Audio CD / DVD), infinite
    scroll via IntersectionObserver, paginated by
    `GET /api/history?limit=50`.
  - DiscIdSheet candidate-driven profile binding: each candidate
    carries `media_type`; clicking a candidate auto-binds the matching
    profile (`movie` → DVD-Movie, `tv` → DVD-Series, audio → CD-FLAC).
    Each TMDB candidate row shows a `FILM` / `TV` badge.
  - "Search manually" wired to `POST /api/discs/:id/identify`: inline
    text input replaces the candidate list while searching; success
    refreshes the list, empty result shows "No matches found", HTTP
    error shows the status.
  - DiscIdSheet now reads its disc reactively from the global `discs`
    store, so manual-identify updates flow through without remounting.
  - New components: `HistoryRow.svelte`, `FilterChips.svelte`.
  - Store additions: `history`, `historyTotal`, `historyLoading`,
    `historyError`, `historyFilter` writables; `fetchHistoryPage` and
    `manualIdentify` imperatives.
  - `lib/time.ts` gains `dayGroupLabel` for the history grouping.
- **M3.1 daemon-side BDMV + UHD pipelines.**
  - `pipelines/bdmv` handler: identifies via volume label → TMDB,
    selects longest title with duration ≥ `min_title_seconds`, uses
    MakeMKV to decrypt+demux that title, then HandBrake transcodes
    to MKV (`x265 RF 19 10-bit`), atomic-moves into the library,
    fires Apprise, ejects.
  - `pipelines/uhd` handler: same identify shape but with an AACS2
    key-file precheck *before* TMDB. UHD-Remux skips the transcode
    and compress steps — the MakeMKV output is the artifact, with
    HDR10/Dolby Vision metadata, lossless audio, and PGS subtitles
    preserved. Output lands at
    `Title (Year)/Title (Year) [UHD].mkv`.
  - `tools/makemkv.go`: `makemkvcon --robot` wrapper with `Scan`
    (info parser → titles + tracks) and `Rip` (decrypt+demux of one
    title to a directory). Progress and operation labels stream to
    the sink.
  - `identify/bdprober.go`: `bd_info` wrapper for AACS2 detection.
  - `identify/fsprobe.go`: `isoinfo -R -l` listing parser, used by
    the classifier to tell DVD apart from BDMV.
  - `identify/bdmt.go`: BDMT_<lang>.xml title parser. Currently used
    only in tests; production identify reads the volume label.
    Off-disc XML extraction lands in a follow-up.
  - Classifier rewritten to a three-step probe (`cd-info` → fs
    listing → `bd_info`). Routes `AUDIO_CD`, `DVD`, `BDMV`, `UHD`,
    `DATA` to the right handler. **Fixes a latent gap from M2.1**
    where the DVD handler was registered but never reachable via
    the disc-flow because the classifier returned `DATA` for all
    non-audio discs.
  - Two new profiles seeded on first start: `BD-1080p` (BDMV) and
    `UHD-Remux` (UHD).
  - New env vars: `DISCECHO_MAKEMKV_BIN` (default `makemkvcon`),
    `DISCECHO_MAKEMKV_DATA` (default `${DISCECHO_DATA}/MakeMKV`),
    `DISCECHO_MAKEMKV_BETA_KEY` (optional public beta key — daemon
    writes `~/.MakeMKV/settings.conf` on start), `DISCECHO_BDINFO_BIN`
    (default `bd_info`).
  - Runtime image now includes `libbluray-bin` (Debian package
    that ships `bd_info`) and `makemkvcon` built from source. MakeMKV
    is compiled in a separate build stage and only its binary +
    shared libs (`libmakemkv.so.1`, `libdriveio.so.0`) are copied
    into the runtime image; build deps (qtbase5-dev, mesa-dev, etc.)
    don't ship. Runtime grew from ~291 MB (M2.1) to ~1 GB (M3.1)
    — the bulk is `libavcodec59` + transitive media-codec deps that
    `makemkvcon` links against. Image-size trim is a future task.
  - README documents the MakeMKV beta-key refresh cadence and
    where to drop `KEYDB.cfg` for UHD.

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.1.0] - YYYY-MM-DD

### Added
- Initial project scaffold.