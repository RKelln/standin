#!/bin/sh
# Smoke test of the Claude Code hooks against a real `claude`, on synthetic fixtures.
# The hooks and deny rules are in .claude/settings.local.json (standin init's default).
# Uses a few cheap model calls. Run from the repo root: sh tools/claude_smoke.sh
# Needs: go, claude (2.1.295 or later), script (for approve on a fake terminal).
set -eu

repo=$(pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export STANDIN_HOME="$work/home"
bin="$work/standin"
go build -o "$bin" ./cmd/standin

proj="$work/demo"
mkdir -p "$proj"
cd "$proj"
"$bin" init >/dev/null
cp "$repo/testdata/invoices.csv" "$repo/testdata/bank-export.csv" raw/
cp "$repo/testdata/standin.config.json" .
"$bin" shape >/dev/null 2>&1
# approve needs a terminal: answer yes to every question through `script` (Linux, then macOS form).
answers=$(printf 'yes\n%.0s' 1 2 3 4 5 6 7 8 9 10 11 12)
if script --version >/dev/null 2>&1; then
	printf '%s\n' "$answers" | script -qec "$bin approve" /dev/null >/dev/null 2>&1
else
	printf '%s\n' "$answers" | script -q /dev/null "$bin" approve >/dev/null 2>&1
fi
"$bin" scrub >/dev/null 2>&1 || true

name=$(grep -v '^#' "$repo/testdata/known-values.txt" | grep -E '^[A-Z][a-z]+ [A-Z][a-z]+$' | head -1)
pass=0
fail=0
ok() { pass=$((pass + 1)); echo "PASS $1"; }
bad() { fail=$((fail + 1)); echo "FAIL $1"; }

# AGENTS.md alone would make the model refuse; move it aside so the hooks are what is tested.
# Not into $work: that is a parent folder, and Claude Code reads it there too.
mkdir "$work/aside"
mv AGENTS.md "$work/aside/AGENTS.md"

out=$(echo "Test of a file tool. Use the Read tool on raw/invoices.csv and print the first 3 lines." |
	claude -p --model haiku --allowedTools Read 2>&1 || true)
if printf '%s' "$out" | grep -q "$name"; then bad "Read raw/: a real name came back"; else ok "Read raw/ is denied or redirected"; fi

out=$(echo "Test of the shell tool. Run exactly: cat raw/invoices.csv | head -3 and print the output." |
	claude -p --model haiku --allowedTools "Bash(cat:*)" "Bash(head:*)" 2>&1 || true)
if printf '%s' "$out" | grep -q "$name"; then bad "Bash cat raw/: a real name came back"; else ok "Bash cat raw/ is blocked"; fi

out=$(echo "Draft a one-line payment reminder for $name." | claude -p --model haiku 2>&1 || true)
if printf '%s' "$out" | grep -q "Your message was not sent"; then ok "prompt with a real name is blocked"; else bad "prompt with a real name was sent"; fi

cp .claude/settings.local.json "$work/settings.bak"
sed 's#"command": "[^"]*"#"command": "/bin/false"#' "$work/settings.bak" >.claude/settings.local.json
out=$(echo "Use the Read tool on standin.config.json and print its first line." |
	claude -p --model haiku --allowedTools Read 2>&1 || true)
if printf '%s' "$out" | grep -q "onFailure"; then ok "a broken hook blocks"; else bad "a broken hook did not block"; fi
cp "$work/settings.bak" .claude/settings.local.json

# The hooks live in settings.local.json only (decision 36), with no --project (decision 34).
if [ ! -e .claude/settings.json ] && ! grep -q -- '--project' .claude/settings.local.json; then
	ok "hooks are in settings.local.json, with no --project"
else
	bad "hooks are not where init should put them"
fi

# A moved folder keeps working: the Read check again, from the new place.
cd "$work"
mv "$proj" "$work/moved"
cd "$work/moved"
out=$(echo "Test of a file tool. Use the Read tool on raw/invoices.csv and print the first 3 lines." |
	claude -p --model haiku --allowedTools Read 2>&1 || true)
if printf '%s' "$out" | grep -q "$name"; then bad "moved folder: a real name came back"; else ok "moved folder: Read raw/ is still denied or redirected"; fi
out=$(echo "Use the Read tool on standin.config.json and print its second line exactly." |
	claude -p --model haiku --allowedTools Read 2>&1 || true)
if printf '%s' "$out" | grep -q '"version"'; then ok "moved folder: normal reads still work"; else bad "moved folder: a normal read was blocked"; fi

# Settings from 0.1.0 still pass --project with the old place: the hook must find the project anyway.
cp .claude/settings.local.json "$work/settings.bak"
sed 's#"PreToolUse"$#"PreToolUse", "--project", "/gone/old-place"#' "$work/settings.bak" >.claude/settings.local.json
grep -q '/gone/old-place' .claude/settings.local.json || bad "could not add an old --project to the settings"
out=$(echo "Use the Read tool on standin.config.json and print its second line exactly." |
	claude -p --model haiku --allowedTools Read 2>&1 || true)
if printf '%s' "$out" | grep -q '"version"'; then ok "old --project that is gone: normal reads still work"; else bad "old --project that is gone: a normal read was blocked"; fi
cp "$work/settings.bak" .claude/settings.local.json

echo "claude $(claude --version 2>/dev/null | head -1): $pass passed, $fail failed"
[ "$fail" -eq 0 ]
