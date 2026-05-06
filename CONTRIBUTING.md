# Contributing to DiscEcho

Welcome, and thanks for looking. DiscEcho is a self-hosted homelab service
that watches optical drives, classifies inserted discs, and runs per-disc-
type rip → transcode → tag → move pipelines, fronted by a mobile-first web
UI. The full design lives in [`ARCHITECTURE.md`](./ARCHITECTURE.md) — start
there before opening a PR.

## Dev setup

Detailed setup will live in `README.md` once we have one (tracked in
[`ROADMAP.md`](./ROADMAP.md), M0). For now: you need Go, Node, Docker,
and a Linux host with at least one optical drive at `/dev/sr0`.

## Branching and commits

- Trunk-based. Short-lived feature branches off `main`, named
  `<type>/<short-desc>` (e.g. `feat/audiocd-pipeline`, `fix/udev-races`).
- [Conventional Commits](https://www.conventionalcommits.org/): `feat:`,
  `fix:`, `chore:`, `docs:`, `refactor:`, `test:`, `ci:`, `perf:`,
  `build:`. The PR title is the squashed commit message — keep it
  imperative and under ~70 characters.
- PRs squash-merge into `main`. Branch protection blocks direct pushes
  and requires CI green.
- No AI attribution in commits, PR bodies, or code comments.

## Code style

Defer to [`.editorconfig`](./.editorconfig) and the language formatters:

- Go: `gofmt` (or `goimports`) on save. `go vet` clean.
- SvelteKit / TypeScript / CSS / JSON / YAML / Markdown: `prettier`.
- Other: respect `.editorconfig`.

No ESLint/Go-lint rule is sacred — if it makes the code worse, file an
issue and we'll discuss.

## Testing

Every PR that changes behaviour includes or updates tests. CI runs
typecheck + lint + the full test suite on every push, and must be green
before a PR can merge. Don't disable tests to make CI pass — fix the
underlying problem, or say so explicitly in the PR.

## Filing issues

**Bugs:** include your environment (OS, kernel, Docker version), drive
model + bus, disc type and (if applicable) the title, exact steps to
reproduce, and the relevant daemon log lines.

**Feature requests:** lead with the use case ("when I rip a [...], I want
[...] because [...]") and only then propose a solution. We're more likely
to merge a small change to a real problem than a big change to a guessed
one.

## Good first issues

Issues tagged `good first issue` are small, well-scoped, and a fine entry
point. The other place opinions are very welcome is
[`OPEN_QUESTIONS.md`](./OPEN_QUESTIONS.md) — answers, alternatives, and
"have you considered…" comments are all useful, even if you're not
writing code.

## License and sign-off

> TODO: choose and document a license. Until then, contributors agree
> that their contributions will be licensed under the project's eventual
> license, whatever it turns out to be. We'll backfill DCO sign-off
> requirements (`Signed-off-by:` trailers) when the license is settled.

## Code of conduct

> TODO: a `CODE_OF_CONDUCT.md` will land before the first public release.
> In the meantime: be kind, assume good faith, and stay focused on the
> work.
