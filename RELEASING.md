# Releasing standin

GoReleaser builds and publishes a release when a tag starting with `v` is pushed
(`.github/workflows/release.yml`). It builds linux, macOS and Windows for amd64 and arm64
(`CGO_ENABLED=0`, `-trimpath`, version and commit set by ldflags), makes `.tar.gz` archives
(`.zip` for Windows), writes `checksums.txt` (SHA-256), and attaches them all to a GitHub Release.
The release notes come from the git log; commits starting `docs:`, `test:` or `chore:` are left out.

The workflows are now in this checkout. They must be pushed with permission to update GitHub Actions
before they can run. CI runs native Go tests on Linux, macOS and Windows, races on Linux and macOS,
module-graph and no-network checks, and builds the six release archives as snapshots. The shipped
binary uses one vendored character-set module; the mail-parser dependency is test-only.
The GoReleaser version is fixed at v2.11.2; action references use full commit hashes. Review and update
those pins deliberately. Sources for the pins are the upstream release pages for
[checkout v5.0.0](https://github.com/actions/checkout/releases/tag/v5.0.0),
[setup-go v6.0.0](https://github.com/actions/setup-go/releases/tag/v6.0.0),
[goreleaser-action v7.0.0](https://github.com/goreleaser/goreleaser-action/releases/tag/v7.0.0), and
[upload-artifact v4.6.2](https://github.com/actions/upload-artifact/releases/tag/v4.6.2).

These workflows have not yet run in GitHub Actions as part of the review fixes. Native macOS/Windows
behavior, hosted-runner setup, installers and live Claude checks remain unverified until those runs
are recorded. Some pseudo-terminal tests are Linux-only and skip on other systems; a green macOS or
Windows job does not cover those skipped cases.

## Dry run without publishing

After pushing the workflow files, use GitHub Actions' **Run workflow** button on **Release**, or:

```sh
gh workflow run release.yml --ref main
```

A manual run always takes the snapshot path: tests run, all platform archives build, and archives plus
checksums are retained as the `standin-snapshot` artifact for seven days. It has no publishing token
and makes no GitHub Release. A `v*` tag push takes the separate publishing path after Linux checks pass.
Wait for the full native CI matrix on that commit before pushing a release tag.

## Before you tag

```sh
make ci                 # format, vet, tests, no-network, tidy, optional lint, build
make release-check      # goreleaser check (needs goreleaser installed)
make release-snapshot   # build every platform into dist/ without publishing
./dist/standin_linux_amd64_v1/standin version   # path varies by goreleaser version
sh tools/claude_smoke.sh --self-test          # offline audit-helper tests; no model calls
```

Run `sh tools/claude_smoke.sh` separately for live Claude integration testing. It uses invented
fixtures and makes paid model calls; it is not run by CI. It now requires the actual requested hook
event, tool, file/command and exit status to appear in a local audit. Successful redirect checks also
need the expected scrubbed path. Authentication failures, missing tool calls and unrelated failures
cannot pass merely because a real name is absent. The wrapper removes the raw Read deny rule only
inside its disposable synthetic project so the hook redirect is exercised; it checks that init wrote
the deny rule first. The broken-hook case requires an audited PreToolUse exit 1 plus `onFailure` output.
This still tests a small fixed set of tools, not arbitrary shell commands or MCP integrations.

## Cut a release

Choose a new version before running these commands; replace `vX.Y.Z` throughout. Do not reuse an
existing tag.

```sh
git checkout main && git pull
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
gh run watch --repo RKelln/standin   # follow the Release workflow
```

For a trial run, tag a release candidate (`vX.Y.Z-rc.1`). GoReleaser marks it as a prerelease.
`install.sh` with no `--version` installs the latest *full* release, so test a candidate with:

```sh
curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh -s -- --version vX.Y.Z-rc.1
```

## After the release: check the install

macOS and Linux (installs to `~/.local/bin`, no sudo):

```sh
curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh
standin version
```

Windows PowerShell (installs to `%LOCALAPPDATA%\Programs\standin`, adds it to the user PATH):

```powershell
irm https://raw.githubusercontent.com/RKelln/standin/main/install.ps1 | iex
standin version
```

The binaries are not signed or notarized yet. macOS quarantine and Windows execution policy depend
on how the package is downloaded and the machine's settings. Test the actual install path on the
target machine; a successful cross-build does not establish that its security policy will run it.

## Later

- Homebrew tap and Scoop bucket (`brews:` and `scoops:` in `.goreleaser.yaml`, as in agentmap). These
  need the tap and bucket repos and a `GH_PAT` secret with push access to them; then switch
  `release.yml` from `secrets.GITHUB_TOKEN` to `secrets.GH_PAT`.
- macOS signing and notarization; Windows code signing.
- A `CHANGELOG.md`.
