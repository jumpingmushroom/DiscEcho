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
- **M3.2 mobile multi-drive UI polish.**
  - `JobRow` shows a `drive_id` chip after the title, so with two
    drives running concurrently you can tell which row belongs to
    which drive.
  - Jobs in `queued` state render a `QUEUED` badge instead of the
    misleading `0%` placeholder. ETA hidden in this state.
  - `DriveCard` gains a `+N queued` pill (next to the state label)
    when more than one job targets the same drive.
  - Dashboard hero-band caption decomposes when both rip and queue
    counts are non-zero — e.g. `1 rip · 2 queued` — instead of a
    plain `3 jobs` that flattens the multi-drive picture.
  - Active-queue list stably sorts running jobs above queued ones.
  - No daemon changes; no wire-type changes. The orchestrator's
    per-drive worker model from M1.1 already supports this; M3.2
    just surfaces it.
- **M4.1 desktop shell + dashboard.**
  - `+layout.svelte` mounts a desktop top-nav (`TopNav.svelte`) visible
    only at the `lg:` breakpoint (1024px) and up. Brand mark on the
    left, four section links centred (`Dashboard / History / Profiles
    / System`), `LiveDot` on the right. Active section detected via
    `$page.url.pathname`.
  - `+page.svelte` becomes a viewport-driven dispatcher that picks
    `MobileDashboard` (verbatim move of the M3.2 content) below `lg:`
    and the new `DesktopDashboard` above. Mobile dashboard behaviour
    unchanged.
  - `DesktopDashboard` composes a hero band of compact `DriveHeroCard`
    components (CSS grid auto-fit, 1-3 drives), a `QueueTable` (real
    `<table>` with type/title/drv/step/pct/ETA columns), and a sticky
    `JobDetailPanel` with `PipelineStepperHorizontal` + 12-line log
    tail.
  - New `selectedJobID` writable in the store. Default `null`;
    `DesktopDashboard` falls back to the first running job when nothing
    is explicitly selected. Click queue row or hero drive card to set.
  - `/profiles` and `/system` get placeholder pages so the new top-nav
    links land somewhere instead of 404'ing. Their content stays
    mobile-style on desktop until M4.2 polishes them desktop-native.
  - No daemon changes; no wire-type changes. M4.2 ships keyboard
    shortcuts, ⌘K palette, and desktop-native History/Profiles/System.
- **M5.1 daemon-side PSX + PS2 game-disc pipelines.**
  - `pipelines/psx` handler: identifies via SYSTEM.CNF boot code →
    Redump dat lookup, rips with redumper (`cd` mode) producing
    `.bin/.cue`, MD5-verifies against the dat entry (warns on
    mismatch but doesn't abort), compresses to CHD with chdman,
    atomic-moves to library. Output template
    `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`.
  - `pipelines/ps2` handler: same shape but redumper `dvd` mode
    producing `.iso`; chdman auto-picks `createdvd` from the
    extension.
  - `tools/redumper.go`: thin wrapper with `Rip(devPath, outDir,
    name, mode, sink)` and a pure progress-line parser
    (`ParseRedumperProgress`).
  - `tools/chdman.go`: thin wrapper with `CreateCHD(input, output,
    sink)` that auto-picks `createcd` (.cue) vs `createdvd` (.iso).
  - `identify/redump.go`: Redump dat-file XML loader with
    boot-code + MD5 indexes.
  - `identify/redumpprobe.go`: SYSTEM.CNF reader (via `isoinfo -x`)
    with PSX/PS2 discrimination via the `BOOT` vs `BOOT2` prefix.
    Normalises PS2's 5-digit boot code (`SCES_50051`) to the dotted
    form (`SCES_500.51`) so it matches Redump dat-file keys.
  - Classifier extended with a fourth probe — when fs listing shows
    `/SYSTEM.CNF`, the classifier reads it and routes to PSX or PS2.
    Falls back to DATA when SYSTEM.CNF is unreadable (conservative,
    same posture as M3.1's `bd_info`-fails-default-to-BDMV).
  - Two new seeded profiles: `PSX-CHD` (PSX) and `PS2-CHD` (PS2),
    both with engine `redumper+chdman` and the CHD output template.
  - `state.Candidate` gains an optional `region` field; wire type
    mirrored. `pipelines.OutputFields` gains `Region` for templates.
  - New env vars: `DISCECHO_REDUMPER_BIN` (default `redumper`),
    `DISCECHO_CHDMAN_BIN` (default `chdman`), `DISCECHO_REDUMP_DIR`
    (default `${DISCECHO_DATA}/redump`).
  - Runtime image gains `mame-tools` (chdman, ~2 MB) and a
    pre-built static `redumper` binary from the GitHub releases
    page (~3 MB, `b720` build, `linux-x64.zip`). No build-from-source.
  - README documents the user-supplied Redump dat-file workflow.
- **M5.2 profile editor + API mutations.**
  - Daemon: `POST/PUT/DELETE /api/profiles` with engine-aware
    schema validation (whipper/MakeMKV/MakeMKV+HandBrake/HandBrake/
    redumper+chdman) and `text/template`-checked output paths.
    422 responses use a flat `{field: msg}` body. Foreign-key
    references from active jobs return 409.
  - Daemon broadcasts `profile.changed` SSE events on every
    create/update/delete (payload `{profile}` for upsert,
    `{profile_id, deleted: true}` for delete).
  - WebUI desktop `/profiles`: two-column editor (list + form)
    with engine-locked editing in edit mode, format restricted
    to the engine's schema, options grid driven by the schema,
    two-step delete, and inline 422 field errors.
  - WebUI mobile `/profiles`: read-only list grouped by disc
    type with an "edit on desktop" hint, replacing the M4.1
    placeholder.
  - New store imperatives: `createProfile`, `updateProfile`,
    `deleteProfile`. New `selectedProfileID` writable.
  - New `lib/api.ts` helper `parseValidationErrors(e)` and
    `apiPut<T>` mirroring `apiPost`.

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.1.0] - YYYY-MM-DD

### Added
- Initial project scaffold.