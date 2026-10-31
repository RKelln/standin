// Package hook implements `standin hook <harness> <event>`: the guard that an
// agent harness runs before and after each tool call and on each prompt.
//
// It fails closed: bad input, an unknown event or any doubt gives exit 2 with a
// short reason on stderr, which Claude Code treats as "block". It never prints a
// real value; findings are named by stand-in (like [PERSON_1]) or kind (like "email").
package hook

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Exit codes (docs/PLAN.md).
const (
	exitOK    = 0
	exitBlock = 2
)

const maxInput = 64 << 20 // 64 MiB of hook JSON is more than any real call

// Run is the entry point for `standin hook ...`. args excludes "hook" itself:
// for example ["claude", "PreToolUse"]. An old --project flag is still read.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("standin hook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	project := fs.String("project", "", "project folder")
	private := fs.String("private", "", "private store folder")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return block(stderr, "standin hook: "+err.Error()+". Use: standin hook claude EVENT")
	}
	if len(pos) != 2 {
		return block(stderr, "standin hook: say the harness and the event, like: standin hook claude PreToolUse")
	}
	harness, event := pos[0], pos[1]
	if harness != "claude" {
		return block(stderr, fmt.Sprintf("standin hook: %q is not a harness standin knows. Only claude works for now.", harness))
	}
	switch event {
	case "PreToolUse", "PostToolUse", "UserPromptSubmit":
	default:
		return block(stderr, fmt.Sprintf("standin hook: %q is not an event standin handles, so it blocks to be safe.", event))
	}

	raw, err := io.ReadAll(io.LimitReader(stdin, maxInput+1))
	if err != nil || len(raw) > maxInput {
		return block(stderr, "standin hook: could not read the hook input, so it blocks to be safe.")
	}
	var in Input
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&in); err != nil {
		return block(stderr, "standin hook: the hook input is not valid JSON, so it blocks to be safe.")
	}
	if dec.More() {
		return block(stderr, "standin hook: the hook input has extra data after the JSON, so it blocks to be safe.")
	}
	if in.HookEventName != event {
		return block(stderr, fmt.Sprintf("standin hook: expected a %s event but got %q, so it blocks to be safe.", event, in.HookEventName))
	}

	proj, err := FindProject(*project, in.Cwd)
	if err != nil {
		return block(stderr, "standin hook: "+err.Error()+", so it blocks to be safe. If the folder was moved, run standin init again in this folder.")
	}
	store, err := Core.StoreDir(proj, *private)
	if err != nil {
		return block(stderr, "standin hook: cannot work out the private store: "+err.Error())
	}
	c := &claude{in: in, g: Guard{Project: proj, Store: store}, stdout: stdout, stderr: stderr}
	switch event {
	case "PreToolUse":
		return c.preToolUse()
	case "PostToolUse":
		return c.postToolUse()
	default:
		return c.userPrompt()
	}
}

// parseInterleaved lets flags come before, between or after the positional words.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func block(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, strings.TrimSpace(msg))
	return exitBlock
}

// Input is the part of Claude Code's hook JSON that standin reads.
type Input struct {
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Prompt        *string         `json:"prompt"`
}
