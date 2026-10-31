package cli

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/config"
	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/shape"
	"github.com/RKelln/standin/internal/store"
	"github.com/RKelln/standin/internal/words"
)

// plan is what scrub will do with one column.
type plan struct {
	action   string
	category string
	why      string // "config", "default", "free text"
}

type scrubFile struct {
	f     rawFile
	d     *dataset.Dataset
	rule  *config.File
	plans []plan
	text  string // text files: config.Words or config.Drop, for the free text
	out   []byte
	// checked are the scanned cells and word-checked lines of the output,
	// looked at once more for known values before anything is written.
	checked []checkedPart
}

// checkedPart is one scanned cell or word-checked line of an output.
type checkedPart struct {
	where string // "line 4" or "column Notes"
	text  string
}

// wordPass reports whether any part of the file gets the word check.
func (sf *scrubFile) wordPass() bool {
	if sf.d.Kind != dataset.Text {
		return false
	}
	if sf.text == config.Words {
		return true
	}
	for i, p := range sf.plans {
		if p.action == config.Scan && sf.d.Source[i] == dataset.FromField {
			return true
		}
	}
	return false
}

// approvalState loads the approval and says whether keeps, scans and word
// checks may be used.
func (c *ctx) approvalState(cfg *config.Config, privDir string) (approved bool, why string, err error) {
	a, err := store.LoadApproval(privDir)
	if err != nil {
		return false, "", err
	}
	if !needsApproval(cfg) {
		return true, "", nil
	}
	if a == nil {
		return false, "The config has not been approved yet.", nil
	}
	if a.ConfigSHA256 != cfg.Hash {
		return false, "The config changed after it was approved, so the approval no longer counts.", nil
	}
	if usesWords(cfg) {
		_, raw, exists, err := words.LoadChoices(filepath.Join(privDir, store.WordsFile))
		if err != nil {
			return false, "", err
		}
		switch {
		case a.WordsSHA256 == "":
			return false, "The word choices for free text have not been approved yet.", nil
		case !exists:
			return false, "The word choices file (words.json, in the private folder) is missing, so the approval no longer counts.", nil
		case config.HashBytes(raw) != a.WordsSHA256:
			return false, "The word choices file (words.json) changed after it was approved, so the approval no longer counts.", nil
		}
	}
	return true, "", nil
}

// needsApproval: a keep shows real values, and a scan or a word check leaves
// in every word it does not flag, so each needs a person to say yes.
func needsApproval(cfg *config.Config) bool {
	for _, f := range cfg.Files {
		for _, cols := range []map[string]config.Column{f.Columns, f.Fields, f.Tables} {
			for _, col := range cols {
				if col.Action == config.Keep || col.Action == config.Scan {
					return true
				}
			}
		}
		if f.UsesWords() {
			return true
		}
	}
	return false
}

// usesWords: some rule asks for the word check on free text.
func usesWords(cfg *config.Config) bool {
	for i := range cfg.Files {
		if cfg.Files[i].UsesWords() {
			return true
		}
		for _, col := range cfg.Files[i].Fields {
			if col.Action == config.Scan {
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
		if d.Kind == dataset.Text && scan.HasStandin(name) {
			return fail.New("standin-in-input", "%s: a table header %s", show(f.rel), what)
		}
		for i, v := range d.Values[col] {
			if scan.HasStandin(v) {
				if name == dataset.KeyColumn {
					return fail.New("standin-in-input", "%s: the key of %s %s", show(f.rel), d.Where(col, i), what)
				}
				if d.Kind == dataset.Text {
					return fail.New("standin-in-input", "%s: %s %s", show(f.rel), d.Where(col, i), what)
				}
				return fail.New("standin-in-input", "%s: %s, column %s %s", show(f.rel), d.Where(col, i), label, what)
			}
		}
	}
	if d.Kind == dataset.Text {
		for i, l := range d.TextLines() {
			if scan.HasStandin(l) {
				return fail.New("standin-in-input", "%s: line %d %s", show(f.rel), i+1, what)
			}
		}
	}
	return nil
}

// run is one scrub (or a dry run of it for approve and review): every raw
// file read and planned, and the mapping filled with the tokenized values.
type run struct {
	m       *mapping.Mapping
	choices *words.Choices
	sfs     []*scrubFile
	known   *scan.Known
	finder  *words.Finder // nil when no file gets the word check
}

// prepare reads and plans every file, then runs pass 1: values of tokenized
// and dropped columns join the mapping, so every known value is in it before
// any scan or word check. It writes nothing.
func (c *ctx) prepare(cfg *config.Config, files []rawFile, m *mapping.Mapping, ch *words.Choices) (result *run, err error) {
	// A later column may teach us that an earlier file name is private.
	// Publish diagnostics only once every filename passed the full check.
	var messages bytes.Buffer
	stderr := c.env.Stderr
	c.env.Stderr = &messages
	defer func() {
		c.env.Stderr = stderr
		if err == nil {
			fmt.Fprint(stderr, messages.String())
		}
	}()
	r := &run{m: m, choices: ch}
	for _, f := range files {
		if dataLikeName(f.inRaw) || knownDataName(f.inRaw, m) {
			return nil, fail.New("name-is-data", "One file name looks like private data (a name, email or number), and the scrubbed copy would keep that name. Rename the file in raw/ to something plain, like invoices.csv, then try again.")
		}
		d, err := c.load(f)
		if err != nil {
			return nil, err
		}
		r.sfs = append(r.sfs, &scrubFile{f: f, d: d})
	}
	// Before naming a file in any rule/input refusal, check its name against
	// every value in the batch. Even a later file can reveal an earlier name.
	names := mapping.New()
	for _, sf := range r.sfs {
		for _, values := range sf.d.Values {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					names.Get("VALUE", value)
				}
			}
		}
	}
	for _, sf := range r.sfs {
		if knownDataName(sf.f.inRaw, names) {
			return nil, fail.New("name-is-data", "One file name repeats a value in the raw files. Rename the file to something plain, like invoices.csv, then try again.")
		}
		if err := refuseStandins(sf.f, sf.d); err != nil {
			return nil, err
		}
		if err := c.planColumns(cfg, sf); err != nil {
			return nil, err
		}
	}
	c.unusedTextNames(cfg, r.sfs)
	// Pass 1. Dropped values join the mapping too (they are not written
	// anywhere), so a name dropped from one column is still swapped in scan text.
	var parts []string
	for _, sf := range r.sfs {
		var infos []shape.ColumnInfo
		if sf.d.Kind == dataset.Text {
			infos = shape.Columns(sf.d)
		}
		for col, p := range sf.plans {
			if p.action != config.Tokenize && p.action != config.Drop {
				continue
			}
			cat := p.category
			if cat == "" {
				cat = "VALUE"
			}
			field := sf.d.Kind == dataset.Text && sf.d.Source[col] == dataset.FromField
			for _, v := range sf.d.Values[col] {
				if strings.TrimSpace(v) == "" {
					continue
				}
				m.Get(cat, v)
				if field {
					// "From: Ana Brown <ana@example.com>": the name and the email
					// are known on their own too, for the text below.
					if i := strings.Index(v, "<"); i > 0 && strings.TrimSpace(v[:i]) != "" {
						m.Get(cat, strings.TrimSpace(v[:i]))
					}
					for _, h := range scan.FindPatterns(v, nil) {
						m.Get(h.Category, v[h.Start:h.End])
					}
					if !infos[col].LooksFreeText() {
						// Name parts come from short values only ("From: Daisy
						// Okafor"), not from prose, where most capitalised words
						// just start a sentence.
						parts = append(parts, v)
					}
				}
			}
		}
	}
	for _, sf := range r.sfs {
		for _, h := range sf.d.Header() {
			headerStandin(m, h)
		}
		if sf.d.Kind == dataset.Text {
			for i, name := range sf.d.Columns {
				if sf.d.Source[i] != dataset.FromField {
					headerStandin(m, name)
				}
			}
		}
	}
	m.SetKept(func(key string) bool { return ch.Keep[words.Key(key)] })
	r.known = scan.NewKnown(m)
	for _, sf := range r.sfs {
		if !sf.wordPass() {
			continue
		}
		if r.finder == nil {
			r.finder = words.NewFinder(r.known)
			for _, v := range parts {
				r.finder.AddFieldParts(v)
			}
		}
		if sf.text == config.Words {
			r.finder.AddLines(sf.d.TextLines())
		}
		for col, p := range sf.plans {
			if p.action == config.Scan && sf.d.Source[col] == dataset.FromField {
				for _, v := range sf.d.Values[col] {
					r.finder.AddLines([]string{v})
				}
			}
		}
	}
	for _, sf := range r.sfs {
		if knownDataName(sf.f.inRaw, m) {
			return nil, fail.New("name-is-data", "One file name holds a private value Standin knows. Rename the file in raw/ to something plain, like invoices.csv, then try again.")
		}
	}
	return r, nil
}

// swapper says, for one file, which words become stand-ins: every candidate
// and every word on the swap list, except kept words and, where the rule
// keeps them, day and month names.
func (r *run) swapper(sf *scrubFile) func(key string) bool {
	keepCal := sf.rule.KeepsGroup(words.GroupCalendar)
	return func(key string) bool {
		if r.choices.Keep[key] {
			return false
		}
		if cd := r.finder.Get(key); cd != nil {
			return !keepCal || cd.Group() != words.GroupCalendar
		}
		return r.choices.Swap[key]
	}
}

// newWords are candidates the person has not decided on yet. Day and month
// names are a group, decided by the config, so they are never new.
func (r *run) newWords() []*words.Cand {
	if r.finder == nil {
		return nil
	}
	var out []*words.Cand
	for _, cd := range r.finder.Candidates() {
		if cd.Group() != words.GroupCalendar && !r.choices.Keep[cd.Key] && !r.choices.Swap[cd.Key] {
			out = append(out, cd)
		}
	}
	return out
}

// loadChoices reads words.json from the private folder.
func loadChoices(privDir string) (*words.Choices, error) {
	ch, _, _, err := words.LoadChoices(filepath.Join(privDir, store.WordsFile))
	return ch, err
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
	lease, err := store.Lock(privDir)
	if err != nil {
		return err
	}
	defer func() {
		if lease.Close() != nil {
			c.warn("I could not unlock the private folder after this run. Ask the person to check it before running Standin again.")
		}
	}()
	approved, why, err := c.approvalState(cfg, privDir)
	if err != nil {
		return err
	}
	if !approved {
		// Refuse rather than tokenize keep and scan columns as VALUE: those
		// whole-cell stand-ins would join the mapping, and a later approved
		// scrub would then swap whole cells in scan columns (decision 33).
		return fail.New("not-approved", "%s The config keeps or scans some columns, or checks the words in free text, so a person must approve it first. I wrote nothing. Run `standin approve` yourself, in your own terminal, then run `standin scrub` again.", why)
	}
	mapPath := filepath.Join(privDir, store.MappingFile)
	m, exists, err := mapping.Load(mapPath)
	if err != nil {
		return err
	}
	if !exists {
		entries, err := os.ReadDir(filepath.Join(c.project, "scrubbed"))
		if err != nil && !os.IsNotExist(err) {
			return fail.New("read", "I could not check the old scrubbed files safely.")
		}
		if len(entries) > 0 {
			return fail.New("mapping-missing", "Scrubbed files exist, but their mapping is missing. I stopped so old stand-ins are not given to different values. Ask the person to restore the matching private folder from a backup, or move the old scrubbed files away before starting a new mapping.")
		}
	}
	ch, err := loadChoices(privDir)
	if err != nil {
		return err
	}
	r, err := c.prepare(cfg, files, m, ch)
	if err != nil {
		return err
	}
	// Pass 2: scans and the word check, then build each output. Words the
	// check swaps join the mapping as it goes, so a scan in a file built
	// before them would miss them: if the mapping grew, build every output
	// again with the bigger known list (stand-ins stay the same).
	known := r.known
	swappedWords := map[string]bool{}
	for round := 0; round < 2; round++ {
		if round == 1 {
			m.SetKept(func(key string) bool { return ch.Keep[words.Key(key)] })
			known = scan.NewKnown(m)
		}
		size := len(m.Entries)
		for _, sf := range r.sfs {
			sf.checked = nil
			plans, d := sf.plans, sf.d
			infos := shape.Columns(d)
			keepCal := sf.rule.KeepsGroup(words.GroupCalendar)
			var swap func(string) bool
			if sf.wordPass() {
				inner := r.swapper(sf)
				swap = func(k string) bool {
					ok := inner(k)
					if ok {
						swappedWords[k] = true
					}
					return ok
				}
			}
			out, err := d.Write(dataset.Rewriter{
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
						var nv string
						if d.Kind == dataset.Text && d.Source[col] == dataset.FromField {
							nv = words.Replace(v, known, m, swap, keepCal)
						} else {
							nv, _ = scan.Replace(v, known, m)
						}
						what := "column " + infos[col].Label
						if d.Kind == dataset.Text {
							what = "field " + infos[col].Label
						}
						sf.checked = append(sf.checked, checkedPart{what, nv})
						return nv, nv != v
					}
					return m.Get(plans[col].category, v), true
				},
				Name: func(name string) string { return headerStandin(m, name) },
				Text: func(i int, s string) string {
					if sf.text != config.Words {
						return "" // dropped
					}
					out := words.Replace(s, known, m, swap, keepCal)
					sf.checked = append(sf.checked, checkedPart{fmt.Sprintf("line %d", i+1), out})
					return out
				},
			})
			if err != nil {
				return fail.New(fail.Code(err), "%s: %s", show(sf.f.rel), err.Error())
			}
			sf.out = out
		}
		if len(m.Entries) == size {
			break
		}
	}

	// Last check before anything is written: no value the mapping knows may
	// be left in a scanned cell or a word-checked line (decision 47). The
	// same finder as check, so kept words and common words are not looked
	// for. Kept columns are not checked: the person approved those values.
	chk := scan.NewKnown(m)
	for _, sf := range r.sfs {
		keepCal := sf.rule.KeepsGroup(words.GroupCalendar)
		for _, part := range sf.checked {
			if st, ok := leftOver(part.text, chk, keepCal); ok {
				return fail.New("left-over", "%s, %s: the scrubbed copy would still hold a real value Standin knows (its stand-in is %s). So I stopped, and wrote nothing. "+
					"In the config, tokenize or drop that part, then try again. Please also tell us with `standin report`: this is a gap in Standin.", show(sf.f.rel), part.where, st)
			}
		}
	}

	// Write the mapping and every scrubbed file, or nothing.
	if _, err := c.privateDir(true); err != nil {
		return err
	}
	outs := []store.Out{{Path: mapPath, Data: m.Marshal(), Perm: 0o600, Root: privDir}}
	for _, sf := range r.sfs {
		outs = append(outs, store.Out{Path: filepath.Join(c.project, "scrubbed", filepath.FromSlash(sf.f.inRaw)), Data: sf.out, Perm: 0o644, Root: filepath.Join(c.project, "scrubbed")})
	}
	if err := store.WriteAll(outs); err != nil {
		return err
	}

	anyScan := false
	for _, sf := range r.sfs {
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
		if sf.d.Kind == dataset.Text {
			text := "dropped"
			switch {
			case sf.d.FreeTextLines() == 0:
				text = "none"
			case sf.text == config.Words:
				text = "word check"
			}
			c.msg("Wrote scrubbed/%s (%s lines): fields and table columns: %d tokenized, %d kept, %d scanned, %d dropped; free text: %s.",
				sf.f.inRaw, rows, counts[config.Tokenize], counts[config.Keep], counts[config.Scan], counts[config.Drop], text)
			continue
		}
		c.msg("Wrote scrubbed/%s (%s rows): %d tokenized, %d kept, %d scanned, %d dropped.",
			sf.f.inRaw, rows, counts[config.Tokenize], counts[config.Keep], counts[config.Scan], counts[config.Drop])
		preview(c, sf)
	}
	c.msg("The mapping has %d stand-ins (%d new).", len(m.Entries), m.Added())
	if anyScan {
		c.msg("Scan columns: I swapped values I already know and patterns (emails, phone numbers, long numbers, postal codes). Names I do not know stay in. Read those columns before you share them.")
	}
	if r.finder != nil {
		c.msg("Free text, this run: %d different words became stand-ins like [NAME_1] (words you kept in approve are not counted), plus known values and patterns. Words I did not flag stay in, such as a name that is also a common word written in lower case. Read the text before you share it.", len(swappedWords))
		if n := len(r.newWords()); n > 0 {
			c.warn("%d new word(s) in the free text became stand-ins. To keep any of them, run `standin approve` yourself, in your own terminal.", n)
		}
	}
	c.msg("Stand-ins still show which rows share a value. The AI should read only scrubbed/, never raw/ or restored/.")
	return nil
}

// leftOver finds a known value in scrubbed text, outside the stand-ins,
// and gives its stand-in. A one-word NAME that is a day or month name is not
// looked for when the rule keeps the calendar group.
func leftOver(text string, known *scan.Known, keepCal bool) (string, bool) {
	for _, h := range known.Find(text) {
		v := text[h.Start:h.End]
		if scan.InsideStandin(text, h.Start, h.End) {
			continue
		}
		if keepCal && h.Category == words.Category && !strings.ContainsAny(v, " \t\u00a0") && words.IsCalendar(v) {
			continue
		}
		return h.Standin, true
	}
	return "", false
}

// headerStandin swaps a header or JSON key for a stand-in when it looks like
// data (the same check as the shape file), or when it equals a value
// already in the mapping.
func headerStandin(m *mapping.Mapping, name string) string {
	if kind, ok := scan.LooksLikeDataHeader(name); ok {
		return m.Get(scan.CategoryForKind(kind), name)
	}
	if utf8.RuneCountInString(mapping.Normalize(name)) >= scan.MinKnownLen {
		if e, ok := m.FindValue(name); ok && (scan.SkipKnown == nil || !scan.SkipKnown(e)) {
			return m.Get(e.Category, name)
		}
	}
	return name
}

// dataLikeHeader: the header was hidden because it looks like data (not
// because it is blank).
func dataLikeHeader(ci shape.ColumnInfo) bool { return ci.HeaderFlag != "" && ci.HeaderFlag != "empty" }

// planColumns decides what to do with each column of a file.
// The config must already be approved (runScrub refuses otherwise).
func (c *ctx) planColumns(cfg *config.Config, sf *scrubFile) error {
	d, f := sf.d, sf.f
	rule := cfg.RuleFor(f.rel)
	sf.rule = rule
	if rule == nil {
		c.warn("No rule in the config matches %s. Every column is tokenized (free text is dropped).", show(f.rel))
	}
	if d.Kind == dataset.Text {
		return c.planText(rule, sf)
	}
	if rule != nil && (len(rule.Fields) > 0 || len(rule.Tables) > 0 || rule.Text != nil) {
		c.warn("The config rule for %s has fields, tables or text. Those are only used for .md and .txt files.", show(f.rel))
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
				if _, ok := rule.Columns[name]; ok && dataLikeHeader(infos[i]) {
					return fail.New("config-bad", "%s: the config names column %d by its header, but that header looks like private data (a name, email or number). Call it %s in the config instead; it can only be tokenize or drop. Fix it, then try again.", config.FileName, i+1, infos[i].Label)
				}
				if _, ok := rule.Columns[name]; ok {
					matched[name] = true
				} else {
					matched[fmt.Sprintf("[COLUMN_%d]", i+1)] = true
				}
			}
		}
		switch {
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

// planText decides what to do with the fields, table columns and free text
// of a .md or .txt file. Like columns, anything the config does not name is
// tokenized, and free text it does not name is dropped.
func (c *ctx) planText(rule *config.File, sf *scrubFile) error {
	d, f := sf.d, sf.f
	infos := shape.Columns(d)
	if rule != nil && len(rule.Columns) > 0 {
		c.warn("The config rule for %s has columns. A text file uses fields, tables and text instead.", show(f.rel))
	}
	for i, name := range d.Columns {
		field := d.Source[i] == dataset.FromField
		var col config.Column
		found := false
		if rule != nil {
			if field {
				col, found = rule.Fields[name]
			} else {
				col, found = rule.Tables[name]
				if found && dataLikeHeader(infos[i]) {
					return fail.New("config-bad", "%s: the config names a table column of %s by its header, but that header looks like private data (a name, email or number). The config cannot name it: take it out of \"tables\". Standin tokenizes it. Fix it, then try again.", config.FileName, show(f.rel))
				}
			}
		}
		what := "field " + name
		if !field {
			what = "table column " + infos[i].Label
		}
		var p plan
		switch {
		case found:
			p = plan{col.Action, col.Category, "config"}
		case infos[i].LooksFreeText():
			p = plan{config.Drop, "", "free text"}
			c.warn("%s: %s reads like free text and is not in the config, so I dropped it. To keep some of it, add it to the config with \"scan\".", show(f.rel), what)
		default:
			p = plan{config.Tokenize, "VALUE", "default"}
		}
		sf.plans = append(sf.plans, p)
	}
	sf.text = config.Drop
	if rule != nil && rule.Text != nil {
		sf.text = rule.Text.Action
	} else if rule != nil && d.FreeTextLines() > 0 {
		c.warn("The config rule for %s has no \"text\" rule, so I dropped its free text. To keep it, add \"text\": {\"action\": \"words\"}.", show(f.rel))
	}
	c.noteEncoding(f, d)
	return nil
}

// unusedTextNames warns about field and table names in the config that no
// text file it matched has.
func (c *ctx) unusedTextNames(cfg *config.Config, sfs []*scrubFile) {
	for i := range cfg.Files {
		rule := &cfg.Files[i]
		if len(rule.Fields) == 0 && len(rule.Tables) == 0 {
			continue
		}
		seen, any := map[string]bool{}, false
		for _, sf := range sfs {
			if sf.rule != rule || sf.d.Kind != dataset.Text {
				continue
			}
			any = true
			for j, name := range sf.d.Columns {
				seen[sf.d.Source[j][:1]+"\x00"+name] = true
			}
		}
		if !any {
			continue
		}
		for _, name := range rule.FieldNames() {
			if !seen["f\x00"+name] {
				c.warn("The config names field %q for %s, but no file it matches has that field.", name, rule.Match)
			}
		}
		for _, name := range rule.TableNames() {
			if !seen["t\x00"+name] {
				c.warn("The config names table column %q for %s, but no file it matches has that column.", name, rule.Match)
			}
		}
	}
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
