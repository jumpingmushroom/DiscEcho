# CLAUDE.md

Project context and working agreement for Claude Code. Read this before starting any task.

## Project

- **Name:** _[fill in]_
- **Stack:** _[e.g. SvelteKit + Drizzle + SQLite, Docker Compose]_
- **Deployment:** _[e.g. Dokploy on homelab, GitHub Actions → registry → pull on host]_
- **Key commands:**
  - Dev: `_[npm run dev]_`
  - Build: `_[npm run build]_`
  - Test: `_[npm test]_`
  - Lint: `_[npm run lint]_`
  - Typecheck: `_[npm run check]_`
  - Container rebuild: `docker compose down && docker compose up -d --build`

---

## Working agreement

### Before starting

- Read the relevant files first. Don't assume structure — verify.
- For multi-file work or anything touching architecture, propose a plan and wait for confirmation before executing.
- Match existing patterns and conventions. Don't introduce new libraries, frameworks, or styling approaches without asking.

### While working

- Look for edge cases: empty inputs, null/undefined, network failures, concurrency, malformed data, large inputs, slow clients.
- Keep diffs focused. Don't refactor unrelated code while fixing a bug — that's a separate commit, ideally a separate PR.
- Comment the *why*, not the *what*. No comments that restate the code.
- No dead code, no commented-out blocks, no stray `console.log` / `print()` / `dbg!` left behind.

### Verification — in this order, before declaring done

1. **Typecheck and lint.** Fix what they flag. Don't suppress without a comment explaining why.
2. **Full test suite.** New behaviour gets new tests. Don't disable failing tests to go green.
3. **Rebuild the Docker container.** `docker compose down && docker compose up -d --build`.
4. **Tail logs through at least one full request cycle.** No errors, no warnings, no stack traces.
5. **Test in Chrome via the MCP DevTools.** For any cosmetic change, bug fix, or feature: verify actual behaviour, network calls, console output.
6. **Re-read the diff.** Anything you'd flag in code review? Fix it now.

### Branching, PRs, and merging

**Never commit to `main` directly.** Every change goes through a branch and a PR.

1. **Branch from latest `main`:**
   ```
   git switch main && git pull --ff-only
   git switch -c <type>/<short-desc>
   ```
   Branch naming: `<type>/<short-desc>` using the conventional commit prefix as the type — `feat/add-bee-tracking`, `fix/login-loop`, `chore/bump-deps`.

2. **Commit on the branch** using conventional commit prefixes: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `perf:`, `test:`, `build:`, `ci:`. Multiple commits per branch is fine — they get squashed at merge. Stylistic changes still go in their own commit. Body explains the *why* when it isn't obvious.

3. **No AI attribution** in commit messages, PR titles, or PR bodies. No `Co-Authored-By: Claude`, no "Generated with Claude Code", no 🤖 markers, no mentions of AI assistance. The committer is the human.

4. **Update `CHANGELOG.md`** in the same branch when the change is user-visible (see Changelog & versioning below).

5. **Run full local verification** (Verification section above) before opening the PR.

6. **Push and open the PR with `gh`:**
   ```
   git push -u origin HEAD
   gh pr create --base main \
     --title "feat: add bee tracking dashboard" \
     --body "What changed, why, and how to test."
   ```
   PR title = the conventional commit message you want on `main` after squash.

7. **Queue auto-merge immediately:**
   ```
   gh pr merge --squash --auto --delete-branch
   ```
   GitHub merges the moment CI is green and deletes the branch. If CI fails, fix on the branch and push again — auto-merge stays queued and re-evaluates on each run.

8. **After merge, sync local `main`:**
   ```
   git switch main && git pull --ff-only
   ```

### Branch protection on `main`

`main` is protected server-side, not just by convention. Configure once per repo (Settings → Branches, or `gh api`):

- Require a PR before merging.
- Require status checks to pass before merging — pin the CI workflow(s).
- Require linear history (enforces squash/rebase, blocks merge commits).
- Block force pushes and branch deletion.
- Apply rules to repository administrators (no self-bypass).

### When stuck

- If something fails twice the same way, stop. Don't loop. Report what you tried and ask.
- If a fix requires changes outside the original scope, surface it before doing the work.
- If the test suite was already broken when you arrived, say so — don't silently disable or skip tests to make your change look clean.

---

## Tools and skills

These tools and skills are part of the workflow. Reach for them deliberately.

### Documentation lookup — Context7 MCP

Before using a library, framework, CLI flag, function signature, or API endpoint that you don't have rock-solid current knowledge of, look it up via Context7. This applies to `gh`, `docker`, `kubectl`, `npm`, framework APIs, library functions, ORM query builders — anything where memory might be older than the current version.

Don't guess at flags or signatures and patch later. Stale or hallucinated APIs are the most common source of "looks right, doesn't run" code. One Context7 lookup is cheaper than one round of failed builds.

### Planning — Superpowers `brainstorming`

For any task that needs more than minor planning — architecture choices, multi-file changes with cross-cutting concerns, ambiguous requirements, anything where it's worth scoping before writing — use `brainstorming` to surface options and questions before executing.

### Deep planning — Superpowers `grill-me`

Use `grill-me` only for **big** changes where assumptions are likely to be wrong and expensive to discover late: new features touching multiple subsystems, large refactors, schema or auth changes, anything where missed requirements turn into rework.

Skip it for routine bug fixes, small features, and anything where `brainstorming` already covered the question set. If unsure, start with `brainstorming` and escalate to `grill-me` only when the open questions keep multiplying.

### Output verbosity — Superpowers `caveman`

Use `caveman` to keep responses terse when there's no real explanation needed: short status updates, simple confirmations, mechanical tasks where the diff or output is self-evident.

**Never use `caveman` during `brainstorming` or `grill-me` rounds.** Those depend on full prose to be useful — terse output defeats the purpose. Same goes for any explanation involving trade-offs, security reasoning, or architectural decisions.

---

## Code quality

- Small, focused functions. If you're scrolling to read one, it's too long.
- Single source of truth — no duplicated business logic.
- Validate inputs at the trust boundary; trust them after.
- Errors: fail loud in dev, fail gracefully in prod, always with context in logs.
- No `any` in TypeScript without a comment explaining why. No `// @ts-ignore` / `# type: ignore` without a linked issue or short justification.

---

## Security & secrets

- Never commit secrets. `.env`, `.env.local`, and equivalents are gitignored.
- `.env.example` is the source of truth for required variables — update it whenever you add a new one.
- Never log tokens, passwords, full request bodies, or PII.
- Before adding a dependency: check last commit date, weekly downloads, and known CVEs. Prefer the standard library or an existing dep when the case is covered.

---

## Database & migrations

- Schema changes go through migrations. Never edit a migration that's been merged.
- Destructive migrations (drop column, drop table, lossy type changes) require explicit confirmation.
- Test the migration up *and* down locally before committing.
- New queries on large tables need an indexing review.

---

## Documentation

- Update `README.md` in the same commit when you add setup steps, env vars, or commands.
- Update API/route docs when endpoints change.
- Non-obvious workarounds get a comment with the reason and (if applicable) a link to the issue.

---

## Changelog & versioning

- `CHANGELOG.md` follows the [Keep a Changelog](https://keepachangelog.com/) format. Keep it current — every user-visible change lands in the changelog in the same commit as the code.
- New entries go under `## [Unreleased]` at the top, grouped by category in this order: **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, **Security**.
- One bullet per change. Write for someone reading release notes, not for the developer who wrote the code.
- Internal-only refactors, test changes, and CI tweaks don't need a changelog entry. If a user wouldn't notice, leave it out.
- Versioning follows [SemVer](https://semver.org/) — `MAJOR.MINOR.PATCH`:
  - **MAJOR** — breaking changes (API, schema, config, behaviour users depend on).
  - **MINOR** — new functionality, backwards compatible.
  - **PATCH** — backwards-compatible bug fixes.
- On release: rename `[Unreleased]` to the new version with the ISO date (`## [1.2.0] - 2026-05-06`), insert a fresh empty `[Unreleased]` block above it, tag the release commit (`git tag -a v1.2.0 -m "v1.2.0"`), and push the tag.
- Pre-`1.0.0`: breaking changes can ship in MINOR bumps. Call them out explicitly in the changelog entry.

---

## What not to do

- Don't bypass the verification steps to "just push a quick fix."
- Don't commit to `main` directly. Every change goes through a branch and a PR — no exceptions, including docs, typos, and "trivial" fixes.
- Don't disable tests, lint rules, or type checks to get something green.
- Don't merge a PR with failing CI. Fix the branch instead.
- Don't introduce a new library when an existing one covers the case.
- Don't mix stylistic and functional changes in the same commit.
- Don't add AI attribution anywhere — commit messages, PR descriptions, code comments, file headers, READMEs. No `Co-Authored-By: Claude`, no "Generated with Claude Code", no 🤖.
- Don't ship a user-visible change without a `CHANGELOG.md` entry in the same branch.
- Don't guess at API signatures, CLI flags, or library behaviour. Look it up via Context7 first.
- Don't reach for `grill-me` on small changes — `brainstorming` is the default for planning.
- Don't use `caveman` during `brainstorming` or `grill-me` rounds, or for explanations involving trade-offs, security, or architecture.