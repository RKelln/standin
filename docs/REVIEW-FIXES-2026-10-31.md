# Review fixes — 2026-10-31

This records changes following [the review](REVIEW-2026-10-31.md) of `cf62164`. Three subagents
implemented separate parts; the coordinator reviewed the integration and added further regressions.
The source changes are local and have not been released. All test data is invented.

## Findings addressed

| Review | Change | Regression evidence |
| --- | --- | --- |
| R1 | Pin output directory handles; refuse linked roots, ancestors and nested output links. Restore cannot write through the reported link into scrubbed/. | `TestReviewRestoreRejectsNestedOutputLink`, boundary tests in `internal/store` |
| R2 | Resolve private aliases, including the longest existing parent; check physical project overlap and case-fold paths where needed. | `TestReviewPrivateAliasCannotLeadIntoProject` covers both private flags and home override |
| R3 | Cross-process lease before reading private state, held through publication. Detect interrupted saves and refuse missing mappings over existing outputs. | Sixteen concurrent scrubs retain sixteen mappings and restore each own value; cross-process lease and killed-writer tests |
| R4 | Refuse reserved literal JSON field names rather than merge distinct paths. | Reserved-key tests plus CLI refusal/no-output test |
| R5 | Refuse duplicate CSV/TSV and within-one-table Markdown headers, including collisions created by rewriting. | Input and rewritten-header collision tests; CLI refusal/no-output test |
| R6 | Validate every JSON key, including empty-container keys; include empty containers in inventory. | Every-key and empty-container tests; CLI refusal/no-output test |
| R7 | Single-word name checks on filenames, checks against batch values, and withheld preparation messages. Initial parse refusals do not name the raw file. | Filename tests cover a first name, common-word name, malformed input and stand-in refusal |
| R8 | Check the fully assembled restored output for newly created formulas, URLs and tags. Leave hazardous substitutions tokenized with warnings. Refuse active HTML containers in text. | Formula, joint-token, link/tag and script/style/body tests |
| R9 | Strict JSON before/after restore, including duplicate keys; strict table quoting/row width and lone CR refusal. | Malformed input and duplicate-result-key tests; multi-file refusal writes no output |
| R10 | Preflight all setup destinations, stage files, roll back new empty folders, and report late failures truthfully. | Blocked-destination snapshots, linked-destination refusal, injected write failure |

See the `review` tests in [CLI](../internal/cli/), [dataset](../internal/dataset/review_test.go),
[store](../internal/store/review_test.go), [restore](../internal/restore/review_test.go), and
[init](../internal/initcmd/review3_test.go). The old diagnostic probes are historical reproductions;
the new tests assert the safe outcome.

## Additional review gaps addressed

- **Interrupted publication:** keep a pending marker and previous private files, sync Unix directories,
  use atomic rollback replacements, report failed rollback, and block private readers/hooks until
  recovery. A killed subprocess and injected publication/rollback failures exercise these paths.
- **Windows permissions:** use protected Windows access lists for the current account, not only Unix
  mode bits. Empty temporary files are secured before private bytes are written; failures refuse.
  Native execution still needs Windows CI; cross-compilation is not ACL validation.
- **Private/config JSON integrity:** shared checks reject duplicate names, trailing content, malformed
  Unicode and excess depth. Mapping rows must agree with their spellings and have complete consecutive
  counters. Approval hashes must be well formed. Error reports accept only built-in error labels and
  valid timestamps from the private log, so edited log text cannot export arbitrary secrets.
- **Terminal checks:** query the actual OS terminal/console, refusing unrelated character devices.
  A pseudo-terminal remains controllable by software and is not proof of a person.
- **Hook coverage:** match every tool, allow a small explicit conversation/task-list set, and refuse
  unknown tools including MCP tools. PreToolUse also refuses interrupted private saves before allowing
  scrubbed reads. The shell check remains a heuristic, not OS isolation.
- **Release/CI:** add pinned native Linux/macOS/Windows CI, cross-platform archives, a manual snapshot
  workflow, and a separately gated tag-release workflow. The live smoke script now requires evidence
  of the actual requested tool, path, hook event and exit status. Its offline self-tests make no model
  calls. No workflow was pushed or run remotely during this task.
- **Honest scope:** README, PLAN, DESIGN, setup messages and release instructions distinguish runtime
  dependency checks, external setup commands, unverified native behavior and interrupted-write recovery.

Implementation decisions and reasons are recorded in DESIGN decisions 59 onward. The current
contract is [PLAN](PLAN.md); the initial browser flow is explicitly marked historical.

## Verification

On Linux amd64 with Go 1.26.4:

| Check | Result |
| --- | --- |
| `go test ./... -count=1` | Passed |
| `go test -race ./... -count=1` | Passed, including all final regressions |
| `go vet ./...` | Passed |
| `make fmt-check no-network` and `git diff --check` | Passed |
| `sh -n install.sh tools/claude_smoke.sh` | Passed |
| `sh tools/claude_smoke.sh --self-test` | Passed; no model calls |
| `CGO_ENABLED=0 go build -trimpath` | Passed for Linux/macOS/Windows, amd64 and arm64 |
| Windows store test binaries | Cross-compiled for amd64 and arm64; not executed |

One race run found an existing leak-check flake: a random generated test path contained the fixture
amount `47600`. The check now removes only its exact generated directory paths before testing for
fixture values; it still checks every filename and actual message. The full race suite then passed.
The e2e harness now fails if it cannot build the CLI, rather than silently skipping the suite.
No GoReleaser or optional linter was installed here; archive configuration and hosted execution are
covered by the added workflows but have not run in this session.

Source checks used the [Go directory-boundary reference](https://pkg.go.dev/os#Root) and Microsoft's
[SetSecurityInfo](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo)
and [descriptor access-list reference](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-getsecuritydescriptordacl).
These informed the implementation; they do not substitute for native execution tests.

## Remaining limits

These are limits, not claims that testing established protection:

- No native macOS or Windows session, installed release, hosted workflow, real-user approval flow,
  live Claude session or real records were used here. The new CI makes native checks runnable when
  pushed; that does not mean they have run.
- A process kill leaves a lease or pending marker and is tested. Disk failure and power-loss recovery
  are not proven; Windows does not use Unix directory `fsync`. Recovery needs a matching generation
  of private state and scrubbed data. Previous private files alone are not a project backup.
- An unrestricted process running as the same OS user can control a pseudo-terminal, move an already
  open directory, change files, or read private files. File boundaries and hooks do not replace a
  sandbox around the agent. Administrators can take ownership of private files.
- The dependency check proves the current binary links no network client package. Installers download
  releases, and setup runs the external `claude --version` command. There was no runtime network trace.
- Name detection remains incomplete. Unknown names, common-word names, identifying combinations and
  structure/frequency can still disclose information. Active-content checks are conservative exposure
  checks, not a general HTML/script or spreadsheet sanitizer.
- No release was published, no program installed, and no repository changes committed by this task.
