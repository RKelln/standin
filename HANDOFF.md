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

## In flight (background agents, started 2026-10-30; none committed)

- R research: `docs/research/prior-art.md`. Prior art verified at source; plus hooks (Claude Code
  PreToolUse `updatedInput` redirect of raw reads to scrubbed copies? PostToolUse output replacement for
  built-in tools? crash = fail open or closed?), Codex/OpenCode, Gatekeeper, QuickBooks/Xero/Wave/OFX formats.
- A core CLI: `cmd/standin`, `internal/...`, unit tests (no `hook`/`init` yet).
- C fixtures + e2e: `testdata/`, `tools/make_fixtures.py` (uv), `e2e/` (skips if CLI not built).
- D release: `.goreleaser.yaml`, `install.sh`, `install.ps1`, `Makefile`, `.github/`, `LICENSE`, `.gitignore`.

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
