# DiscEcho — Architecture

This document is the day-one read for a contributor. It covers the system
shape, data model, daemon↔webui contract, the pipeline-engine abstraction,
and the deployment shape.

## System overview

DiscEcho is a Linux service that watches optical drives, classifies
inserted discs, runs per-disc-type rip → transcode → tag → move pipelines,
and exposes a mobile-first web UI for live status and history.

```
                ┌────────────────────────────┐
                │    udev / netlink (host)   │
                └──────────────┬─────────────┘
                               │ disc-insert / disc-remove
                               ▼
┌───────────────────────────────────────────────────────────────┐
│                   DiscEcho daemon (Go)                        │
│                                                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐ │
│  │  drive.svc   │  │ identify.svc │  │ pipelines/<type>     │ │
│  │  (udev sub,  │─►│ (probe +     │─►│ (plan, run, parse    │ │
│  │   trays,     │  │  metadata)   │  │  tool output)        │ │
│  │   eject)     │  └──────────────┘  └──────────┬───────────┘ │
│  └──────────────┘                               │             │
│                                                 ▼             │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ tools/{makemkv,handbrake,whipper,redumper,dvdbackup,    │  │
│  │        chdman,dd,...}  — exec.Cmd + stream scanners     │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                               │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────────┐   │
│  │  state       │──►│  broadcaster │──►│ HTTP+SSE server  │──┼──► webui
│  │  (in-mem +   │   │  (chan Event)│   │ (REST + /events) │   │
│  │   SQLite)    │   └──────────────┘   └──────────────────┘   │
│  └──────────────┘                                             │
│                                                               │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ embed: SvelteKit static build served at /                │ │
│  └──────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────┘
                               │
                               ▼
                       ┌───────────────┐
                       │   library/    │  (bind mount)
                       └───────────────┘
```

## Components

| Component | Tech | Responsibility |
|---|---|---|
| `daemon/drive` | Go | udev subscription, drive registry, eject/load |
| `daemon/identify` | Go | probe inserted disc, classify type, fetch metadata candidates |
| `daemon/pipelines` | Go | per-disc-type handlers (one file per type) |
| `daemon/tools` | Go | `exec.Cmd` wrappers + stdout/stderr scanners per tool (`makemkv`, `handbrake`, `whipper`, `redumper`, `dvdbackup`, `chdman`, `dd`, `apprise`, …) |
| `daemon/state` | Go + SQLite | in-mem job state, persistent history, profiles |
| `daemon/api` | Go (net/http + chi) | REST + SSE |
| `daemon/embed` | Go | embeds the SvelteKit `build/` and serves it on `/` |
| `webui` | SvelteKit + Tailwind | mobile-first PWA + desktop dashboard |
| `shared` | TypeScript types / JSON Schema | wire-format definitions |

## Data model (SQLite)

Tables, with the columns the UI actually consumes from the handoff:

- `drives` — `id`, `model`, `bus`, `dev_path`, `state` (`idle|identifying|ripping|ejecting|error`), `last_seen_at`, `notes`.
- `discs` — `id`, `drive_id`, `type` (`AUDIO_CD|DVD|BDMV|UHD|PSX|PS2|XBOX|SAT|DC|VCD|DATA`), `title`, `year`, `runtime_seconds`, `size_bytes_raw`, `metadata_provider`, `metadata_id`, `created_at`.
- `profiles` — `id`, `disc_type`, `name`, `engine`, `format`, `preset`, `options_json`, `output_path_template`, `enabled`, `step_count`, `created_at`, `updated_at`.
- `jobs` — `id`, `disc_id`, `drive_id` (nullable while queued), `profile_id`, `state` (`queued|identifying|running|paused|done|failed|cancelled|interrupted`), `active_step`, `progress`, `speed`, `eta_seconds`, `elapsed_seconds`, `started_at`, `finished_at`, `error_message`.
- `job_steps` — `id`, `job_id`, `step` (one of the 8 canonical steps), `state` (`pending|running|done|skipped|failed`), `started_at`, `finished_at`, `attempt_count`.
- `log_lines` — `id`, `job_id`, `t` (RFC3339 ms), `level` (`debug|info|warn|error`), `message`. (Bounded ring per job; older lines truncated.)
- `history` — Materialised view / query over `jobs` + `discs` for the History screen.
- `notifications` — `id`, `name`, `url` (an [Apprise](https://github.com/caronc/apprise) URL such as `ntfys://topic`, `discord://webhook_id/token`, `tgram://bot_token/chat_id`, `mailto://user:pass@host`, etc.), `tags` (comma-separated for routing), `triggers` (subset of `done,failed,warn`), `enabled`.
- `settings` — `key`, `value` (library path, accent/mood/density, identification provider preferences, Apprise CLI path).

The 8 canonical pipeline steps are fixed and not user-editable for MVP:
`detect`, `identify`, `rip`, `transcode`, `compress`, `move`, `notify`,
`eject`. Profiles can mark steps `skipped` (e.g. data ISO has no
transcode/compress).

## Daemon ↔ webui contract

REST (JSON):

- `GET /api/state` → snapshot of `{ drives, jobs, profiles, settings }` for first paint.
- `GET /api/drives`, `GET /api/drives/:id`
- `POST /api/drives/:id/eject`
- `GET /api/jobs`, `GET /api/jobs/:id`
- `POST /api/jobs/:id/pause` | `/resume` | `/cancel`
- `POST /api/discs/:id/identify` (force re-id)
- `POST /api/discs/:id/start` `{ profile_id, overrides? }`
- `GET /api/profiles`, `POST /api/profiles`, `PUT /api/profiles/:id`, `DELETE /api/profiles/:id`
- `GET /api/history?type=&from=&to=&limit=`
- `GET /api/settings`, `PUT /api/settings`
- `GET /api/health`, `GET /api/version`

SSE:

- `GET /api/events` — server-sent events, one stream per client. Event
  names: `drive.changed`, `disc.detected`, `disc.identified`, `job.created`,
  `job.step`, `job.progress`, `job.log`, `job.done`, `job.failed`,
  `profile.changed`, `settings.changed`. Payload is a JSON object whose
  shape mirrors the REST resources.

Authentication: bearer-token auth is enforced when `DISCECHO_TOKEN` is
set; otherwise the API is open (LAN-only default). Internet-facing
deployments are expected to terminate TLS and inject the bearer at a
reverse proxy.

## Pipeline-engine abstraction

A *pipeline* is a list of *steps* (subset of the 8 canonical). A *handler*
is per-disc-type Go code that produces and runs the plan for a given
profile. *Tools* are reusable wrappers around external binaries.

```go
type Handler interface {
    DiscType() DiscType
    Identify(ctx context.Context, drv *Drive) (*Disc, []Candidate, error)
    Plan(ctx context.Context, disc *Disc, profile *Profile) ([]Step, error)
    Run(ctx context.Context, plan []Step, sink EventSink) error
}

type Step struct {
    ID       string                 // one of the 8 canonical step IDs
    ToolName string                 // "makemkv", "handbrake", "whipper", ...
    Args     []string
    Env      map[string]string
    Workdir  string
    Outputs  []OutputSpec
    Skip     bool
}

type EventSink interface {
    OnStepStart(stepID string)
    OnProgress(stepID string, pct float64, speed string, eta string)
    OnLog(line LogLine)
    OnStepDone(stepID string)
}

type Tool interface {
    Name() string
    Run(ctx context.Context, args []string, sink EventSink) error
}
```

What goes where:

- **Code:** stdout/stderr parsing, retry/backoff, signal handling, child-
  process lifecycles, output-file verification (sha256, sector counts).
- **Data (in `profiles`):** codec, CRF, container, output path template,
  step-enable flags, naming conventions.

For MVP the disc types implemented are listed in `ROADMAP.md`. New disc
types are added by adding a `daemon/pipelines/<type>.go` file implementing
`Handler` and registering it in `daemon/pipelines/registry.go`.

## Notifications

Notifications go through the [Apprise](https://github.com/caronc/apprise) CLI,
which gives one unified URL scheme covering ntfy, Discord, Telegram, Slack,
Pushover, email, Matrix, Gotify, and ~80 others. The daemon writes the
notification target list (rows from the `notifications` table) into a config
file at startup and on changes, then for each `job.done` / `job.failed`
event invokes `apprise -t <title> -b <body> --tag <triggers>` with the
relevant tag(s). No bespoke per-backend code in DiscEcho.

This is why the runtime image is `python:3.12-slim` rather than scratch /
distroless: Apprise is a Python package installed via `pip install apprise`.
The Go binary is copied in alongside it. (If image size becomes a concern
post-MVP, swap to the standalone `apprise-bin` build or sidecar
`apprise-api`.)

## Deployment

Single Docker container, single Go binary plus the Apprise CLI, embedded
UI assets.

```yaml
# docker-compose.yml (sketch)
services:
  discecho:
    build: .
    container_name: discecho
    restart: unless-stopped
    devices:
      - /dev/sr0:/dev/sr0
      - /dev/sr1:/dev/sr1
    group_add:
      - "${CDROM_GID}"
    volumes:
      - /srv/media:/library
      - ./data:/var/lib/discecho   # SQLite, logs
      - ./config:/etc/discecho     # tokens, provider keys
    environment:
      DISCECHO_LIBRARY: /library
      DISCECHO_DATA: /var/lib/discecho
      DISCECHO_TOKEN: ${DISCECHO_TOKEN:-}   # optional; LAN deploys leave it empty
    ports:
      - "8088:8088"
```

Reverse-proxy (Traefik / Caddy) terminates TLS upstream. PWA install works
behind a TLS-enabled hostname.

## Repo layout

```
DiscEcho/
├── ARCHITECTURE.md
├── ROADMAP.md
├── OPEN_QUESTIONS.md
├── CLAUDE.md
├── CHANGELOG.md
├── README.md                  # populated at M0
├── docker-compose.yml         # populated at M0
├── Dockerfile                 # populated at M0
├── daemon/
│   ├── cmd/discecho/main.go
│   ├── api/                   # REST + SSE
│   ├── drive/                 # udev, registry
│   ├── identify/              # probe + metadata
│   ├── pipelines/             # one file per disc type
│   ├── tools/                 # one file per external binary
│   ├── state/                 # in-mem state + SQLite
│   ├── embed/                 # //go:embed of webui build
│   └── go.mod
├── webui/
│   ├── src/
│   ├── static/
│   ├── tailwind.config.ts
│   ├── package.json
│   └── svelte.config.js
└── shared/
    └── wire.ts                # source-of-truth wire types
```
