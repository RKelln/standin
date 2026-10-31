package initcmd

import (
	"errors"
	"github.com/RKelln/standin/internal/store"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInitPreflightsEveryDestination(t *testing.T) {
	for _, blocked := range []string{"shape", "scrubbed", "restored", "standin-harness", ".claude/settings.local.json.bak"} {
		t.Run(blocked, func(t *testing.T) {
			fakeClaude(t, "2.1.300", true, nil)
			p := newProject(t)
			path := filepath.Join(p, filepath.FromSlash(blocked))
			if strings.HasSuffix(blocked, ".bak") {
				os.MkdirAll(path, 0755)
				os.WriteFile(filepath.Join(p, ".claude", "settings.local.json"), []byte(`{"model":"invented-model"}`), 0600)
			} else {
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte("blocker"), 0644)
			}
			before := snapshot(t, p)
			r := runInit(t, "--project", p, "--bin", "/x/standin", "--harness", "claude,codex")
			if r.code != 2 || !strings.Contains(r.err, "Nothing was changed") {
				t.Fatalf("%+v", r)
			}
			if after := snapshot(t, p); !reflect.DeepEqual(before, after) {
				t.Fatalf("project changed after refusal: before %v, after %v", before, after)
			}
		})
	}
}

func TestInitRollsBackNewFoldersAfterPublishFailure(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte("# Existing instructions\n"), 0600)
	before := snapshot(t, p)
	old := writeSetup
	writeSetup = func([]store.Out) error { return errors.New("invented write failure") }
	t.Cleanup(func() { writeSetup = old })
	r := runInit(t, "--project", p, "--bin", "/x/standin")
	if r.code != 2 || strings.Contains(r.err, "Nothing was changed") || !strings.Contains(r.err, "Setup did not finish") {
		t.Fatalf("%+v", r)
	}
	if after := snapshot(t, p); !reflect.DeepEqual(before, after) {
		t.Fatalf("new folders remained after failed setup: %v", after)
	}
}

func TestInitRefusesLinkedDestinations(t *testing.T) {
	fakeClaude(t, "2.1.300", true, nil)
	p := newProject(t)
	target := filepath.Join(t.TempDir(), "target")
	os.Mkdir(target, 0755)
	if err := os.Symlink(target, filepath.Join(p, "scrubbed")); err != nil {
		t.Skip(err)
	}
	r := runInit(t, "--project", p, "--bin", "/x/standin")
	if r.code != 2 || !strings.Contains(r.err, "uses a link") {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(p, "raw")); !os.IsNotExist(err) {
		t.Fatal("setup made raw before refusing the link")
	}
}

func TestInitRefusesAmbiguousSettingsJSON(t *testing.T) {
	for _, body := range []string{`{"model":"invented-a","model":"invented-b"}`, `{} {}`, `{"model":"\ud800"}`} {
		t.Run(body, func(t *testing.T) {
			fakeClaude(t, "2.1.300", true, nil)
			p := newProject(t)
			os.Mkdir(filepath.Join(p, ".claude"), 0755)
			os.WriteFile(filepath.Join(p, ".claude", "settings.local.json"), []byte(body), 0600)
			before := snapshot(t, p)
			r := runInit(t, "--project", p, "--bin", "/x/standin")
			if r.code != 2 || !reflect.DeepEqual(before, snapshot(t, p)) {
				t.Fatalf("changed ambiguous settings: %+v", r)
			}
		})
	}
}
