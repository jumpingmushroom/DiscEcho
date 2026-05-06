# DiscEcho — Open Questions

Numbered. For each: options, my recommendation, why. Edit this file with
your answers; I will not start M0 until at least items 1–7 are resolved.

## 1. Stack confirmation

**Question:** Web UI = SvelteKit, daemon = Go, DB = SQLite (WAL).

**Options:**
- A. SvelteKit + Go + SQLite (recommended).
- B. SvelteKit + Python (FastAPI) + SQLite.
- C. SvelteKit + Rust (axum) + SQLite.
- D. Other.

**Recommendation:** A. CLAUDE.md hints SvelteKit; Go's subprocess + SSE
ergonomics fit the daemon's job; SQLite per CLAUDE.md preferences.

**Your answer:** A

## 2. Single-container vs. two-service deploy

**Options:**
- A. Single Go binary, embedded SvelteKit assets, one container
  (recommended).
- B. Two services in one compose file (daemon + nginx-static).

**Recommendation:** A. Easier device passthrough, fewer moving parts.

**Your answer:** A

## 3. Pipeline-engine abstraction

**Options:**
- A. Hardcoded handlers per disc type, profiles as data (recommended).
- B. YAML-templated pipelines.
- C. External plugin processes.

**Recommendation:** A (hybrid: handlers in Go, profiles in SQLite). Tool
output parsing is too varied for templates; plugin overhead unjustified
at this scope.

**Your answer:** A

## 4. Metadata providers

**Options for video discs:**
- A. TMDB (API key required, free for non-commercial).
- B. OMDb.
- C. Local-only (filename heuristics, no remote lookup).

**Options for game discs:**
- A. Redump (datfile match by hash).
- B. No-Intro.
- C. Both, prefer Redump.

**Recommendation:** TMDB for video; Redump for games. Audio CDs already
fixed at MusicBrainz + AccurateRip per the mock.

**Your answer:** Agreed with recommendation

## 5. UHD support scope

**Question:** AACS2 stripping requires user-supplied keys. Build the UHD
pipeline now or defer?

**Options:**
- A. Build it; user supplies keys via config (recommended for M3).
- B. Defer to post-MVP.

**Recommendation:** A. The handoff features UHD prominently; build it
with an explicit "you must supply keys" setup step in README.

**Your answer:** A

## 6. Authentication for MVP

**Options:**
- A. Single shared bearer token via env (recommended).
- B. Per-user accounts + sessions.
- C. None — rely entirely on reverse-proxy auth.

**Recommendation:** A. Homelab single-user; add accounts only if a real
need appears.

**Your answer:** A

## 7. Notification backends for MVP

**Options:**
- A. ntfy + Discord webhook only (mocked in handoff).
- B. Add web push (VAPID) at M1.
- C. Add email (SMTP).

**Recommendation:** ntfy + Discord at M1; web push at M6 with PWA install.

**Your answer:** I think apprise support is better, so lets focus on that instead

## 8. Library naming convention

**Question:** Plex/Jellyfin-compatible naming, or custom?

**Recommendation:** Plex-compatible by default (`Movie Name (YYYY)/
Movie Name (YYYY).mkv`, `Show/Season 01/Show - S01E01.mkv`); profile
override with a Go template. Decide before M2.

**Your answer:** Plex/Jellyfin-compatible naming is fine

## 9. History retention

**Recommendation:** Keep forever by default; setting to enable rolling
retention (e.g. 90 / 365 days). Decide before M6.

**Your answer:** Keep forever

## 10. Drive-passthrough strategy

**Options:**
- A. Bind specific `/dev/sr*` nodes via compose `devices:` (recommended).
- B. Privileged container.

**Recommendation:** A. Privileged is unnecessary and a security regression.

**Your answer:** A

## 11. The `Override` action on mobile job detail

**Question:** What does this do?

**Recommendation:** Defer to post-MVP. Mid-run profile change is messy;
ship without it.

**Your answer:** No idea, post-mvp.

## 12. PWA push

**Question:** Own VAPID web-push or rely on ntfy/Discord only?

**Recommendation:** ntfy/Discord only for MVP; revisit at M6.

**Your answer:** apprise push only for MVP.
