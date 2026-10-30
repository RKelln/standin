# Handoff (orchestrator state, 2026-10-30)

Read `AGENTS.md`, `docs/PLAN.md` (the contract), `docs/DESIGN.md`.

## Ryan's steer (2026-10-30)

- Go CLI, stdlib only, one static binary; called mainly by agent-harness hooks; setup hands-on.
  Replaces the one-HTML-file MVP (`AGENTS.md` updated).
- Needed "relatively useful" by the Get2Know workshop, 2026-11-02. Good on synthetic data first.
- Workshop Macs install via `curl` (no Gatekeeper quarantine) and have recent Python.
- Name stays `standin`; textmark `[standin]`. MIT for all (code and docs).
- Release pipeline cribbed from RKelln/agentmap (clone in the session scratchpad `ref/agentmap`).
- xlsx not needed for MVP; accounting/bank exports (QuickBooks, OFX/QFX) matter more.
- Agents using standin should report issues and fixes, data-free (`standin report`, plus skill text),
  so a leak does not recur.
- Work in `main` and branches; repo https://github.com/RKelln/standin (public).
- Shapes are setup-only; day to day the agent sees scrubbed files only. Harness deny-configs are
  "not foolproof but help": ship them for Claude Code, Codex, OpenCode.

## Status (2026-10-30 evening)

- Landed and pushed: research, release pipeline (minus workflows, see below), fixtures + e2e, core CLI,
  hooks + init + skill. `go test ./...` green. Real Claude Code smoke test: `sh tools/claude_smoke.sh`
  (4/4 on 2.1.314). Success check 3 passed (DESIGN decision 23).
- Track V review done: 16 findings (4 high). Fix decisions are DESIGN 24-31. Fixes in flight: the hook
  builder (internal/hook, initcmd, skill) and the core builder (cli + core packages), in parallel.
  Then: rerun `go test ./...`, `sh tools/claude_smoke.sh`, then tag v0.1.0 and publish with a local
  `goreleaser release` (the token cannot push workflows, so CI is not on GitHub yet).

## Next (orchestrator)

1. As each lands: read its report, run `gofmt -l . && go vet ./... && go test ./...`, reconcile any
   contract ambiguities into `docs/PLAN.md`, commit.
2. When R lands: record decisions in `docs/DESIGN.md` "Decisions" (CLI pivot, token format `[CATEGORY_N]`,
   private store outside project, first-spelling restore, coarse shape bands, known-values scan in MVP,
   Go over Rust/Python, MIT for all). Then brief track B: `standin hook` (Claude Code first, fail closed),
   `standin init` (folders + harness configs + AGENTS.md lines), `skill/` text incl. `standin report`.
3. Track V: a fresh review agent (leaks, fail-closed, default-deny, cross-track drift), then fixes.
4. Success check 3 (a small model proposes a config from shape only); record results here.
5. Tag v0.1.0; Ryan tests curl install on the Mac mini. Update README status.

## Blocked on Ryan

- The PAT cannot push workflows. CI and release workflows are on the local branch `ci-workflows` (not pushed); Ryan merges and pushes them. Until then, releases are built locally (`make` cross-builds plus `gh release`).
- Windows Smart App Control may block unsigned binaries; paid code signing is Ryan's call.
