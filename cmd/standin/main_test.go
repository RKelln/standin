package main

import (
	"os/exec"
	"strings"
	"testing"
)

func deps(t *testing.T, format, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "-f", format, pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// The tool must never make a network request. The surest check: the binary
// does not link any package that can open a connection.
func TestNoNetworkPackages(t *testing.T) {
	banned := map[string]bool{"net": true, "crypto/tls": true, "plugin": true}
	for _, pkg := range deps(t, "{{.ImportPath}}", ".") {
		// net/url and net/netip cannot open a connection, but they are not
		// needed here, and a blanket ban on net/* is easier to audit than a
		// list of exceptions. golang.org/x/text (character sets) is fine: it
		// is a table lookup. golang.org/x/net is a network stack.
		if banned[pkg] || strings.HasPrefix(pkg, "net/") || strings.HasPrefix(pkg, "golang.org/x/net") {
			t.Errorf("standin links %s", pkg)
		}
	}
}

// Only init may run another program (it runs `claude --version`). Every other
// package, the hook and the scrubber included, cannot.
func TestOnlyInitRunsPrograms(t *testing.T) {
	pkgs := deps(t, "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	for _, pkg := range pkgs {
		if pkg == "github.com/RKelln/standin/internal/initcmd" || pkg == "github.com/RKelln/standin/internal/cli" ||
			pkg == "github.com/RKelln/standin/cmd/standin" {
			continue
		}
		for _, d := range deps(t, "{{.ImportPath}}", pkg) {
			if d == "os/exec" {
				t.Errorf("%s links os/exec; only internal/initcmd may", pkg)
			}
		}
	}
	for _, pkg := range []string{".", "github.com/RKelln/standin/internal/cli"} {
		out, err := exec.Command("go", "list", "-f", "{{join .Imports \" \"}}", pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		for _, imp := range strings.Fields(string(out)) {
			if imp == "os/exec" {
				t.Errorf("%s imports os/exec directly; only internal/initcmd may", pkg)
			}
		}
	}
}
