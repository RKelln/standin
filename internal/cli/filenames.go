package cli

import (
	"path/filepath"
	"strings"

	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
)

// knownDataName checks each component independently: punctuation and underscores
// are filename separators, not proof that a known private value is safe to show.
func knownDataName(rel string, m *mapping.Mapping) bool {
	known := scan.NewKnown(m)
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		stem := strings.TrimSuffix(part, filepath.Ext(part))
		for _, s := range []string{part, stem, strings.ReplaceAll(stem, "_", " "), strings.ReplaceAll(stem, "-", " ")} {
			if _, ok := m.FindValue(s); ok {
				return true
			}
			if len(known.Find(s)) != 0 {
				return true
			}
		}
	}
	return false
}
