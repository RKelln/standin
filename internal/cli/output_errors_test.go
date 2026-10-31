package cli

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

type brokenOutput struct {
	trigger string
	short   bool
}

func (w brokenOutput) Write(p []byte) (int, error) {
	if w.trigger != "" && !strings.Contains(string(p), w.trigger) {
		return len(p), nil
	}
	if w.short {
		return 0, nil
	}
	return 0, errors.New("Zelda Quorn private path")
}

func outputEnv(p *tp, input io.Reader, stdout, stderr io.Writer) Env {
	return Env{Stdin: input, Stdout: stdout, Stderr: stderr,
		StdinIsTerminal: func() bool { return true }, StdoutIsTerminal: func() bool { return true },
		StderrIsTerminal: func() bool { return true },
		Getenv: func(k string) string {
			if k == "STANDIN_HOME" {
				return p.home
			}
			return ""
		}}
}

func TestConsoleFailuresDoNotReturnSuccessOrEchoCause(t *testing.T) {
	for _, command := range []string{"help", "version", "licences", "review", "report"} {
		for _, short := range []bool{false, true} {
			t.Run(command, func(t *testing.T) {
				p := newTP(t)
				p.write("standin.config.json", `{"version":1,"files":[]}`)
				var stderr bytes.Buffer
				code := Run([]string{command, "--project", p.dir}, outputEnv(p, strings.NewReader(""), brokenOutput{short: short}, &stderr))
				if code != ExitRefused || strings.Contains(stderr.String(), "Zelda Quorn") {
					t.Fatalf("bad output status or private cause: %d, %s", code, stderr.String())
				}
			})
		}
	}
}

type countReads struct{ reads int }

func (r *countReads) Read(p []byte) (int, error) {
	r.reads++
	return copy(p, "yes\nyes\n"), io.EOF
}

func TestApproveDoesNotReadOrSaveAfterPromptFailure(t *testing.T) {
	for _, destination := range []string{"stdout", "stderr"} {
		t.Run(destination, func(t *testing.T) {
			p := newTP(t)
			p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.csv","columns":{"Amount":{"action":"keep"}}}]}`)
			input := &countReads{}
			var good bytes.Buffer
			var stdout, stderr io.Writer = &good, &good
			if destination == "stdout" {
				stdout = brokenOutput{trigger: "Type yes:"}
			} else {
				stderr = brokenOutput{}
			}
			code := Run([]string{"approve", "--project", p.dir}, outputEnv(p, input, stdout, stderr))
			if code != ExitRefused || input.reads != 0 {
				t.Fatalf("unseen prompt consumed an answer: status %d, reads %d", code, input.reads)
			}
			for _, name := range []string{"approval.json", "words.json"} {
				matches, err := filepath.Glob(filepath.Join(p.home, "projects", "*", name))
				if err != nil || len(matches) != 0 {
					t.Fatalf("saved approval after output failure: %v, %v", matches, err)
				}
			}
		})
	}
}

func TestApproveWordPageFailureSavesNoChoices(t *testing.T) {
	p := newTP(t)
	p.write("raw/notes.md", "Hi Oksana.\n")
	p.write("standin.config.json", `{"version":1,"files":[{"match":"raw/*.md","text":{"action":"words","keep_groups":[]}}]}`)
	var stderr bytes.Buffer
	code := Run([]string{"approve", "--project", p.dir}, outputEnv(p, strings.NewReader("yes\nnone\nyes\n"), brokenOutput{trigger: "Keep:"}, &stderr))
	if code != ExitRefused {
		t.Fatalf("word page output failure was accepted: %d %s", code, stderr.String())
	}
	for _, name := range []string{"approval.json", "words.json"} {
		matches, err := filepath.Glob(filepath.Join(p.home, "projects", "*", name))
		if err != nil || len(matches) != 0 {
			t.Fatalf("saved choices after output failure: %v, %v", matches, err)
		}
	}
}

func TestRestoreStdoutFailureReturnsError(t *testing.T) {
	p := newTP(t)
	p.write("raw/notes.csv", "Customer\nZelda Quorn\n")
	p.write("standin.config.json", `{"version":1,"files":[]}`)
	if r := p.run("", false, "scrub"); r.code > 1 {
		t.Fatalf("scrub: %+v", r)
	}
	var stderr bytes.Buffer
	code := Run([]string{"restore", "-", "--project", p.dir}, outputEnv(p, strings.NewReader("[VALUE_1]"), brokenOutput{}, &stderr))
	if code != ExitRefused || strings.Contains(stderr.String(), "Zelda Quorn") {
		t.Fatalf("restore lost output error or echoed cause: %d, %s", code, stderr.String())
	}
}
