# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-05-12

### Fixed

- udev `DEVNAME` is now normalized when the event comes from `udevd`
  (full path form `/dev/sr0`) rather than the bare kernel form (`sr0`).
  Without this, disc-insert events on hosts using systemd-udevd / eudev
  produced `dev=/dev//dev/sr0` and `disc-flow: no drive registered`,
  so insertion never started the pipeline. Affects every deployment
  whose container receives events via the udev netlink group (group 2)
  — which is the default for the embedded `pilebones/go-udev`
  watcher.

## [0.1.0] - 2026-05-12

### Removed

- Appearance settings (accent / mood / density picker) on Settings.
  The theme is now fixed at `aurora` accent, `carbon` mood, `standard`
  density. The `prefs.accent`, `prefs.mood`, and `prefs.density`
  setting keys are no longer accepted by `PUT /api/settings`. Existing
  rows in the database are ignored.

### Added

- Settings → System tab now mirrors the original mockup: library-path
  editor, drives panel, external-connection status (TMDB configured /
  language, MusicBrainz endpoint, Apprise binary + version), and host
  info (kernel, CPU count, uptime, disk usage for `/library` and
  `/var/lib/discecho`).
- New read-only endpoints `GET /api/system/host` and
  `GET /api/system/integrations` back the System tab. The TMDB key
  itself is never returned — only a `configured` boolean.
- `library.path` is now an editable setting. The `DISCECHO_LIBRARY` env
  var seeds the value on first boot; subsequent edits in the UI win
  on next container restart (running pipelines capture the path at
  boot, so a restart is required for the change to apply).
- **Typed library roots.** Settings → System → Library paths now exposes
  five separate roots — `library.movies`, `library.tv`, `library.music`,
  `library.games`, `library.data` — each editable individually and
  surfaced via the new `library_roots` field on
  `GET /api/system/integrations`. New per-root env overrides
  `DISCECHO_LIBRARY_{MOVIES,TV,MUSIC,GAMES,DATA}` set defaults; stored
  values always win over env. Each pipeline now writes to its typed
  root: audio CD → music, DVD/BD/UHD → movies, PSX/PS2/Saturn/DC/Xbox
  → games, data → data. **Migration:** on upgrade, deployments with an
  existing `library.path` row see the five typed roots seeded as
  `<library.path>/<media>` so existing layouts keep working.
  DVD-Series profiles temporarily land under `library.movies` until the
  orchestrator can route series-typed jobs to `library.tv` (planned for
  the typed-encoding milestone).
- **`FormSection` / `FormRow` / `PathField` desktop primitives** ported
  from the original handoff bundle. Used by the rewritten System tab;
  available for the Profiles editor refactor in the next milestone.
- **Typed profile encoding fields.** Profiles now carry first-class
  `container`, `video_codec`, `quality_preset`, `hdr_pipeline`,
  `drive_policy`, and `auto_eject` columns instead of stuffing the
  same values into a flat `options` map. The Profiles editor is
  rebuilt around the original mockup's four sections — Engine
  (Reader / Drive policy / Auto-eject), Encoding (Container / Video
  codec / Quality preset / HDR pipeline), Post-processing (placeholder
  for the chain UX), and Library (Output path) — with a typed dropdown
  per field. The sidebar gains a `DiscTypeBadge` per row and an engine
  sub-line. The legacy `format` and `preset` fields are kept as a
  fallback for one release; new clients should write the typed fields
  directly. Validator errors come back keyed on the new field names
  (`container`, `video_codec`, `hdr_pipeline`, `drive_policy`).

### Deprecated

- `library.path` setting key. Writes still succeed for one release and
  fan out to the five typed `library.<media>` keys. Will be removed in
  a follow-up release; switch UIs and scripts to write the typed keys
  directly.
- Profile `format` and `preset` fields. The daemon still accepts and
  returns them for one release as a fallback when `container` /
  `quality_preset` are empty; both the migration and the editor mirror
  values into the typed columns. Will be removed in a follow-up
  release.

### Added — connections list

- Settings → System → **API keys & connections** is now a structured
  list: TMDB, MusicBrainz, redump, Apprise. Each row shows a status
  pill (`connected` / `not configured` / `error: …`), an optional
  detail (TMDB language, MusicBrainz endpoint, Apprise version), and
  an Edit button. Apprise's Edit scrolls to the Notifications section;
  the others surface the env var to set in `.env`.
- `GET /api/system/integrations` gains an `items: IntegrationStatus[]`
  field driving the new list. The legacy flat `tmdb` / `musicbrainz` /
  `apprise` objects are kept alongside for one release so older
  clients (mobile read-only view) keep working.
- New webui primitive: `ApiRow.svelte`. Mirrors the original handoff's
  `ApiRow` from `desktop.jsx`.

### Fixed

- SSE stream `/api/events` is no longer killed every 30 s by the global
  request-timeout middleware; the route now bypasses the timeout and
  emits a `: ping` keepalive comment every 15 s so reverse-proxy idle
  timeouts don't drop the connection either.
- `POST /api/discs/{id}/start` now persists the user-selected
  candidate's identity (title / year / provider / metadata id) and
  picks `TMDBID` for movie/TV candidates instead of the empty `MBID`.
  Previously the manual choice was silently dropped before the
  orchestrator re-read the disc row.
- Bearer-token comparison in the auth middleware is now constant-time.
- Apprise CLI invocations (`DryRun`, `Send`, `BuildAppriseArgs`) now
  reject URLs that begin with `-` and place a `--` separator before
  positional URL arguments so a malformed URL cannot smuggle CLI flags
  into the apprise process.
- Orchestrator `Close` no longer races `Submit` into a closed-channel
  panic on shutdown; per-drive queues are no longer closed and workers
  exit via `o.stopped` instead.

### Removed

- Dropped the unimplemented `POST /api/jobs/{id}/pause` endpoint (it
  was a 501 placeholder).
- Deleted the dead, drift-prone `shared/wire.ts` — `webui/src/lib/wire.ts`
  is the sole frontend wire-types source.
- Auto-generated bearer token at `<DATA>/token`. The token is now
  sourced exclusively from the `DISCECHO_TOKEN` env var; no on-disk
  persistence, no auto-generation. Existing token files are left
  untouched but ignored on startup. To migrate: copy the value from
  the old `<DATA>/token` into a `DISCECHO_TOKEN` env var on the
  daemon, then delete the on-disk file.
- `DISCECHO_AUTH_DISABLED` env var. The default is now "auth off";
  opt back in by setting `DISCECHO_TOKEN`. To migrate: drop
  `DISCECHO_AUTH_DISABLED=true` from your environment — the daemon
  now behaves the same way without it.

### Changed

- Replaced the hand-rolled bubble sort in `tools/makemkv.go` with
  `slices.Sort`.
- `daemon/cmd/discecho/main.go` extracts a single `urlsForTrigger`
  closure shared by every pipeline instead of re-defining it per
  registration.
- The daemon no longer logs a partial token at startup; it logs only
  whether the bearer token is configured.
- Daemon defaults to no authentication when `DISCECHO_TOKEN` is unset.
  This matches the homelab/LAN deployment model the project is
  designed around: the embedded UI works on first install with zero
  config. Set `DISCECHO_TOKEN` to enable bearer auth for proxy or
  exposed deployments.

### Added

- Initial repo skeleton: `daemon/`, `webui/`.
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
- **M6.1 daemon-side Saturn / Dreamcast / Xbox / Data pipelines.**
  - `pipelines/saturn` handler: identifies via Saturn IP.BIN
    (sector 0 magic `SEGA SEGASATURN` + product number) → Redump
    dat lookup, rips with redumper (`cd` mode), MD5-verifies
    against the dat entry (warns on mismatch), compresses to CHD
    with chdman, atomic-moves to library. Output template
    `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`.
  - `pipelines/dreamcast` handler: type-only classification
    pre-rip via TOC heuristic (2 sessions + session-2 start at
    LBA ≥ 45000). Rips with redumper (`cd` mode), produces
    multi-track GDI, computes post-rip MD5 against Redump DC
    dat — on hit, fills in real `Title` and `Region` before the
    move step. Compresses to CHD-GDI via chdman.
  - `pipelines/xbox` handler: identifies via XBE certificate
    from `default.xbe` on the mounted disc (title ID +
    allowed-region bitfield) → Redump dat lookup, rips with
    redumper (`xbox` mode) producing `.iso`, MD5-verifies,
    atomic-moves. No compress step — XBOX ISO is the
    deliverable. Output template
    `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).iso`.
    Original Xbox only.
  - `pipelines/data` handler: filesystem volume label becomes
    `Title` (`data-disc-YYYYMMDD-HHMMSS` fallback when label is
    empty). Rips with `dd` (`bs=2048 conv=noerror,sync`),
    sha256-hashes the ISO, records hash and size on the disc
    record, atomic-moves. Output template
    `{{.Title}}/{{.Title}}.iso`.
  - `tools/redumper.go`: `xbox` mode added; emits `<name>.iso`
    like `dvd` mode. New exported `RedumperOutputExt(mode)`
    helper.
  - `tools/dd.go`: new wrapper around `dd if=<dev> of=<out>
    bs=2048 conv=noerror,sync status=progress`. Pure
    `ParseDDProgress` for unit tests. Surfaces bad-sector
    handling via job log lines.
  - `identify/saturnprobe.go`: pure `ProbeSaturnReader` over an
    `io.Reader` plus `ProbeSaturn(devPath)` wrapper.
  - `identify/xboxprobe.go`: parses XBE base-address +
    certificate-VA offsets to extract title ID and
    allowed-region bitfield. Refactored to expose `ProbeXBE(data
    []byte)` so the pipeline can probe via `isoinfo -x
    /default.xbe` without mounting the disc.
  - `identify/dctoc.go`: `DCTOCProber` wrapping `cdrdao
    disk-info` + pure `looksLikeDreamcast` helper.
  - `identify/redump.go`: `LoadRedumpDir(rootDir)` walks
    `<root>/<system>/*.dat` and merges all dat-files into one
    in-memory index. New `LookupByXboxTitleID(uint32)` method.
    Bracket-code regex generalised to handle Saturn product
    numbers and Xbox hex title IDs alongside PSX/PS2 boot codes.
  - Classifier extended: Saturn / Dreamcast / Xbox / Data
    fallback branches layered after the existing PSX/PS2 logic.
    Probe errors route to DATA (matches the M3.1/M5.1
    conservative posture).
  - Engine schema gains `redumper` (formats `[ISO]`) and `dd`
    (formats `[ISO]`). `webui/src/lib/profile_schema.ts`
    mirrors. `DISC_TYPES` array gains `SAT`, `DC`, `XBOX`.
  - Four new seeded profiles: `Saturn-CHD`, `DC-CHD`,
    `XBOX-ISO`, `Data-ISO`. Idempotent seed functions in
    `daemon/settings/settings.go`.
  - Daemon main wiring switched from per-file
    `LoadRedumpDB(<system>.dat)` to a single
    `LoadRedumpDir(${DISCECHO_DATA}/redump)` call shared across
    all five Redump-aware handlers (PSX, PS2, Saturn, DC, Xbox).
  - README documents the per-system Redump dat-file layout
    under `${DISCECHO_DATA}/redump/<system>/`.
  - **Drops VCD from the disc-type matrix** (dead format; falls
    through to DATA pipeline as ISO).
- **M6.2 PWA install.**
  - Manifest, service worker (app-shell-only precache via Workbox),
    and update toast.
  - Mobile install via Chrome's URL-bar prompt; iOS Safari users see
    a one-time "Share then Add to Home Screen" hint banner,
    dismissible via a `localStorage` flag.
  - Update flow: when a new version is available, a "Reload" toast
    appears in the bottom-right; clicking it activates the new SW.
    Background `registration.update()` runs every 60 minutes so
    phone-pinned PWAs pick up updates without manual reloads.
  - Default icon set in `static/icons/` (existing — replace the PNGs
    to rebrand). Apple meta tags added to `app.html` for iOS
    add-to-home-screen support.
  - `theme-color` updated to the accent green (`#00d68f`) so the
    Android Chrome status bar tints correctly when launched as a PWA.
  - Service worker disabled in `pnpm dev` to keep hot-reload working;
    test the PWA via `pnpm preview` or against the deployed daemon.
  - No web push (notifications continue to flow through Apprise
    server-side per M0/M1 stack decisions).
- **M6.3 settings page + Apprise URL management + retention.**
  - Daemon: POST/PUT/DELETE on `/api/notifications` with
    `apprise --dry-run` validation at create/update time. 422 returns
    flat `{field: msg}`. Plus `POST /api/notifications/{id}/validate`
    (idempotent dry-run) and `POST /api/notifications/{id}/test` (real
    Apprise send; 502 on upstream failure). SSE `notification.changed`.
  - Daemon: `/api/settings` now accepts PUT for a partial map of
    M6.3-editable keys: `prefs.{accent,mood,density}` and
    `retention.{forever,days}`. Validation surfaces flat 422 errors per
    key. `retention.forever = "true"` is seeded on first daemon start
    so existing deployments don't see sudden deletions.
  - Daemon: history retention sweeper (`daemon/state/sweeper.go`) runs
    immediately on startup and daily at 03:00 daemon-local. Deletes
    `jobs` rows in `done|failed|cancelled` older than the configured
    cutoff; FK cascade trims `job_steps` and `log_lines`; orphaned
    `discs` rows pruned in the same transaction.
  - Daemon: `tools/apprise.go` gains error-surfacing `DryRun(url)` and
    `Send(urls, title, body)` methods alongside the existing `Run`
    (which keeps its swallow-and-warn semantics for the in-pipeline
    notify step).
  - WebUI desktop `/settings`: single scrolling page with System,
    Notifications, Appearance, Retention sections. NotificationEditor
    rows have Save / Validate / Test / Delete (two-step) buttons with
    inline 422 errors. AppearanceSection auto-applies on change
    (localStorage cache + DOM dataset + daemon PUT). RetentionSection
    has a toggle + numeric input with client-side validation.
  - WebUI mobile `/settings`: read-only list of all four sections;
    URLs truncated to just the scheme (e.g. `ntfy://...`,
    `tgram://...`) so embedded credentials never leak onto a phone
    screen. "Edit on desktop" footer.
  - WebUI: first-paint prefs hydration via inline IIFE in `app.html`
    reading `localStorage.discecho.prefs`. Bootstrap reconciles with
    daemon truth a few hundred ms later — no flash on revisits, single
    source of truth across devices.

### Changed
- Redump dat-files must now live under per-system subdirectories
  (`${DISCECHO_DATA}/redump/{psx,ps2,saturn,dreamcast,xbox}/*.dat`).
  Files placed directly under `${DISCECHO_DATA}/redump/` are no
  longer loaded — move them into the right subdirectory.

### Deprecated

### Removed

### Fixed

### Security

## [0.0.1] - 2026-05-06

### Added
- Initial project scaffold.