## What

<!-- One paragraph: what does this PR do, and why? -->

## Conventional commit prefix

- [ ] PR title starts with one of: `feat:` `fix:` `chore:` `refactor:`
      `docs:` `perf:` `test:` `build:` `ci:`

## Verification

- [ ] `golangci-lint run` clean (or no Go changes)
- [ ] `go test ./...` passes (or no Go changes)
- [ ] `pnpm check` and `pnpm build` clean (or no UI changes)
- [ ] `docker compose up -d --build` succeeds (or no infra changes)
- [ ] Manual UI verification done in Chrome (for any UI change)

## Changelog

- [ ] User-visible change has an entry under `## [Unreleased]` in
      `CHANGELOG.md`, OR
- [ ] N/A — internal-only change

## Notes for reviewer

<!-- Anything tricky, surprising, or worth flagging. -->
