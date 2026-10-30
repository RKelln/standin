# Releasing standin

GoReleaser builds and publishes a release when a tag starting with `v` is pushed
(`.github/workflows/release.yml`). It builds linux, macOS and Windows for amd64 and arm64
(`CGO_ENABLED=0`, `-trimpath`, version and commit set by ldflags), makes `.tar.gz` archives
(`.zip` for Windows), writes `checksums.txt` (SHA-256), and attaches them all to a GitHub Release.
The release notes come from the git log; commits starting `docs:`, `test:` or `chore:` are left out.

## Before you tag

```sh
make ci                 # gofmt check, vet, test, no-network check, lint (if installed), build
make release-check      # goreleaser check (needs goreleaser installed)
make release-snapshot   # build every platform into dist/ without publishing
./dist/standin_linux_amd64_v1/standin version   # path varies by goreleaser version
```

## Cut a release

```sh
git checkout main && git pull
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
gh run watch --repo RKelln/standin   # follow the Release workflow
```

For a trial run, tag a release candidate (`v0.1.0-rc.1`). GoReleaser marks it as a prerelease.
`install.sh` with no `--version` installs the latest *full* release, so test a candidate with:

```sh
curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh -s -- --version v0.1.0-rc.1
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

Using curl (not a browser) means macOS does not quarantine the binary, so Gatekeeper does not block it.
A binary downloaded by a browser would need `xattr -d com.apple.quarantine standin`.
The binaries are not signed or notarized yet.

## Later

- Homebrew tap and Scoop bucket (`brews:` and `scoops:` in `.goreleaser.yaml`, as in agentmap). These
  need the tap and bucket repos and a `GH_PAT` secret with push access to them; then switch
  `release.yml` from `secrets.GITHUB_TOKEN` to `secrets.GH_PAT`.
- macOS signing and notarization; Windows code signing.
- A `CHANGELOG.md`.
