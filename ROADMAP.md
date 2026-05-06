# DiscEcho — Roadmap

Each milestone is independently testable and shippable. "Shippable" =
container builds, runs on the homelab host, doesn't regress earlier
milestones. No milestone is considered done until the verification gate
in `CLAUDE.md` passes.

## M0 — Skeleton + CI + container

**Goal:** a runnable empty service. Insert disc → daemon logs detection.

**Deliverables:**

- Repo scaffolding per `ARCHITECTURE.md` repo layout.
- Go daemon: HTTP server on `:8088`, `GET /api/health` returns `{ok:true}`,
  `GET /api/version` returns build info.
- udev subscription: log every disc-insert/eject event with drive path.
- SvelteKit project initialised with Tailwind, design tokens copied from
  `DiscEcho.html` `:root` block, dark-only.
- `Dockerfile` (multi-stage: Go + Node build stages → `python:3.12-slim`
  runtime with `pip install apprise` + the daemon binary + embedded UI),
  `docker-compose.yml` with `/dev/sr0` passthrough and a `data/` volume.
- GitHub Actions: lint + typecheck + test on push, build container image
  on tag.
- `CHANGELOG.md` updated; branch protection on `main` configured.

**Acceptance:**

- `docker compose up -d --build` succeeds on the homelab host.
- `curl http://localhost:8088/api/health` returns `{"ok":true}`.
- Inserting a disc into `/dev/sr0` produces a log line within 2 s.
- CI is green; PR template enforces conventional commits.

## M1 — Audio CD pipeline end-to-end (smallest meaningful slice)

**Goal:** insert an audio CD, see it identified, ripped to FLAC with
cuesheet, moved to library. Drive 1 only.

**Deliverables:**

- `daemon/identify` minimal classifier (TOC-based audio-CD detection).
- MusicBrainz client (`shared/wire.ts` `Candidate` type).
- `tools/whipper` wrapper with progress parser.
- `pipelines/audiocd.go` implementing `Handler`.
- `profiles` table seeded with `CD-FLAC`.
- REST endpoints: `/api/state`, `/api/drives`, `/api/jobs`,
  `/api/discs/:id/start`. SSE `/api/events` with `drive.changed`,
  `disc.detected`, `disc.identified`, `job.*` events.
- webui: mobile dashboard (drives + queue) + new-disc bottom sheet
  (auto-confirm 8 s) + mobile job detail with vertical stepper. No
  desktop UI yet.
- Notifications: `tools/apprise.go` shells out to the Apprise CLI on
  `job.done` and `job.failed`, using URLs from the `notifications` table.
  At least one URL pre-seeded via env / config for first run.

**Acceptance:**

- Insert an audio CD → mobile dashboard shows it within 2 s.
- New-disc sheet shows ≥1 MusicBrainz candidate.
- Top match auto-confirms; pipeline runs through all enabled steps.
- Output FLAC + cuesheet appear at the configured library path.
- An Apprise notification arrives on completion via the configured URL
  (verified with an `ntfys://` or `discord://` target).
- History row appears in SQLite (UI yet to render it).

## M2 — DVD-Video pipeline + history UI

**Goal:** add DVD-Video, plus the history screen on mobile.

**Deliverables:**

- `pipelines/dvdvideo.go` using `tools/handbrake` (and `tools/dvdbackup`
  if needed for VOB extraction).
- TMDB client behind a feature flag (provider configurable).
- `DVD-Movie` and `DVD-Series` profiles seeded.
- Mobile History screen with day-grouping and disc-type filter chips.

**Acceptance:**

- Insert a DVD-Video → identified, ripped, transcoded to MP4 (or MKV per
  profile), moved.
- History screen shows last 30 days, filter chips work.

## M3 — BDMV + UHD + multi-drive

**Goal:** Blu-ray and UHD-BD pipelines; concurrent jobs on multiple drives.

**Deliverables:**

- `pipelines/bdmv.go` (`MakeMKV → HandBrake`), `pipelines/uhd.go`
  (MakeMKV remux passthrough).
- Job scheduler: per-drive serialisation, cross-drive concurrency, queue
  for jobs without an available drive.
- `BD-1080p`, `UHD-Remux` profiles seeded.
- UHD requires user-supplied AACS2 key file; explicit setup step in README.

**Acceptance:**

- Two drives, two concurrent jobs, dashboard reflects both with live
  progress.
- A queued job (no drive available) shows the `queued · drv-X` hint.

## M4 — Desktop dashboard with live updates

**Goal:** desktop view brought to feature parity with the mock.

**Deliverables:**

- Desktop top-nav, sections (`dashboard`, `history`, `profiles`, `system`).
- Desktop dashboard hero drive + queue table + right-side job-detail
  panel + horizontal pipeline stepper.
- Keyboard shortcuts (`g d / g h / g p / g s`, `esc`).
- `⌘K` quick-jump (cosmetic only initially: search across drives, jobs,
  history titles).
- SSE reconnection + diff reconciliation on reconnect.

**Acceptance:**

- Open desktop URL, see live updates without page reload.
- Click a queue row → right-side panel updates without flicker.

## M5 — Profile editor + game discs

**Goal:** editable profiles on desktop; PSX/PS2 pipelines.

**Deliverables:**

- Desktop profile editor: codec/CRF/container/preset/output template.
- Validation against engine capabilities; per-step toggles.
- `pipelines/psx.go`, `pipelines/ps2.go` using `tools/redumper` and
  `tools/chdman`. Redump matching for identification.

**Acceptance:**

- Edit a profile, save, immediately apply to next job.
- Insert a PSX disc → identified against Redump → ripped to CHD.

## M6 — Polish + remaining disc types

**Goal:** remaining disc types (Saturn, Dreamcast, XBOX, raw data, VCD),
PWA install, settings polish, Apprise URL management.

**Deliverables:**

- One pipeline file per remaining disc type.
- PWA install (manifest, service worker, offline shell). Push notifications
  remain server-side via Apprise — no VAPID, no in-app push.
- Apprise URL management on the settings page: add / edit / delete /
  test-send for each row in the `notifications` table. Mobile: read-only
  list with toggles; desktop: full editor with `apprise --dry-run` for
  validation.
- Settings page on mobile (read-only) + desktop (full).
- Tweaks panel from the mock (accent / mood / density) wired to per-user
  settings.

**Acceptance:**

- All disc types in the handoff data set are handled.
- PWA installs on iOS Safari + Android Chrome.
- Adding a new Apprise URL from the desktop settings page and clicking
  "Test" delivers a real notification.

## Stretch (post-MVP)

- `Override` action on the job-detail screen (mid-run profile swap).
- Activity sparkline backed by real CPU/throughput sampling.
- Multi-host (federate two homelab boxes' drive lists).
- Integration with Plex/Jellyfin library refresh on `move` step.
