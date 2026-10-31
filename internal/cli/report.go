package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/RKelln/standin/internal/config"
	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/shape"
	"github.com/RKelln/standin/internal/store"
)

// runReport prints facts that help fix a problem, and nothing from the
// person's files: no file names, column names or values.
func runReport(c *ctx) error {
	w := c.env.Stdout
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }
	commit := c.env.Commit
	if commit == "" {
		commit = "unknown"
	}
	p("## Standin issue report")
	p("")
	p("This report has no file names, column names or values from your data. Read it before you send it.")
	p("")
	p("- Standin: %s (commit %s)", c.version(), commit)
	p("- System: %s/%s, built with %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	cfg, exists, err := config.Load(c.project)
	switch {
	case !exists && err == nil:
		p("- Config: none")
	case err != nil:
		p("- Config: present, refused (%s)", fail.Code(err))
	default:
		counts, texts := map[string]int{}, map[string]int{}
		for _, f := range cfg.Files {
			for _, cols := range []map[string]config.Column{f.Columns, f.Fields, f.Tables} {
				for _, col := range cols {
					counts[col.Action]++
				}
			}
			if f.Text != nil {
				texts[f.Text.Action]++
			}
		}
		p("- Config: %d file rule(s); columns, fields and table columns: %d keep, %d tokenize, %d scan, %d drop; free text: %d words, %d drop",
			len(cfg.Files), counts[config.Keep], counts[config.Tokenize], counts[config.Scan], counts[config.Drop], texts[config.Words], texts[config.Drop])
	}

	privDir, perr := store.PrivateDir(c.project, c.private, c.env.Getenv("STANDIN_HOME"))
	if perr == nil && cfg != nil {
		approved, _, aerr := c.approvalState(cfg, privDir)
		switch {
		case aerr != nil:
			p("- Approval: %s", fail.Code(aerr))
		case approved:
			p("- Approval: matches the config (or none needed)")
		default:
			p("- Approval: missing or out of date")
		}
	}

	// Raw files: kinds and shapes only.
	type kindCount struct {
		ext string
		n   int
	}
	exts := map[string]int{}
	var lines []string
	root := filepath.Join(c.project, "raw")
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil //nolint:nilerr // Reports are best effort: keep describing other files after a walk error.
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == "" {
			ext = "(none)"
		} else if !plainExt[ext] {
			ext = "(other)" // an unknown extension could hold private text
		}
		exts[ext]++
		if _, err := dataset.KindFor(path); err != nil {
			return nil //nolint:nilerr // Unsupported kinds are counted above; only readable kinds have a shape.
		}
		ds, err := dataset.Load(path)
		if err != nil {
			lines = append(lines, fmt.Sprintf("  - a %s file: refused (%s)", ext, fail.Code(err)))
			return nil
		}
		hidden := 0
		for _, ci := range shape.Columns(ds) {
			if ci.HeaderFlag != "" {
				hidden++
			}
		}
		js := ""
		if ds.JSONShape != "" {
			js = ", " + ds.JSONShape
		}
		lines = append(lines, fmt.Sprintf("  - a %s file%s: %s, %d columns (%d look like data), rows %s",
			ext, js, ds.Encoding, len(ds.Columns), hidden, shape.CountBand(ds.Rows)))
		return nil
	})
	kc := make([]kindCount, 0, len(exts))
	for e, n := range exts {
		kc = append(kc, kindCount{e, n})
	}
	sort.Slice(kc, func(i, j int) bool { return kc[i].ext < kc[j].ext })
	if len(kc) == 0 {
		p("- raw/: no files")
	} else {
		parts := make([]string, 0, len(kc))
		for _, k := range kc {
			parts = append(parts, fmt.Sprintf("%s: %d", k.ext, k.n))
		}
		p("- raw/ files by kind: %s", strings.Join(parts, ", "))
		sort.Strings(lines)
		for _, l := range lines {
			p("%s", l)
		}
	}

	if perr == nil {
		m, exists, err := mapping.Load(filepath.Join(privDir, store.MappingFile))
		switch {
		case err != nil:
			p("- Mapping: %s", fail.Code(err))
		case !exists:
			p("- Mapping: none yet")
		default:
			counts := m.CountByCategory()
			parts := make([]string, 0, len(counts))
			for _, cc := range counts {
				parts = append(parts, cc.Category+" "+shape.CountBand(cc.Count))
			}
			p("- Mapping: stand-ins by category: %s", strings.Join(parts, ", "))
		}
		if st, err := os.Stat(privDir); err == nil && runtime.GOOS != "windows" {
			p("- Private folder permissions: %o", st.Mode().Perm())
		}
		es := store.LoadErrors(privDir)
		if len(es) == 0 {
			p("- Last errors: none recorded")
		} else {
			p("- Last errors (newest last):")
			for _, e := range es {
				p("  - %s %s: %s", e.Time, e.Command, e.Code)
			}
		}
	} else {
		p("- Private folder: %s", fail.Code(perr))
	}
	return nil
}

// File extensions the report may name. Any other is shown as "(other)".
var plainExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(".csv .tsv .json .txt .md .pdf .xlsx .xls .xlsm .ods .numbers .docx .doc .odt .rtf .pages " +
		".png .jpg .jpeg .gif .heic .webp .tif .tiff .bmp .ofx .qfx .qbo .iif .xml .html .htm .zip .eml .msg .mbox") {
		plainExt[e] = true
	}
}
