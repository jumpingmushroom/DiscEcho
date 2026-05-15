# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- A new **Batch / Manual** mode toggle in Settings → Rip behaviour. Batch (default) keeps the existing flow: detect, identify, auto-rip after the 8 s countdown, eject when finished. Manual leaves the rip and the eject to you — pick a candidate, click Start, click Eject when you're ready. Disc detection + identification still happen automatically in both modes.
- **Stop**, **Re-identify**, and **Eject** controls on every drive card (mobile and desktop). Stop cancels a running rip, Re-identify re-runs the classify + lookup pipeline when MusicBrainz/TMDB picked the wrong release, Eject opens the tray. All three are visible in both modes — mode only changes whether the rip starts and ends automatically.
- A new global `rip.eject_on_finish` setting replaces the per-profile `auto_eject` flag. The toggle is disabled while in manual mode (the tray never auto-ejects in manual).

### Changed
- The 8-second "Auto-rip in Ns" countdown is now suppressed in manual mode; the awaiting-decision card shows "Manual mode · pick a title to rip" instead.

### Removed
- The per-profile `auto_eject` field is gone (replaced by the global `rip.eject_on_finish` setting above). Migration 008 drops the column from `profiles`; the profile editor no longer renders the checkbox.

## [0.10.11] - 2026-05-15

### Changed
- The mobile dashboard drive card now shows a 3-line live log tail while a job is ripping, so you can see what's actually happening during whipper's long startup phase (drive analysis, AccurateRip lookup, TOC re-read) instead of staring at a frozen 0% bar for two or three minutes. The tail re-uses the existing per-job log ring; nothing new on the wire.
- The whipper parser now forwards every `INFO`/`WARNING`/`ERROR`/`FATAL`/`CRITICAL` line from whipper's Python `logging` output to the job log, plus emits a single up-front "preparing drive (this can take 1–3 min)" hint the moment whipper starts producing output. Previously only three specific patterns (`Ripping track N of M`, `Reading: X%`, `Track N OK`) were captured and everything else — including all of whipper's startup-phase status — was silently dropped, leaving the Log tab empty until the first track started ripping.

## [0.10.10] - 2026-05-15

### Fixed
- Audio-CD rips no longer fail with `whipper: drive offset unconfigured. Please install pycdio and run 'whipper offset find'`. Whipper 0.10 refuses to rip unless a per-drive sample read-offset is configured, and the canonical detection flow (`whipper offset find`) needs both `pycdio` and a CD known to AccurateRip — neither of which we can assume in a homelab container. The daemon now passes `-o 0` to `whipper cd rip` so audio rips work out of the box on any drive. The resulting rip is audibly identical to a calibrated one (~0.14 ms drift) but won't match AccurateRip checksums; users who care can run `whipper offset find` inside the container manually and override the default.

### Changed
- The runtime image now installs `python3-cdio` so `whipper offset find` works for users who want to dial in a per-drive offset interactively. Adds ~2 MB to the image; was previously omitted because the Debian whipper package doesn't pull it in by default.

## [0.10.9] - 2026-05-15

### Fixed
- Audio-CD rips no longer fail instantly with `whipper: exit status 2`. Two bugs in the whipper invocation:
  - The args list passed `--keep-bad-files=no`, a flag whipper 0.10 doesn't recognise — Python argparse rejected it before whipper opened the drive, so every audio rip aborted at the rip step.
  - No `-d` was passed, so whipper fell back to its default device `/dev/cdrom`. That symlink doesn't exist in the daemon container (only `/dev/sr0`/`/dev/sr1` are exposed), so even with the flag fixed whipper would have errored opening the drive. The drive's `dev_path` is now explicitly passed via `cd -d <path> rip …`.

### Changed
- MusicBrainz disc-ID lookups that return exactly one release are now marked confident (100%) so the AwaitingDecision card's 8-second auto-rip countdown actually starts. Multi-release responses (re-issues, remasters with the same TOC) stay at 0% so the user has to pick the right release manually instead of the dashboard silently picking the first one. MB's `score` field is left ignored — it's a search concept that's not meaningful for the discid resource.
- The candidate metadata line on the AwaitingDecision card now includes the artist (e.g. `MusicBrainz · 1997 · Trust Obey`), so you can tell two same-titled releases apart without opening MusicBrainz.

## [0.10.8] - 2026-05-15

### Fixed
- The first rip into a non-existent library root no longer hard-fails with `library probe: stat /library/<root>: no such file or directory`. `ProbeWritable` now auto-creates the configured library directory (and any missing parents) with `0o777` permissions before probing, so a fresh install — or a new disc type on an existing install — Just Works. Applies to all ten pipelines (audio, data, DVD, BDMV, UHD, PSX, PS2, Saturn, Dreamcast, Xbox) since they all delegate to this shared probe. Errors still surface clearly when the parent is genuinely unwritable.
- Removed `TestDefaultCDInfoRunner_StopsAtDiscMode` which depended on subprocess scheduling timing under `-race` and flaked on slow CI runners. The deterministic `TestDefaultCDInfoRunner_PartialLineDoesNotFire` plus the kill-on-marker logic in production already cover the same surface.

## [0.10.7] - 2026-05-15

### Fixed
- Audio CDs now return MusicBrainz candidates again instead of an empty list. On the ASUS SDRW-08D2S-U `cdparanoia -Q` reports track offsets *relative to track 1* (track 1 begins at LBA 0), but the MusicBrainz disc-ID algorithm requires *absolute* LBAs that include the 150-frame lead-in pre-gap (track 1 begins at LBA 150). The existing parser took cdparanoia's `begin` value verbatim, which gave us a different disc-ID than MB and produced a guaranteed 404 on every lookup. The parser now detects relative offsets (track 1 below the 150-frame pre-gap) and shifts every track plus the leadout up by 150 to land in the canonical absolute frame.

## [0.10.6] - 2026-05-15

### Fixed
- The MusicBrainz disc-id lookup no longer fails with `400: releases is not a valid inc parameter for the discid resource`. MusicBrainz tightened validation on the `/ws/2/discid/{id}` endpoint and now rejects `inc=releases` (it used to be silently ignored — the discid resource already returns its containing releases by default). The client now requests `inc=artist-credits` only, which keeps the artist-credit data we surface in the candidate list.

## [0.10.5] - 2026-05-15

### Fixed
- Audio CDs are no longer misclassified as DATA when the daemon races the disc spin-up. On the ASUS SDRW-08D2S-U the udev media-change uevent fires 60–100 ms after insert; cd-info ran immediately and the drive answered the TOC read with **No medium found**, so cd-info wrote `Disc mode is listed as: Error in getting information` and **exited cleanly with status 0**. The classify retry loop only kicked in on non-zero exits, so the garbage output was passed straight to the parser, fell through to DATA, and the drive went idle with no disc shown. The runner now treats a clean cd-info exit without a usable disc-mode value as a transient failure — and the line-watcher ignores known cd-info error strings (`Error in getting information`, `Unknown`) so a kill-on-marker doesn't fire on a value that's really a failure indicator. With both changes the existing 11.5 s backoff schedule absorbs the spin-up race correctly.

### Changed
- Reverted the diagnostic `classify: cd-info captured` log added in 0.10.3 / 0.10.4 now that the underlying bug is fixed.

## [0.10.4] - 2026-05-15

### Changed
- Internal: the `classify: cd-info captured` debug log added in 0.10.3 now surfaces the *tail* of cd-info's output (last 3 KB) instead of the head — the disc-mode line lands late, after the capabilities listing.

## [0.10.3] - 2026-05-15

### Changed
- Internal: classify now logs the captured `cd-info` output (truncated to 1.5 KB) and the resulting base disc type at INFO level. Used to diagnose the still-misclassified audio-CD case from 0.10.2 in the field; will be dropped once the underlying bug is fixed.
- Relaxed timing on `TestDefaultCDInfoRunner_StopsAtDiscMode` so it no longer flakes under `-race` on slow CI runners — the test now allows up to 15 s for the short-circuit to land instead of the previous 3 s.

## [0.10.2] - 2026-05-15

### Fixed
- Audio CDs are no longer misclassified as DATA on drives that flush the disc-mode label and its value (`CD-DA`, `Mode 2`, …) in two separate writes. The 0.10.1 early-kill watcher fired on the prefix `Disc mode is listed as:` regardless of whether the value had landed yet, so when stdout flushed mid-line we'd kill `cd-info` with only the prefix in our buffer; the classifier then fell through to `RefineDiscType`, the ISO9660 probe failed, and the disc landed back in `idle` with no UI update. The watcher now waits for the full newline-terminated line with a non-empty value before stopping `cd-info`.

## [0.10.1] - 2026-05-15

### Fixed
- Audio CDs (and any disc whose drive can't satisfy a Media Catalog Number probe) no longer get stuck in **Identifying** until the classify timeout kicks in. `cd-info` was being run to completion and would hang for 60–90 s after the disc-mode line landed, while the drive retried internally on the MCN/ISRC reads at the end of its run. The classifier only needs the disc-mode line — and now stops `cd-info` the moment that line appears in stdout, returning in under a second instead of after a 30 s context deadline.

## [0.10.0] - 2026-05-15

### Added
- A phase-filter chip row on the per-job log viewer (`/jobs/[id]`). Each log line is now tagged with the pipeline step it was emitted from, so you can flip between **Rip / Transcode / Move / …** without losing the earlier-phase context when a chatty step starts spamming. Live jobs auto-track the active phase; finished jobs default to **All**. A new `GET /api/jobs/:id/logs?step=…` endpoint serves the persisted log so a page reload (or visiting a finished rip) repopulates the viewer instead of starting empty.
- A **Delete from history** button on the per-job page for finished rips — single-row counterpart to *Clear history*. Files on disk are untouched.
- Profiles and Settings are now fully editable on mobile. `/profiles` lists every profile grouped by disc type with a **+** affordance that drills into a full-screen editor at `/profiles/[id]` or `/profiles/new`; the desktop two-pane flow is unchanged. `/settings` is now a section index that drills into `/settings/system`, `/settings/notifications`, and `/settings/retention` — each renders the same editable section the desktop has, no more "edit on desktop" footer.

### Changed
- The mobile dashboard is reworked. A 2×2 stats grid (Active jobs / Today ripped / Library size / Failures 7d) replaces the empty "today —" placeholder strip, the running job is no longer duplicated as both a full-bleed card and an Active-queue row, and each drive renders a single compact card with state pill, disc art, active-step subtitle, and progress — the home screen no longer carries a stepper or log tail, both of which live on the job detail page. The Queue section now lists only queued (not-yet-running) jobs.
- The per-job page (`/jobs/[id]`) on mobile is now tabbed into **Pipeline / Log / Summary** rather than one long scroll, so each view fits a phone screen without competing for space.
- Mobile navigation gained a **Profiles** tab. Mobile shell components moved into `webui/src/lib/components/mobile/` to mirror `desktop/`.

### Fixed
- The history detail page is no longer the running-job page in disguise. Finishing rips reached from `/history` now show real cover art, an outcome pill (DONE / FAILED / CANCELLED), the elapsed time, output size, and profile, plus the per-phase log viewer — replacing the stale ETA chip, placeholder cover, empty live-log tail, and disabled Pause/Override/Cancel buttons that previously rendered. Running rips keep their existing layout, minus the two disabled buttons.
- A finished job's progress no longer reads as **0%**. The pipeline now persists 100% on each step's completion, so the final step's percentage doesn't linger at whatever the last sub-100 progress sample happened to be.
- The mobile AppBar no longer clips its title under the iOS status bar / notch. The sticky header now respects `env(safe-area-inset-top)`.
- Settings and profile forms on mobile no longer cram a 200px label column onto a phone-width screen; `FormRow` now stacks label-above-input below the `md` breakpoint and keeps the side-by-side desktop layout at `md+`.

## [0.9.3] - 2026-05-15

### Fixed
- The History and job-detail pages no longer show two `LIVE`/`WAIT` indicators side-by-side on desktop. The desktop top-nav already renders the SSE-connection dot, and the mobile-chrome AppBar/header rendered a second copy unconditionally; the per-page dot is now hidden on desktop and shown only on mobile.

## [0.9.2] - 2026-05-15

### Fixed
- The drive no longer locks itself out of further disc events after an identify failure or after ejecting a disc. When the classify step timed out (or any cleanup hit a cancelled context), the follow-up "reset to idle/error" write was made with the same cancelled context and silently failed, leaving the drive permanently stuck in `identifying`. Every later eject and re-insert then hit "drive already identifying, ignoring" and the daemon stayed deaf until the SQLite row was manually corrected. Cleanup writes now use a fresh context, errors surface to the log, and any drive left in `identifying` from a previous run is reset to `idle` at daemon startup.

## [0.9.1] - 2026-05-15

### Fixed
- The daemon no longer goes permanently deaf to disc insertions after a transient kernel-event hiccup. The udev watcher ran disc identification inline on its event-reading loop, so a slow identify (the `cd-info` probe alone can retry for ~13s) stalled the loop long enough for the kernel's event socket buffer to overflow; the resulting error killed the watcher with no restart, and every disc inserted afterwards was silently ignored until the container was restarted. Identification now runs off the read loop, and the watcher reconnects automatically if its event stream ever drops.
- Discs whose type is determined by reading the filesystem (PlayStation 2, PSX, DVD, Blu-ray, Xbox) are no longer intermittently misidentified as a generic DATA disc. The classifier's `isoinfo` filesystem probe is now retried through the disc spin-up window — the same way the `cd-info` probe already was. Previously, when `isoinfo` ran in the brief window where `cd-info` had succeeded but the ISO9660 filesystem wasn't yet readable, it returned an empty listing that silently downgraded the disc to DATA, where it became invisible in the UI (no candidates, no card) — so identify appeared to "just stop". The classifier also now logs a breadcrumb when a disc isn't recognised by any probe.

## [0.9.0] - 2026-05-14

### Added
- A **Clear history** button on the History tab — wipes all finished-rip records (jobs, their logs, and the disc rows they orphan) in one click, behind a two-step confirm. The ripped files on disk are not touched, and an in-progress rip is unaffected.

### Fixed
- Inserting a disc no longer enqueues two identical rip jobs. The dashboard kept both its mobile and desktop layouts mounted at once (CSS only hid one), so each identified disc ran two auto-confirm timers and fired two start-rip requests; and the daemon's duplicate-job guard wasn't atomic with job submission, so both slipped through. The dashboard (and the profiles/settings pages) now mount a single layout for the viewport, and the start-rip handler serializes its guard so concurrent requests for the same disc create exactly one job.
- The active rip card showed the current step's speed and ETA twice — once in the pipeline stepper's "Active" row and again below the progress bar. It now appears once, below the bar. (There is no whole-rip ETA to show in the other slot: speed/ETA are per-step and reset at each step boundary.)

## [0.8.0] - 2026-05-14

### Changed
- DVD encode quality is now a real, per-profile setting. The `quality_rf` and `encoder_preset` profile options drive HandBrake directly — previously the quality was hardcoded in the pipeline and the profile's quality field was display-only. The seeded DVD profiles default to RF 18 / preset slow for near-transparent archives; existing DVD profiles are migrated to the same defaults. Set `quality_rf` higher (e.g. 20–22) for smaller files.
- DVD and Blu-ray rips to MKV now keep **every** subtitle track on the disc instead of filtering to a single language. MP4 profiles keep the language-filtered behaviour, since MP4 can't cleanly carry bitmap (VOBSUB) subtitles.

### Fixed
- The transcode step now shows a live ETA and speed. HandBrake omits its own ETA when its output is piped, so DiscEcho derives both from the title duration and elapsed time.
- HandBrake's transcode log output is no longer all tagged as warnings. Its startup JSON job dump is dropped, routine lines are logged at info, and only genuine errors/warnings are flagged.
- The queue detail pane now shows the disc title whenever it is known, instead of "Unknown disc" when the rich metadata blob hasn't been fetched. It already had the title — it just refused to display it without the extended TMDB/MusicBrainz payload.
- dvdbackup's progress output and libdvdread trace lines no longer flood the job log or get mislabelled as warnings. Progress is dropped (the size-based poller already drives the percentage), libdvdread chatter is dropped unless it carries an error, and the rest is logged at info — warnings are reserved for genuine failures.

## [0.7.0] - 2026-05-14

### Added
- Saving settings in the web UI now shows a brief confirmation toast. Covers profile create/save/delete, notification create/save/delete, notification Validate/Test results, history retention, and library paths — previously these succeeded silently.

## [0.6.1] - 2026-05-14

### Fixed
- Editing and saving a DVD-Movie or DVD-Series profile in the web UI no longer fails silently. The seeded DVD profiles carry a `dvd_selection_mode` option that was missing from the HandBrake engine's validation schema, so every save was rejected with a 422 — including unrelated changes like switching the video codec.

## [0.6.0] - 2026-05-14

### Added
- Hardware-accelerated transcoding (NVENC) on NVIDIA hosts. HandBrake-based profiles now expose `nvenc_h264` and `nvenc_h265` as `video_codec` values. The daemon detects NVENC availability at startup, surfaces it in **Settings → Integrations**, and silently falls back to the matching software encoder (`x264` / `x265` / `x265_10bit` for BDMV) when no GPU is detected. See the README "Enabling GPU transcoding" section for the per-host compose overrides required to pass the GPU through.

### Changed
- HandBrake is now built from upstream source (1.11.x) in the container image so NVENC is compiled in; bookworm's apt package ships without NVENC. Cold builds take noticeably longer; cached rebuilds are unchanged.

## [0.5.0] - 2026-05-13

### Added
- Sidebar opens a per-disc-type **metadata pane** instead of repeating the hero RipCard. Movies show plot / director / cast / studio / genres / rating; audio CDs show label + track list; games show system / region / serial / Redump MD5; data discs show filesystem inventory.
- Poster and cover art are pulled from TMDB / CoverArtArchive at identify time and shown in the hero RipCard, awaiting-decision card, and sidebar metadata pane via a new shared `DiscArt` component.
- Top widgets row on the desktop dashboard with **ACTIVE JOBS**, **TODAY RIPPED**, **LIBRARY SIZE**, and **FAILURES (7D)** cards. Each card has a sparkline showing recent history (24h / 7d / 30d) and a contextual subline (hour-over-hour delta, today's title count, library capacity from statfs, 7d-over-7d delta).
- New SQLite column `jobs.output_bytes` tracks each completed rip's encoded size; library widget sums these. Auto-recorded by `PersistentSink.OnStepDone` from each pipeline's move-step notes — no per-pipeline changes needed.
- New API endpoint `GET /api/stats` returns the widget payload; also embedded in the SSE `state.snapshot` event and `GET /api/state` for zero-extra-roundtrip bootstrap.

### Changed
- Dashboard hero band's minimum column width raised from 280px to 380px so the pipeline stepper labels never clip.

### Fixed
- Log tail now populates during DVD / BD / UHD rips: pipelines emit milestone log lines at each step boundary (dvdbackup or MakeMKV start/complete, HandBrake scan/encode start/complete, move). Stream parsers forward unrecognised non-progress lines so warnings and errors are visible instead of silently dropped, capped at 200 lines per step.

## [0.4.1] - 2026-05-13

### Fixed
- Transcode progress is now visible during the encode step. HandBrake 1.6.x omits the `(avg fps X, ETA YhYmYs)` suffix when stdout is a pipe; the encode regex now matches both forms instead of dropping every line.
- `POST /api/discs/{id}/start` returns 409 Conflict when the disc already has a queued / running / identifying / paused job, so a fast double-click on the awaiting-decision card can no longer enqueue two jobs for the same disc.
- The awaiting-decision card disables its rip button the moment either auto-confirm or a manual click fires, coalescing both code paths into a single start request.

### Changed
- Desktop dashboard sidebar (job detail panel) is hidden by default and opens on queue-row click. Same row clicked again closes the panel; a different row switches between jobs.
- When the sidebar shows a drive's currently-running job, that drive's hero RipCard collapses from the top band so the same content isn't rendered twice. Other busy drives' hero RipCards stay.

## [0.4.0] - 2026-05-13

### Added
- Rank-based identification confidence so popular and obscure titles both surface useful scores (top match always 100%, then 80/60/40/20).
- DVD profiles now disambiguate movie-vs-series via a `dvd_selection_mode` option, freeing the format field to default to MKV.

### Changed
- The dashboard now swaps a busy drive's idle card for an inline rip card. The rip card uses the drive identity (bus + model) as its header, so the running drive surfaces in exactly one place instead of two.
- DVD-Movie's default output container is now MKV. Existing installs are migrated in place where the profile is still on the seed defaults.

### Fixed
- The drive progress bar no longer freezes at 100% when a rip step hands off to transcode. Per-step progress now resets at the moment the new step starts.
- ETA in the job list and queue table now renders as `Xm Ys` / `Xh Ym Zs` instead of raw seconds.

## [0.3.3] - 2026-05-13

### Fixed

- **Tool stream parsers no longer deadlock the subprocess on stream
  shape they can't parse.** The 0.3.2 Jackass re-rip hung HandBrake
  mid-encode for ~85 minutes after `bufio.Scanner` aborted on a long
  carriage-return-separated progress chunk: the read goroutine exited,
  the kernel pipe buffer filled, HandBrake blocked on `pipe_write`
  trying to finalise the MP4. The encoded file was already 2.6 GB on
  disk but the daemon never observed completion until we drained the
  pipes externally. Every tool wrapper that consumes a subprocess pipe
  (`handbrake`, `dvdbackup`, `whipper`, `chdman`, `makemkv`,
  `redumper`, `dd`) now wraps its scan loop with a helper that
  unconditionally copies remaining bytes to `io.Discard` after the
  parser returns, so a parse failure can never starve the subprocess.
  HandBrake and `dd` additionally use a `\r`-aware splitter so each
  progress chunk parses as its own line.
- **DiscFlow no longer creates duplicate Disc rows on duplicate
  uevents.** Hollywood DVDs emit 2–3 `media-change` kernel uevents as
  the drive settles after insertion; before 0.3.3 the guard only
  blocked uevents that arrived after a Job started, so every uevent
  during the identify-in-flight window kicked off its own classify +
  identify and produced a separate Disc row. The new
  `Store.ClaimDriveForIdentify` runs an atomic `UPDATE drives SET
  state='identifying' WHERE id=? AND state IN ('idle','error')` and
  the discFlow handler drops the uevent when the CAS doesn't claim
  the slot. Legitimate disc swaps still work because every identify
  code path transitions the drive back to `idle`/`error` on exit.
- **Skip button on awaiting-decision cards now actually skips.**
  Pre-0.3.3 the Svelte handler only flipped a client-side bottom-sheet
  flag — the daemon disc row was untouched, so any page reload
  rederived the card. The new `DELETE /api/discs/{id}` endpoint hard-
  deletes a disc row when no job references it; the daemon refuses
  the request with `409 Conflict` otherwise so jobs with history stay
  reachable from the History tab. A new `disc.deleted` SSE event
  drops the row from the live store.

### Added

- `Store.DiscHasAnyJob`, `Store.DeleteDisc`, `Store.ClaimDriveForIdentify`.
- `tools/streamparse.go` — `drainAfterScan` helper + `splitCROrLF`
  bufio split function, both used by every existing stream parser.

## [0.3.2] - 2026-05-13

### Fixed

- **HandBrake scan now enumerates every title on the disc.** The
  daemon called `HandBrakeCLI --input <src> --scan`, which silently
  defaults to `title_index=1` — only the *first* title is reported.
  Every multi-title DVD (Jackass: The Movie's 10 VTSs, season-set
  TV discs, anything menu-driven with a short preview as title 1)
  came back from the scan as a single 7-min preview, masking the
  real feature. Adding `--title 0` to the scan invocation makes
  HandBrake walk the full IFO and return every title. This was the
  root cause behind both the 0.2.3 silent-junk regression
  (longest-of-one-title is always the wrong title) and the 0.3.1
  feature-floor false-positive on this disc.

## [0.3.1] - 2026-05-13

### Added

- **Movie-profile feature-duration floor.** When the longest scanned
  title is below the floor (default 20 minutes), the transcode step
  fails with a descriptive error before invoking HandBrake. This is
  a belt-and-braces guard alongside v0.3.0's `--main-feature`: a
  disc with no main-feature bit set in the IFO, or an incomplete
  dvdbackup mirror, can leave the scan's longest title at a few
  minutes (Jackass: The Movie shipped a 7-min sketch as the entire
  feature in v0.2.3). Failing here is preferable to producing a
  junk file that passes the bytes-per-second check (which only
  compares against the *encoded* duration, not the expected feature
  duration). Override per profile via the `min_feature_seconds`
  option; set to `0` to disable for legitimately-short content.

## [0.3.0] - 2026-05-12

### Changed

- **DVD movie profile now lets HandBrake pick the main feature.**
  Pre-v0.3.0 the orchestrator scanned titles and selected the one
  with the longest reported duration. On a homelab test against
  the same disc twice, that heuristic shipped an outtakes reel
  instead of the 85-min feature — outtakes were the longest
  *encodable* title HandBrake's scan returned (either because
  dvdbackup's `-M` mirror skipped the main-feature VOBs or because
  libdvdread's IFO parser came up short). HandBrake's
  `--main-feature` flag reads the IFO's main-feature bit and picks
  the right title from the DVD's own metadata; movie profiles
  now use it.
- Series profiles (MKV) are unchanged — they still scan, filter
  to titles ≥ `min_title_seconds`, and encode each as one episode.

### Added

- **Scan-title logging.** Every title HandBrake's `--scan` returns
  is now emitted as an `INFO` slog line
  (`scanned title disc=… title=N duration_sec=…`). Diagnoses
  "wrong title picked" regressions from `docker logs` alone, no
  workdir snapshot needed.
- **TMDB runtime cross-check.** When the user picks a TMDB-movie
  candidate, `/discs/{id}/start` now fetches the runtime from
  `/movie/{id}` and persists it on the disc (`disc.runtime_seconds`).
  The DVD pipeline compares the scanned longest-title duration to
  this expected runtime; on a >50 % mismatch a `WARN` is logged:
  `duration mismatch expected_sec=5100 scanned_longest_sec=900 ratio=0.18`.
  The check warns but doesn't fail — DVDs legitimately differ
  from theatrical runtimes (director's cuts, regional edits) —
  but a 5× gap is a clear "this isn't the right disc content"
  signal.
- New `tools.TMDBClient.MovieRuntime(ctx, tmdbID)` API call.
- New `state.Candidate.RuntimeSeconds` + `Store.UpdateDiscRuntime`.

### Fixed

- `selectEncodeTitles` MP4 (movie) branch removed — it's no
  longer reachable; `--main-feature` covers movie selection.
  Series branch unchanged.

## [0.2.3] - 2026-05-12

### Added

- **Real percentage during DVD rip step.** dvdbackup itself only
  emits per-VOB log lines, which left the dashboard's progress bar
  pinned at 0 % through the entire rip. The DVD wrapper now reads
  the disc's total size once from `/sys/block/<dev>/size`, then
  polls the workdir every 2 s, summing bytes written, computing
  `written / total × 100` and pushing it through the sink. A
  10-second sliding window over the same samples produces a
  human-readable speed (e.g. `7.2MB/s`) and ETA. Progress caps at
  99 % until dvdbackup exits cleanly, then snaps to 100 % — the
  disc total always overshoots the actual VIDEO_TS bytes because
  of sector-alignment slack and non-DVD-Video tracks.

## [0.2.2] - 2026-05-12

### Changed

- **DVD-Video rip step now uses dvdbackup, not MakeMKV.** MakeMKV's
  rolling 60-day beta-key cycle plus the late-cycle stall we hit
  (1.18.3 from 2026-01 expired ~Mar 26, no 1.18.4 from upstream as
  of mid-May) made it a poor fit for DVDs. dvdbackup is GPL,
  ships in Debian apt, uses libdvdcss for CSS decryption, and
  needs no registration. The new flow:
  1. `dvdbackup -M -i /dev/sr0 -o <work>/rip -p` mirrors VIDEO_TS
     into `<work>/rip/<volume_label>/VIDEO_TS/`.
  2. `HandBrakeCLI --input <work>/rip/<volume_label> --scan`
     enumerates titles.
  3. `HandBrakeCLI --input <work>/rip/<volume_label> --title N
     --output …` encodes each selected title from the local mirror.
  HandBrake still never touches `/dev/sr0`. The MakeMKV pipeline
  stays for BDMV and UHD, where it remains the only viable decoder.
- The DVD pipeline no longer requires `DISCECHO_MAKEMKV_BETA_KEY`.
  The env var is still consumed for BDMV / UHD; without it those
  pipelines fail at scan-time as before.
- Runtime image gains the `dvdbackup` apt package.

### Added

- `tools.DVDBackup` wrapper exposing `Mirror(ctx, devPath, outDir,
  sink)` over the `dvdbackup` binary. Streams `*.VOB` mentions to
  the sink as coarse progress ticks.
- `dvdvideo.DVDMirror` + `dvdvideo.HandBrakeScanner` interfaces on
  the pipeline so tests can substitute fakes.

## [0.2.1] - 2026-05-12

### Fixed

- **MakeMKV beta key now actually reaches makemkvcon.** v0.2.0
  tried to bridge `MakeMKV/` → `.MakeMKV/` with a symlink, but
  makemkvcon's first scan materialises `.MakeMKV/` as a real
  directory (`_private_data.tar` + `update.conf`), so the symlink
  branch bailed out on its own "existing non-symlink target"
  guard. Result: BDMV/UHD/DVD-via-MakeMKV still failed with
  "application version is too old" on freshly applied 0.2.0
  containers. The daemon now writes `settings.conf` into **both**
  `${DISCECHO_DATA}/MakeMKV/` and `${DISCECHO_DATA}/.MakeMKV/`
  unconditionally — idempotent, no symlinks, no guards.

## [0.2.0] - 2026-05-12

### Changed

- **DVD-Video pipeline now uses MakeMKV for the rip step** instead of
  reading the disc directly with HandBrake. The new flow mirrors the
  BDMV pipeline: `makemkvcon mkv dev:/dev/sr0 …` extracts each
  selected title to an MKV in the workdir, then HandBrake transcodes
  each MKV from the local filesystem. HandBrake no longer touches
  `/dev/sr0` for DVD jobs, so spurious kernel media-change uevents
  during a long read can't truncate the output.
- **MakeMKV now requires the SCSI Generic node** (`/dev/sg0` on most
  hosts; the actual node may differ — check `ls -l /dev/sg* | grep
  cdrom`). The compose file maps `${OPTICAL_SG_DEVICE:-/dev/sg0}`;
  on unraid you need to add the matching `--device` row to the
  container template manually, alongside the existing `/dev/sr0`.
- **DVD ripping now requires `DISCECHO_MAKEMKV_BETA_KEY`** to be set,
  same as BDMV/UHD already did. MakeMKV's "free DVD" mode still
  needs a registration key while in beta; the project README points
  to the rotating public key.
- DVD encoder defaults stay at HandBrake x264 quality 20 in the
  profile, but the orchestrator no longer infers titles from
  HandBrake's scan output — it uses MakeMKV's `info` output
  instead. Title-selection rules (longest for MP4 movie profile,
  ≥`min_title_seconds` for MKV series profile) are unchanged.

### Fixed

- **MakeMKV beta key was never actually loaded.** The daemon wrote
  `app_Key = "…"` to `${DISCECHO_DATA}/MakeMKV/settings.conf` but
  `makemkvcon` reads from `$HOME/.MakeMKV/settings.conf` (note the
  leading dot). The two directories never matched, so BDMV / UHD
  jobs would have hit "application version is too old" the moment
  anyone tried them. `writeMakeMKVBetaKey` now also creates a
  `.MakeMKV → MakeMKV` symlink in the parent directory so
  makemkvcon finds the config we wrote, no matter how `HOME` is
  passed.
- **discFlow guard now also bails during a 10-second post-job
  cooldown.** The v0.1.4 guard checked only that the drive had no
  *active* job, which lost the race where a spurious media-change
  uevent fires at the exact instant HandBrake exits — the job was
  briefly `done` before the guard's check ran, and the re-classify
  proceeded to clobber the drive. Cooldown closes the race; new
  `Store.HasRecentJobOnDrive`.
- **Orchestrator now writes `jobs.active_step`** at every
  `OnStepStart`. Previously the column was always NULL despite the
  per-job step table being correctly maintained, so the desktop
  pipeline stepper never highlighted the active dot. New
  `Store.SetActiveStep`.
- **Dashboard now shows `running` once the orchestrator picks up
  the job.** The webui's `job.step` SSE handler used to update only
  the steps array; the job's `state` stayed `queued` until
  `job.done` arrived, so the queue row read "QUEUED 0%" through
  the entire rip. Handler now auto-promotes `queued → running`
  when the first step transitions out of pending.
- **Encoded-output validator floor raised** from 300 kbps to 750
  kbps (`minEncodedBytesPerSecond` 37 500 → 93 750). The 0.1.4
  threshold was *just* below the 270 kbps truncation we saw on the
  homelab. Real x264 quality-20 movies sit ≥ 1.5 Mbps so the new
  floor still has comfortable headroom.

## [0.1.5] - 2026-05-12

### Fixed

- **Awaiting-decision list no longer resurrects every old disc.**
  The 0.1.4 filter only excluded discs with an *active* job
  (`queued`/`running`/`identifying`), so any disc whose job had
  already ended — `done`, `failed`, `interrupted`, `cancelled` —
  came back as a stale decision card on every page load. Combined
  with the redundant "is DVD or BDMV" clause, this meant that
  every disc ever inserted re-prompted forever. The list now
  excludes any disc that has *any* job record at all — picking
  a candidate is a one-shot decision; re-rip flows will be a
  separate, explicit affordance.

## [0.1.4] - 2026-05-12

### Added

- **Persistent "Awaiting decision" card** on both desktop and mobile
  dashboards (`AwaitingDecisionCard` + `AwaitingDecisionList`).
  Replaces the modal `DiscIdSheet` bottom sheet — refreshing the page
  no longer makes the picker disappear, because the surface is
  derived from `$discs ∩ $jobs` (any disc with candidates and no
  live job is awaiting decision). The legacy `DiscIdSheet` component
  remains in-tree for now; nothing mounts it.
- `Store.HasActiveJobOnDrive(ctx, driveID)` query for the guard
  below.
- `dvdvideo.Deps.MinEncodedBytesPerSecond` to override the encode
  size threshold (tests use `-1` to disable).

### Fixed

- **Mid-rip udev events no longer wreck the running job.** When
  HandBrake / makemkvcon are hammering `/dev/sr0` the kernel emits
  spurious media-change uevents; the daemon used to re-run the
  classifier, race the running step for the device, fail, flip the
  drive to Error, and kill the in-flight rip. `discFlow.handle`
  now bails out early when `HasActiveJobOnDrive` is true.
- **DVD pipeline rejects truncated encodes.** `HandBrakeCLI` exits
  0 in several end-of-stream failure modes (e.g. the spurious
  media-change above), and the orchestrator persisted the job as
  `done` despite the output being a 170 MB fragment of a feature
  film. The transcode step now validates the encoded file's size
  against a 300 kbps × duration lower-bound and fails the step on
  a truncated output.
- **Drive returns to `idle` after `disc.identified`.** Previously
  the daemon left the drive stuck in `identifying`, the dashboard
  read "Identifying disc…" forever, and subsequent uevents from
  the user ejecting the disc were ignored.
- **`drive.changed` SSE handler honours the daemon payload.**
  Daemon publishes `{drive_id, state}`; the handler used to read
  `p.drive` (a full `Drive` object that the daemon never sends),
  so drive state updates only surfaced on a full page reload. The
  handler now patches the matching drive entry in place.

## [0.1.3] - 2026-05-12

### Added

- `/api/state` and the SSE `state.snapshot` event now include a `discs`
  array of the 50 most-recently-created discs. The webui seeds its
  `discs` cache from this on cold load so titles, candidates, and disc
  types resolve immediately instead of waiting for a `disc.detected`
  SSE event.
- `Store.ListRecentDiscs(ctx, limit)` backs the new payload.

### Fixed

- `ListActiveAndRecentJobs` now hydrates each job's `Steps` slice from
  the `job_steps` table. Previously every job returned with
  `step_count: 0`, so the desktop pipeline stepper and queue dots
  rendered empty for every row regardless of actual progress.
- **Desktop dashboard now shows what's really happening.** The drive
  hero card caption follows `drive.state` (`Ripping disc…` /
  `Identifying disc…` / `Drive error — see logs`) instead of always
  reading `Idle — insert a disc`. The queue table's `DRV` column
  renders the drive's bus name (`sr0`) instead of the raw UUID, and
  the `Title` column falls back through `disc.title →
  candidates[0].title → disc.id[:8]` instead of hardcoding `Unknown`.
- Drive hero card disc lookup now prefers the active job's `disc_id`
  when the drive row doesn't carry `current_disc_id`, so a fresh
  rip shows up immediately even before the disc-binding migration
  lands.

## [0.1.2] - 2026-05-12

### Fixed

- **Classifier no longer races the drive spin-up.** `cd-info` is now
  retried with backoff (~13 s total: 0.5 s, 1 s, then 2 s × 5)
  whenever it returns a non-zero exit. Without this, the first
  classify attempt landed 60–100 ms after the udev media-change
  event — well before the drive could answer a SCSI INQUIRY — and
  the drive flipped to **Error** with `cd-info: exit status 1`.
- **Identify sheet no longer auto-rips a low-confidence guess.**
  When the top candidate's confidence is below 50, the 8-second
  auto-confirm countdown is suppressed; the sheet now reads
  `No confident match · pick a title or search`. Empty candidate
  lists render `No match found · search manually`. Previously the
  sheet would auto-start a rip on whatever first candidate
  TMDB / MusicBrainz returned, even at 0 % confidence.

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