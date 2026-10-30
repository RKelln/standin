package hook

import (
	"path/filepath"
	"strings"

	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
	corestore "github.com/RKelln/standin/internal/store"
)

// Finding is one thing a scan found. It never holds the real value.
type Finding struct {
	// Name is the stand-in for a known value, like "[PERSON_1]". Empty for a pattern hit.
	Name string
	// Kind is "known value" or a pattern kind such as "email".
	Kind string
}

// Scanner looks for real values in text.
type Scanner interface {
	Scan(text string) []Finding
}

// CoreFuncs are the core CLI functions this package uses (internal/store,
// internal/scan, internal/mapping). Tests may replace them.
type CoreFuncs struct {
	// StoreDir returns the private store folder for a project.
	StoreDir func(project, private string) (string, error)
	// NewScanner builds a scanner from the mapping in the store. A missing
	// mapping is not an error (patterns only). A mapping it cannot read is.
	NewScanner func(project, store string) (Scanner, error)
}

// Core holds the functions in use. Tests and main may change it.
var Core = CoreFuncs{StoreDir: StoreDir, NewScanner: CoreScanner}

// coreScanner uses the same mapping reader and scan code as `standin check`.
type coreScanner struct{ known *scan.Known }

// CoreScanner loads <store>/mapping.json with the core reader. A missing
// mapping means patterns only. A damaged one is an error (fail closed).
func CoreScanner(project, store string) (Scanner, error) {
	m, _, err := mapping.Load(filepath.Join(store, corestore.MappingFile))
	if err != nil {
		return nil, err
	}
	return coreScanner{known: scan.NewKnown(m)}, nil
}

func (c coreScanner) Scan(text string) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, h := range scan.FindAll(text, c.known) {
		key := h.Standin
		if key == "" {
			key = "kind:" + h.Kind
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Finding{Name: h.Standin, Kind: h.Kind})
	}
	return out
}

// describe lists findings by stand-in name and kind only.
func describe(fs []Finding) string {
	var names, kinds []string
	seenK := map[string]bool{}
	for _, f := range fs {
		if f.Name != "" {
			names = append(names, f.Name)
		} else if !seenK[f.Kind] {
			seenK[f.Kind] = true
			kinds = append(kinds, f.Kind)
		}
	}
	var parts []string
	if len(names) > 0 {
		parts = append(parts, "real values of "+strings.Join(names, ", "))
	}
	if len(kinds) > 0 {
		parts = append(parts, "something that looks like: "+strings.Join(kinds, ", "))
	}
	return strings.Join(parts, "; ")
}

func knownOnly(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Name != "" {
			out = append(out, f)
		}
	}
	return out
}
