package cli

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/RKelln/standin/internal/config"
	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/shape"
	"github.com/RKelln/standin/internal/store"
)

// plan is what scrub will do with one column.
type plan struct {
	action   string
	category string
	why      string // "config", "default", "free text", "not approved"
}

type scrubFile struct {
	f     rawFile
	d     *dataset.Dataset
	plans []plan
	out   []byte
}

// approvalState loads the approval and says whether keeps may be used.
func (c *ctx) approvalState(cfg *config.Config, privDir string) (approved bool, why string, err error) {
	a, err := store.LoadApproval(privDir)
	if err != nil {
		return false, "", err
	}
	if !hasKeeps(cfg) {
		return true, "", nil
	}
	if a == nil {
		return false, "The config has not been approved yet.", nil
	}
	if a.ConfigSHA256 != cfg.Hash {
		return false, "The config changed after it was approved, so the approval no longer counts.", nil
	}
	return true, "", nil
}

func hasKeeps(cfg *config.Config) bool {
	for _, f := range cfg.Files {
		for _, col := range f.Columns {
			if col.Action == config.Keep {
				return true
			}
		}
	}
	return false
}

func (c *ctx) loadConfig() (*config.Config, error) {
	cfg, exists, err := config.Load(c.project)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fail.New("no-config", "There is no %s in the project folder. Run `standin shape`, then have the AI write the config from the shape files.", config.FileName)
	}
	return cfg, nil
}

// refuseStandins stops if raw input already holds stand-in-shaped text, since
// restore would later swap it for a real value.
func refuseStandins(f rawFile, d *dataset.Dataset) error {
	const what = "already holds text shaped like a stand-in (like [NAME_1]). Restore could later swap it for a real value, so I stopped. Change that text in the raw file (for example remove the square brackets), then try again."
	for i, h := range d.Header() {
		if scan.HasStandin(h) {
			return fail.New("standin-in-input", "%s: the header of column %d %s", show(f.rel), i+1, what)
		}
	}
	for col, name := range d.Columns {
		label, _ := shape.Label(name, col)
		if d.Kind == dataset.JSON && scan.HasStandin(name) {
			return fail.New("standin-in-input", "%s: a name (key) in column %d %s", show(f.rel), col+1, what)
		}
		for i, v := range d.Values[col] {
			if scan.HasStandin(v) {
				if name == dataset.KeyColumn {
					return fail.New("standin-in-input", "%s: the key of %s %s", show(f.rel), d.Where(col, i), what)
				}
				return fail.New("standin-in-input", "%s: %s, column %s %s", show(f.rel), d.Where(col, i), label, what)
			}
		}
	}
	return nil
}

func runScrub(c *ctx) error {
	cfg, err := c.loadConfig()
	if err != nil {
		return err
	}
	files, err := c.rawFiles()
	if err != nil {
		return err
	}
	privDir, err := c.privateDir(false)
	if err != nil {
		return err
	}
	approved, why, err := c.approvalState(cfg, privDir)
	if err != nil {
		return err
	}
	mapPath := filepath.Join(privDir, store.MappingFile)
	m, _, err := mapping.Load(mapPath)
	if err != nil {
		return err
	}

	// Read and check every file before changing anything.
	var sfs []*scrubFile
	for _, f := range files {
		if dataLikeName(f.inRaw) {
			return fail.New("name-is-data", "One file name looks like private data (a name, email or number), and the scrubbed copy would keep that name. Rename the file in raw/ to something plain, like invoices.csv, then try again.")
		}
		d, err := c.load(f)
		if err != nil {
			return err
		}
		if err := refuseStandins(f, d); err != nil {
			return err
		}
		sf := &scrubFile{f: f, d: d}
		if err := c.planColumns(cfg, sf, approved); err != nil {
			return err
		}
		sfs = append(sfs, sf)
	}
	if !approved {
		c.warn("%s Every column marked keep is tokenized instead. Run `standin approve` to use the keeps.", why)
	}

	// Pass 1: tokenize columns (and data-like headers) in every file, so
	// every known value is in the mapping before any scan.
	for _, sf := range sfs {
		for _, h := range sf.d.Header() {
			headerStandin(m, h)
		}
		for col, p := range sf.plans {
			if p.action != config.Tokenize {
				continue
			}
			for _, v := range sf.d.Values[col] {
				if strings.TrimSpace(v) != "" {
					m.Get(p.category, v)
				}
			}
		}
	}
	// Pass 2: scan columns, then build each output.
	known := scan.NewKnown(m)
	for _, sf := range sfs {
		plans := sf.plans
		out, err := sf.d.Write(dataset.Rewriter{
			Cell: func(col int, v string) (string, bool) {
				if strings.TrimSpace(v) == "" {
					return v, false
				}
				switch plans[col].action {
				case config.Keep:
					return v, false
				case config.Drop:
					return "", true
				case config.Scan:
					nv, _ := scan.Replace(v, known, m)
					return nv, nv != v
				}
				return m.Get(plans[col].category, v), true
			},
			Name: func(name string) string { return headerStandin(m, name) },
		})
		if err != nil {
			return fail.New(fail.Code(err), "%s: %s", show(sf.f.rel), err.Error())
		}
		sf.out = out
	}

	// Write the mapping and every scrubbed file, or nothing.
	if _, err := c.privateDir(true); err != nil {
		return err
	}
	outs := []store.Out{{Path: mapPath, Data: m.Marshal(), Perm: 0o600}}
	for _, sf := range sfs {
		outs = append(outs, store.Out{Path: filepath.Join(c.project, "scrubbed", filepath.FromSlash(sf.f.inRaw)), Data: sf.out, Perm: 0o644})
	}
	if err := store.WriteAll(outs); err != nil {
		return err
	}

	anyScan := false
	for _, sf := range sfs {
		counts := map[string]int{}
		for _, p := range sf.plans {
			counts[p.action]++
		}
		if counts[config.Scan] > 0 {
			anyScan = true
		}
		rows := fmt.Sprint(sf.d.Rows)
		if sf.d.Rows < 5 {
			rows = shape.CountBand(sf.d.Rows)
		}
		c.msg("Wrote scrubbed/%s (%s rows): %d tokenized, %d kept, %d scanned, %d dropped.",
			sf.f.inRaw, rows, counts[config.Tokenize], counts[config.Keep], counts[config.Scan], counts[config.Drop])
		preview(c, sf)
	}
	c.msg("The mapping has %d stand-ins (%d new).", len(m.Entries), m.Added())
	if anyScan {
		c.msg("Scan columns: I swapped values I already know and patterns (emails, phone numbers, long numbers, postal codes). Names I do not know stay in. Read those columns before you share them.")
	}
	c.msg("Stand-ins still show which rows share a value. The AI should read only scrubbed/, never raw/ or restored/.")
	return nil
}

// headerStandin swaps a header or JSON key that looks like data for a stand-in.
func headerStandin(m *mapping.Mapping, name string) string {
	if kind, ok := scan.LooksLikeData(name); ok {
		return m.Get(scan.CategoryForKind(kind), name)
	}
	return name
}

// planColumns decides what to do with each column of a file.
func (c *ctx) planColumns(cfg *config.Config, sf *scrubFile, approved bool) error {
	d, f := sf.d, sf.f
	rule := cfg.RuleFor(f.rel)
	if rule == nil {
		c.warn("No rule in the config matches %s. Every column is tokenized (free text is dropped).", show(f.rel))
	}
	infos := shape.Columns(d)
	matched := map[string]bool{}
	for i, name := range d.Columns {
		label := infos[i].Label
		var p plan
		col, found := config.Column{}, false
		if rule != nil {
			var err error
			col, found, err = rule.ColumnFor(name, i)
			if err != nil {
				return err
			}
			if found {
				if _, ok := rule.Columns[name]; ok {
					matched[name] = true
				} else {
					matched[fmt.Sprintf("[COLUMN_%d]", i+1)] = true
				}
			}
		}
		switch {
		case found && col.Action == config.Keep && !approved:
			cat := "VALUE"
			if name == dataset.KeyColumn {
				cat = "KEY"
			}
			p = plan{config.Tokenize, cat, "not approved"}
		case found:
			p = plan{col.Action, col.Category, "config"}
		case name == dataset.KeyColumn:
			p = plan{config.Tokenize, "KEY", "default"}
		case infos[i].LooksFreeText():
			p = plan{config.Drop, "", "free text"}
			c.warn("%s: column %s reads like free text and is not in the config, so I dropped it. To keep some of it, add it to the config with \"scan\".", show(f.rel), label)
		default:
			p = plan{config.Tokenize, "VALUE", "default"}
		}
		sf.plans = append(sf.plans, p)
	}
	if rule != nil {
		for _, name := range rule.ColumnNames() {
			if !matched[name] {
				c.warn("The config names column %q for %s, but that file has no such column.", name, show(f.rel))
			}
		}
	}
	c.noteEncoding(f, d)
	return nil
}

// preview prints the first three rows of a scrubbed table, so the person can eyeball it.
func preview(c *ctx, sf *scrubFile) {
	if sf.d.Kind == dataset.JSON {
		return
	}
	r := csv.NewReader(bytes.NewReader(sf.out))
	r.Comma = ','
	if sf.d.Kind == dataset.TSV {
		r.Comma = '\t'
	}
	recs, err := r.ReadAll()
	if err != nil || len(recs) < 2 {
		return
	}
	n := len(recs)
	if n > 4 {
		n = 4
	}
	c.msg("  First rows of the scrubbed copy (open raw/ yourself to compare):")
	for _, rec := range recs[:n] {
		c.msg("    %s", strings.Join(rec, " | "))
	}
}
