// smokecheck is a development helper for claude_smoke.sh, not part of standin.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type audit struct {
	Event      string `json:"event"`
	Tool       string `json:"tool"`
	Subject    string `json:"subject"`
	Status     int    `json:"status"`
	Redirected bool   `json:"redirected"`
}

func subject(tool string, input map[string]any) string {
	if tool == "Read" {
		p, _ := input["file_path"].(string)
		p = filepath.ToSlash(filepath.Clean(p))
		for _, suffix := range []string{"raw/invoices.csv", "standin.config.json"} {
			if p == suffix || strings.HasSuffix(p, "/"+suffix) {
				return suffix
			}
		}
	}
	if tool == "Bash" && input["command"] == "cat raw/invoices.csv | head -3" {
		return "raw-cat"
	}
	return "other"
}

func record(input, output []byte, status int) (audit, error) {
	var in struct {
		Event string         `json:"hook_event_name"`
		Tool  string         `json:"tool_name"`
		Input map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Event == "" {
		return audit{}, fmt.Errorf("hook input is missing")
	}
	a := audit{Event: in.Event, Tool: in.Tool, Subject: subject(in.Tool, in.Input), Status: status}
	var out struct {
		Hook struct {
			Updated struct {
				Path string `json:"file_path"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if json.Unmarshal(output, &out) == nil {
		p := filepath.ToSlash(out.Hook.Updated.Path)
		a.Redirected = strings.HasSuffix(p, "/scrubbed/invoices.csv")
	}
	return a, nil
}

func contains(data []byte, want audit) bool {
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var got audit
		if json.Unmarshal(line, &got) != nil {
			return false
		}
		if got == want {
			return true
		}
	}
	return false
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("say audit, expect, or settings")
	}
	switch args[0] {
	case "audit":
		if len(args) != 5 {
			return fmt.Errorf("audit needs input, output, exit status and log")
		}
		input, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		output, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		status, err := strconv.Atoi(args[3])
		if err != nil {
			return err
		}
		a, err := record(input, output, status)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(a)
		f, err := os.OpenFile(args[4], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = f.Write(append(b, '\n'))
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		return err
	case "expect":
		if len(args) != 7 {
			return fmt.Errorf("expect needs log, event, tool, subject, status and redirected")
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("no hook audit was recorded")
		}
		status, err := strconv.Atoi(args[5])
		if err != nil {
			return err
		}
		redirect, err := strconv.ParseBool(args[6])
		if err != nil {
			return err
		}
		if !contains(b, audit{args[2], args[3], args[4], status, redirect}) {
			return fmt.Errorf("the expected hook event, tool, path and exit status were not recorded")
		}
		return nil
	case "settings":
		if len(args) != 3 {
			return fmt.Errorf("settings needs a file and mode")
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		var settings map[string]any
		if err := json.Unmarshal(b, &settings); err != nil {
			return err
		}
		switch args[2] {
		case "hook-read":
			permissions := settings["permissions"].(map[string]any)
			var deny []any
			for _, rule := range permissions["deny"].([]any) {
				if rule != "Read(./raw/**)" {
					deny = append(deny, rule)
				}
			}
			permissions["deny"] = deny
		case "old-project":
			for _, group := range settings["hooks"].(map[string]any)["PreToolUse"].([]any) {
				for _, h := range group.(map[string]any)["hooks"].([]any) {
					hook := h.(map[string]any)
					hook["args"] = append(hook["args"].([]any), "--project", "/gone/old-place")
				}
			}
		default:
			return fmt.Errorf("unknown settings mode")
		}
		b, err = json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(args[1], append(b, '\n'), 0o600)
	}
	return fmt.Errorf("unknown command")
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "smoke check:", err)
		os.Exit(1)
	}
}
