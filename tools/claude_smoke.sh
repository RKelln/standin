#!/bin/sh
# Live integration check on invented data; this makes paid model calls.
# Run from the repo root: sh tools/claude_smoke.sh
# Offline helper tests only: sh tools/claude_smoke.sh --self-test
# Needs go, claude >=2.1.295, script, and a logged-in Claude account.
set -eu

if [ "${1:-}" = "--self-test" ]; then
	go test ./tools/smokecheck
	sh -n tools/claude_smoke.sh
	exit
fi
[ "$#" -eq 0 ] || { echo "Usage: sh tools/claude_smoke.sh [--self-test]" >&2; exit 2; }

repo=$(pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
export STANDIN_HOME="$work/home"
export STANDIN_SMOKE_WORK="$work"
export STANDIN_SMOKE_BIN="$work/standin"
export STANDIN_SMOKE_CHECK="$work/smokecheck"
export STANDIN_SMOKE_AUDIT="$work/audit.jsonl"
export STANDIN_SMOKE_BROKEN=0
bin="$STANDIN_SMOKE_BIN"
check="$STANDIN_SMOKE_CHECK"
go build -o "$bin" ./cmd/standin
go build -o "$check" ./tools/smokecheck

# A missing/auth-failed/model-refused call cannot pass by merely omitting a
# name: the requested hook must run. The audit uses fixed subject labels only.
cat >"$work/hook-wrapper" <<'WRAPPER'
#!/bin/sh
set -eu
input=$(mktemp "$STANDIN_SMOKE_WORK/hook-input.XXXXXX")
output=$(mktemp "$STANDIN_SMOKE_WORK/hook-output.XXXXXX")
trap 'rm -f "$input" "$output"' EXIT HUP INT TERM
cat >"$input"
status=0
if [ "$STANDIN_SMOKE_BROKEN" = 1 ] && [ "${3:-}" = PreToolUse ]; then
	status=1
else
	"$STANDIN_SMOKE_BIN" "$@" <"$input" >"$output" || status=$?
fi
"$STANDIN_SMOKE_CHECK" audit "$input" "$output" "$status" "$STANDIN_SMOKE_AUDIT"
cat "$output"
exit "$status"
WRAPPER
chmod 700 "$work/hook-wrapper"

proj="$work/demo"
mkdir -p "$proj"
cd "$proj"
"$bin" init --bin "$work/hook-wrapper" >/dev/null
cp "$repo/testdata/invoices.csv" "$repo/testdata/bank-export.csv" raw/
cp "$repo/testdata/standin.config.json" .
"$bin" shape >/dev/null 2>&1
answers=$(printf 'yes\n%.0s' 1 2 3 4 5 6 7 8 9 10 11 12)
if script --version >/dev/null 2>&1; then
	printf '%s\n' "$answers" | script -qec "$bin approve" /dev/null >/dev/null 2>&1
else
	printf '%s\n' "$answers" | script -q /dev/null "$bin" approve >/dev/null 2>&1
fi
scrub_status=0
"$bin" scrub >/dev/null 2>&1 || scrub_status=$?
[ "$scrub_status" -le 1 ] || { echo "Fixture scrub failed; no live checks ran." >&2; exit 1; }
[ -s scrubbed/invoices.csv ] || { echo "Fixture scrub produced no output." >&2; exit 1; }

name=$(grep -v '^#' "$repo/testdata/known-values.txt" | grep -E '^[A-Z][a-z]+ [A-Z][a-z]+$' | head -1)
[ -n "$name" ] || { echo "No synthetic name was found." >&2; exit 1; }
pass=0
fail=0
ok() { pass=$((pass + 1)); echo "PASS $1"; }
bad() { fail=$((fail + 1)); echo "FAIL $1 (expected tool/hook evidence or successful CLI exit missing)"; }

# The raw Read deny rule can stop a read before our hook. Remove it and the
# agent instructions only in this disposable project to prove hook redirects.
if [ ! -e .claude/settings.json ] && ! grep -q -- '--project' .claude/settings.local.json &&
	grep -Fq 'Read(./raw/**)' .claude/settings.local.json; then
	ok "generated local settings and raw Read deny rule"
else
	bad "generated local settings and raw Read deny rule"
fi
mkdir "$work/aside"
mv AGENTS.md "$work/aside/AGENTS.md"
"$check" settings .claude/settings.local.json hook-read

call() {
	prompt=$1
	shift
	: >"$STANDIN_SMOKE_AUDIT"
	call_status=0
	printf '%s\n' "$prompt" | claude -p --model haiku "$@" >"$work/answer" 2>"$work/error" || call_status=$?
}
has_audit() { "$check" expect "$STANDIN_SMOKE_AUDIT" "$@" >/dev/null 2>&1; }
no_name() { ! grep -Fq "$name" "$work/answer" && ! grep -Fq "$name" "$work/error"; }

call "Use the Read tool on raw/invoices.csv and print the first 3 lines. You must make this tool call." --allowedTools Read
if [ "$call_status" -eq 0 ] && has_audit PreToolUse Read raw/invoices.csv 0 true && no_name; then
	ok "Read raw/ was redirected by the hook"
else
	bad "Read raw/ was redirected by the hook"
fi

call "Use Bash to run exactly this command, with no extra arguments: cat raw/invoices.csv | head -3" --allowedTools "Bash(cat:*)" "Bash(head:*)"
if [ "$call_status" -eq 0 ] && has_audit PreToolUse Bash raw-cat 2 false && no_name; then
	ok "Bash cat raw/ was blocked by the hook"
else
	bad "Bash cat raw/ was blocked by the hook"
fi

call "Draft a one-line payment reminder for $name."
if has_audit UserPromptSubmit "" other 2 false &&
	grep -Fq "Your message was not sent" "$work/error" "$work/answer"; then
	ok "prompt with a known real value was blocked"
else
	bad "prompt with a known real value was blocked"
fi

export STANDIN_SMOKE_BROKEN=1
call "Use the Read tool on standin.config.json and print its second line exactly. You must make this tool call." --allowedTools Read
if has_audit PreToolUse Read standin.config.json 1 false &&
	grep -Fq 'onFailure' "$work/error" "$work/answer"; then
	ok "a broken hook blocked through onFailure"
else
	bad "a broken hook blocked through onFailure"
fi
export STANDIN_SMOKE_BROKEN=0

cd "$work"
mv "$proj" "$work/moved"
cd "$work/moved"
call "Use the Read tool on raw/invoices.csv and print the first 3 lines. You must make this tool call." --allowedTools Read
if [ "$call_status" -eq 0 ] && has_audit PreToolUse Read raw/invoices.csv 0 true && no_name; then
	ok "moved folder: raw Read was redirected"
else
	bad "moved folder: raw Read was redirected"
fi
call "Use the Read tool on standin.config.json and print its second line exactly. You must make this tool call." --allowedTools Read
if [ "$call_status" -eq 0 ] && has_audit PreToolUse Read standin.config.json 0 false &&
	grep -Fq '"version"' "$work/answer"; then
	ok "moved folder: normal Read ran"
else
	bad "moved folder: normal Read ran"
fi

"$check" settings .claude/settings.local.json old-project
call "Use the Read tool on standin.config.json and print its second line exactly. You must make this tool call." --allowedTools Read
if [ "$call_status" -eq 0 ] && has_audit PreToolUse Read standin.config.json 0 false &&
	grep -Fq '"version"' "$work/answer"; then
	ok "missing old --project: normal Read ran"
else
	bad "missing old --project: normal Read ran"
fi

echo "claude $(claude --version): $pass passed, $fail failed"
[ "$fail" -eq 0 ]
