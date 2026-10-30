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

## Status (2026-10-30 night)

- v0.1.0 is READY but NOT released: tagging and publishing need Ryan's go-ahead (blocked by a permission
  check). To release: `git tag -a v0.1.0 -m v0.1.0 && git push origin v0.1.0 &&
  GITHUB_TOKEN=$(gh auth token) goreleaser release --clean` (goreleaser v2.12.7 is in ~/go/bin;
  `goreleaser release --snapshot --clean` already builds all 6 archives).
- All tracks landed. Fresh review: 16 findings, fixed per DESIGN 24-31; reviewer repros rerun clean
  (except `--private` by hand, which is allowed for the person; the hook blocks it for agents).
- `go test ./...` green. `sh tools/claude_smoke.sh`: 4/4 on Claude Code 2.1.314 (Linux).
- Success check 3 passed (DESIGN 23).

## For Ryan

- Approve the v0.1.0 tag and release (above).
- Then test the curl install on the Mac mini, and run `sh tools/claude_smoke.sh` there (needs go, claude).
- Merge and push the local `ci-workflows` branch with a token that has `workflow` scope.
- Windows: install.ps1 never run; Smart App Control and code signing are your call.
- Sandbox snippet in `init` output is from docs, untested.

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
