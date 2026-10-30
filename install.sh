#!/bin/sh
# install.sh: install standin on macOS or Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh
#
# Options (put them after "sh -s --"):
#   --version vX.Y.Z   install that release (default: the latest release)
#   --bin-dir DIR      install into DIR (default: ~/.local/bin)
#
#   curl -fsSL https://raw.githubusercontent.com/RKelln/standin/main/install.sh | sh -s -- --version v0.1.0
#
# It never asks for sudo or an admin password. It checks the download against the
# release's checksums.txt and stops if they do not match. Downloading with curl
# (not a web browser) means macOS does not block the program as "from the internet".

set -eu

REPO="RKelln/standin"
BIN_NAME="standin"
BIN_DIR="${STANDIN_BIN_DIR:-}"
VERSION="${STANDIN_VERSION:-latest}"

if [ -t 1 ]; then
  BOLD="$(printf '\033[1m')"; RED="$(printf '\033[31m')"; GREEN="$(printf '\033[32m')"; RESET="$(printf '\033[0m')"
else
  BOLD=""; RED=""; GREEN=""; RESET=""
fi

say()  { printf '%s\n' "$*"; }
fail() { printf '%sStopped:%s %s\n' "${BOLD}${RED}" "${RESET}" "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || fail "--version needs a value, like v0.1.0"; VERSION="$2"; shift ;;
    --bin-dir) [ $# -ge 2 ] || fail "--bin-dir needs a folder"; BIN_DIR="$2"; shift ;;
    -h|--help)
      say "Usage: install.sh [--version vX.Y.Z] [--bin-dir DIR]"
      exit 0 ;;
    *) fail "Unknown option: $1 (try --help)" ;;
  esac
  shift
done

case "$(uname -s)" in
  Darwin) OS="Darwin" ;;
  Linux)  OS="Linux" ;;
  MINGW*|MSYS*|CYGWIN*) fail "This is Windows. In PowerShell, run: irm https://raw.githubusercontent.com/RKelln/standin/main/install.ps1 | iex" ;;
  *) fail "This computer's system ($(uname -s)) is not supported yet." ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) fail "This computer's chip type ($(uname -m)) is not supported yet." ;;
esac

command -v curl >/dev/null 2>&1 || fail "This needs the 'curl' program, and it is not here."
command -v tar  >/dev/null 2>&1 || fail "This needs the 'tar' program, and it is not here."
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  fail "This needs 'sha256sum' or 'shasum' to check the download, and neither is here."
fi

[ -n "${HOME:-}" ] || fail "HOME is not set, so there is no home folder to install into. Use --bin-dir DIR."
[ -n "$BIN_DIR" ] || BIN_DIR="$HOME/.local/bin"

ARCHIVE="${BIN_NAME}_${OS}_${ARCH}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  # The /latest/download/ link avoids the GitHub API, which limits how often one
  # network (say, a workshop room) may call it.
  BASE_URL="https://github.com/${REPO}/releases/latest/download"
else
  case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
  BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
fi

say ""
say "${BOLD}Installing standin${RESET} (${VERSION}, ${OS} ${ARCH})"
say "  into: ${BIN_DIR}"
say ""

TMP="$(mktemp -d 2>/dev/null || mktemp -d -t standin)"
trap 'rm -rf "$TMP"' EXIT INT TERM

say "Downloading ${ARCHIVE} ..."
curl -fsSL -o "$TMP/$ARCHIVE" "$BASE_URL/$ARCHIVE" \
  || fail "Could not download $BASE_URL/$ARCHIVE. Check your internet connection, or the version number."
curl -fsSL -o "$TMP/checksums.txt" "$BASE_URL/checksums.txt" \
  || fail "Could not download the checksum list for this release, so the download cannot be checked."

say "Checking the download ..."
expected="$(awk -v f="$ARCHIVE" '$2 == f || $2 == "*" f {print $1; exit}' "$TMP/checksums.txt")"
[ -n "$expected" ] || fail "The checksum list does not name $ARCHIVE. Nothing was installed."
actual="$(sha256 "$TMP/$ARCHIVE")"
[ "$expected" = "$actual" ] || fail "The download does not match its checksum (it may be damaged or changed). Nothing was installed."

tar -xzf "$TMP/$ARCHIVE" -C "$TMP" "$BIN_NAME" || fail "Could not unpack $ARCHIVE. Nothing was installed."
[ -f "$TMP/$BIN_NAME" ] || fail "The download did not contain the standin program. Nothing was installed."

mkdir -p "$BIN_DIR" 2>/dev/null || fail "Could not make the folder $BIN_DIR. Pick another with --bin-dir."
[ -w "$BIN_DIR" ] || fail "You cannot write to $BIN_DIR. Pick a folder you own with --bin-dir (this script never uses sudo)."

chmod 755 "$TMP/$BIN_NAME"
mv -f "$TMP/$BIN_NAME" "$BIN_DIR/$BIN_NAME" || fail "Could not put standin into $BIN_DIR."

# Belt and braces: clear the macOS quarantine flag if anything set it.
if [ "$OS" = "Darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$BIN_DIR/$BIN_NAME" 2>/dev/null || true
fi

installed="$("$BIN_DIR/$BIN_NAME" version 2>/dev/null || true)"
say ""
say "${BOLD}${GREEN}Done.${RESET} standin is installed at $BIN_DIR/$BIN_NAME"
[ -z "$installed" ] || say "  $installed"
say ""
say "${BOLD}Next steps${RESET}"

step=1
case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *)
    case "${SHELL:-}" in
      */zsh)  profile="$HOME/.zprofile" ;;
      */bash) if [ "$OS" = "Darwin" ]; then profile="$HOME/.bash_profile"; else profile="$HOME/.bashrc"; fi ;;
      *)      profile="$HOME/.profile" ;;
    esac
    say "  $step. Your terminal does not look in $BIN_DIR yet. Run this once:"
    say ""
    say "       echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> $profile"
    say ""
    say "     Then close this terminal window and open a new one."
    step=$((step + 1))
    ;;
esac
say "  $step. Check it works:   standin version"
step=$((step + 1))
say "  $step. Ask your AI agent to help you set up standin in your project folder."
say "     Your real files go in raw/. The AI only reads scrubbed/."
say ""
say "  Keep your private table (the mapping that links stand-ins back to real values)"
say "  to yourself. Standin stores it outside your project, in ~/.standin."
say ""
