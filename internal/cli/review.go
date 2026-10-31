package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/RKelln/standin/internal/config"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
	"github.com/RKelln/standin/internal/output"
	"github.com/RKelln/standin/internal/shape"
	"github.com/RKelln/standin/internal/store"
	"github.com/RKelln/standin/internal/words"
)

type reviewRow struct {
	match  string
	column string // as shown: "Amount", "field: Date", "table: Price", "(free text)"
	name   string // as asked: "Amount", "field Date", "table column Price", "the free text"
	col    config.Column
	groups []string // free text: kept word groups
}

// rowsKeepsFirst lists every rule: keeps, then scans, word checks, tokenizes and drops.
func rowsKeepsFirst(cfg *config.Config) []reviewRow {
	order := []string{config.Keep, config.Scan, config.Words, config.Tokenize, config.Drop}
	var rows []reviewRow
	for _, a := range order {
		for i := range cfg.Files {
			f := &cfg.Files[i]
			for _, name := range f.ColumnNames() {
				if col := f.Columns[name]; col.Action == a {
					rows = append(rows, reviewRow{f.Match, name, name, col, nil})
				}
			}
			for _, name := range f.FieldNames() {
				if col := f.Fields[name]; col.Action == a {
					rows = append(rows, reviewRow{f.Match, "field: " + name, "field " + name, col, nil})
				}
			}
			for _, name := range f.TableNames() {
				if col := f.Tables[name]; col.Action == a {
					rows = append(rows, reviewRow{f.Match, "table: " + name, "table column " + name, col, nil})
				}
			}
			if t := f.Text; t != nil && t.Action == a {
				rows = append(rows, reviewRow{f.Match, "(free text)", "the free text", config.Column{Action: t.Action, Reason: t.Reason}, t.Groups()})
			}
		}
	}
	return rows
}

func printReview(w io.Writer, cfg *config.Config) error {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ACTION\tFILES\tCOLUMN\tCATEGORY\tREASON")
	for _, r := range rowsKeepsFirst(cfg) {
		action := r.col.Action
		switch action {
		case config.Keep:
			action = "KEEP (real values)"
		case config.Words:
			action = "words (check)"
		}
		cat := r.col.Category
		if len(r.groups) > 0 {
			cat = "keeps " + strings.Join(r.groups, ", ")
		}
		if cat == "" {
			cat = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", action, r.match, r.column, cat, oneLine(r.col.Reason))
	}
	if err := tw.Flush(); err != nil {
		return output.Failure()
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "keep      the AI sees the real values. Only keep what is not private.")
	_, _ = fmt.Fprintln(w, "scan      free text: known values and patterns are swapped; names I do not know stay in.")
	_, _ = fmt.Fprintln(w, "          (In a .md or .txt file, a scan field gets the word check too.)")
	_, _ = fmt.Fprintln(w, "words     free text in a .md or .txt file: names and unusual words become stand-ins")
	_, _ = fmt.Fprintln(w, "          like [NAME_1], except words you keep in approve. Words I do not flag stay in.")
	_, _ = fmt.Fprintln(w, "tokenize  each value becomes a stand-in like [PERSON_1].")
	_, _ = fmt.Fprintln(w, "drop      the value is blanked.")
	_, _ = fmt.Fprintln(w, "Any column, field or table column not listed is tokenized. Free text not listed is dropped.")
	return nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// dryRun reads the raw files and runs scrub's first steps in memory, with
// warnings off, to count or ask about the words in free text. It writes
// nothing and changes no file.
func (c *ctx) dryRun(cfg *config.Config, privDir string) (*run, error) {
	files, err := c.rawFiles()
	if err != nil {
		return nil, err
	}
	m, _, err := mapping.Load(filepath.Join(privDir, store.MappingFile))
	if err != nil {
		return nil, err
	}
	ch, err := loadChoices(privDir)
	if err != nil {
		return nil, err
	}
	c.quiet = true
	defer func() { c.quiet = false }()
	return c.prepare(cfg, files, m, ch)
}

type groupCount struct{ unusual, nameLike, calendar int }

func countGroups(cs []*words.Cand) groupCount {
	var g groupCount
	for _, cd := range cs {
		switch cd.Group() {
		case words.GroupUnusual:
			g.unusual++
		case words.GroupNameLike:
			g.nameLike++
		default:
			g.calendar++
		}
	}
	return g
}

func runReview(c *ctx) error {
	cfg, err := c.loadConfig()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.env.Stdout, "Config: %s\n\n", config.FileName)
	if err := printReview(c.env.Stdout, cfg); err != nil {
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
	if usesWords(cfg) {
		// Counts only, in bands: the AI may run review (decision 18), and the
		// words themselves are shown only inside approve, on a terminal.
		_, _ = fmt.Fprintln(c.env.Stdout)
		r, err := c.dryRun(cfg, privDir)
		if err != nil {
			_, _ = fmt.Fprintf(c.env.Stdout, "Free text: I could not count the words (%s).\n", fail.Code(err))
			c.warnings++
		} else {
			var all []*words.Cand
			if r.finder != nil {
				all = r.finder.Candidates()
			}
			g := countGroups(all)
			_, _ = fmt.Fprintf(c.env.Stdout, "Free text, word check: unusual words %s, name-like words %s, day and month names %s; new since the last approve: %s.\n",
				shape.CountBand(g.unusual), shape.CountBand(g.nameLike), shape.CountBand(g.calendar), shape.CountBand(len(r.newWords())))
			_, _ = fmt.Fprintln(c.env.Stdout, "Review shows counts only. The person sees the words, and picks any to keep, in `standin approve`.")
		}
	}
	_, _ = fmt.Fprintln(c.env.Stdout)
	switch {
	case !needsApproval(cfg):
		_, _ = fmt.Fprintln(c.env.Stdout, "This config has no keep, scan or word check, so it needs no approval.")
	case approved:
		_, _ = fmt.Fprintln(c.env.Stdout, "Approved: yes. The keeps, scans and word choices above will be used.")
	default:
		_, _ = fmt.Fprintf(c.env.Stdout, "Approved: no. %s `standin scrub` will not run until you run `standin approve` yourself, in your own terminal.\n", why)
		c.warnings++
	}
	return nil
}

func runApprove(c *ctx) error {
	if !c.env.StdinIsTerminal() {
		return fail.New("no-terminal", "approve must be run by a person typing in a terminal, not by an AI or a script. Open a terminal, go to the project folder, and run `standin approve`.")
	}
	cfg, err := c.loadConfig()
	if err != nil {
		return err
	}
	wordCheck := usesWords(cfg)
	if wordCheck && !c.env.StdoutIsTerminal() {
		return fail.New("no-terminal", "approve shows words from your files, so it must print to your screen, not to a file or another program. Run `standin approve` in your own terminal.")
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
	out := c.env.Stdout
	_, _ = fmt.Fprintf(out, "Config: %s\n\n", config.FileName)
	if err := printReview(out, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	in := bufio.NewReader(c.env.Stdin)
	readLine := func() (string, error) {
		if err := c.output.Error(); err != nil {
			return "", err
		}
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", fail.New("no-answer", "I got no answer, so I approved nothing.")
		}
		return line, nil
	}
	ask := func(q string) (bool, error) {
		_, _ = fmt.Fprint(out, q+" Type yes: ")
		line, err := readLine()
		if err != nil {
			return false, err
		}
		a := strings.ToLower(strings.TrimSpace(line))
		return a == "yes" || a == "y", nil
	}
	var keeps, scans, texts []string
	// One question for all the keeps (or scans) in a file rule, so a long
	// config is a few questions, not one per column. The question names each.
	type groupKey struct{ action, match string }
	grouped := map[groupKey][]string{}
	for _, r := range rowsKeepsFirst(cfg) {
		if r.col.Action == config.Keep || r.col.Action == config.Scan {
			k := groupKey{r.col.Action, r.match}
			grouped[k] = append(grouped[k], r.name)
		}
	}
	asked := map[groupKey]bool{}
	for _, r := range rowsKeepsFirst(cfg) {
		var qs []string
		verb := ""
		switch r.col.Action {
		case config.Keep, config.Scan:
			k := groupKey{r.col.Action, r.match}
			if !asked[k] {
				asked[k] = true
				list := strings.Join(grouped[k], ", ")
				if r.col.Action == config.Keep {
					qs, verb = []string{fmt.Sprintf("Keep %s in %s? The AI will see these real values.", list, r.match)}, "keeping"
				} else {
					qs, verb = []string{fmt.Sprintf("Scan %s in %s? It is free text: names Standin does not know stay in, and the AI will see them.", list, r.match)}, "scanning"
				}
				r.name = list
			}
		case config.Words:
			verb = "the word check on"
			qs = []string{fmt.Sprintf("Use the word check on the free text in %s? Names and unusual words become stand-ins, except words you keep below. Words Standin does not flag stay in, and the AI will see them.", r.match)}
			for _, g := range r.groups {
				if g == words.GroupCalendar {
					qs = append(qs, fmt.Sprintf("Keep day and month names (like Monday or March) as they are in the free text of %s?", r.match))
				}
			}
		default:
			continue
		}
		for _, q := range qs {
			ok, err := ask(q)
			if err != nil {
				return err
			}
			if !ok {
				return fail.New("not-approved", "You did not approve %s %s in %s, so I approved nothing. Change it in %s (for example to tokenize or drop), then run approve again.", verb, r.name, r.match, config.FileName)
			}
		}
		switch r.col.Action {
		case config.Keep:
			keeps = append(keeps, r.match+":"+r.column)
		case config.Scan:
			scans = append(scans, r.match+":"+r.column)
		default:
			texts = append(texts, r.match+":text")
		}
	}
	var ch *words.Choices
	if wordCheck {
		ch, err = c.approveWords(cfg, privDir, out, readLine)
		if err != nil {
			return err
		}
	}
	ok, err := ask("Use this config for scrub?")
	if err != nil {
		return err
	}
	if !ok {
		return fail.New("not-approved", "Not approved. Nothing changed.")
	}
	if err := c.output.Error(); err != nil {
		return err
	}
	if err := store.Ensure(privDir); err != nil {
		return err
	}
	a := store.Approval{Version: 1, ConfigSHA256: cfg.Hash, ApprovedAt: time.Now().UTC().Format(time.RFC3339), Keeps: keeps, Scans: scans, Texts: texts}
	if a.Keeps == nil {
		a.Keeps = []string{}
	}
	var outs []store.Out
	if ch != nil {
		wb := ch.Marshal()
		a.WordsSHA256 = config.HashBytes(wb)
		outs = append(outs, store.Out{Path: filepath.Join(privDir, store.WordsFile), Data: wb, Perm: 0o600, Root: privDir})
	}
	outs = append(outs, store.Out{Path: filepath.Join(privDir, store.ApprovalFile), Data: store.MarshalApproval(a), Perm: 0o600, Root: privDir})
	if err := store.WriteAll(outs); err != nil {
		return err
	}
	c.msg("Approved. If the config changes, you need to approve it again.")
	if ch != nil {
		c.msg("Your word choices are in %s. It is as secret as the mapping. If you change it, run approve again.", filepath.Join(privDir, store.WordsFile))
	}
	return nil
}

// pageSize is how many words approve shows at a time.
const pageSize = 20

// approveWords shows the new words in the free text, page by page, and asks
// which to keep. Only words in neither list of words.json are asked about.
func (c *ctx) approveWords(cfg *config.Config, privDir string, out io.Writer, readLine func() (string, error)) (*words.Choices, error) {
	r, err := c.dryRun(cfg, privDir)
	if err != nil {
		return nil, fail.New(fail.Code(err), "I could not read the raw files to find the words to ask you about, so I approved nothing. %s", err.Error())
	}
	ch := r.choices
	var all []*words.Cand
	if r.finder != nil {
		all = r.finder.Candidates()
	}
	g := countGroups(all)
	fresh := r.newWords()
	var groupA, groupB []*words.Cand
	for _, cd := range fresh {
		if cd.Group() == words.GroupUnusual {
			groupA = append(groupA, cd)
		} else {
			groupB = append(groupB, cd)
		}
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Free text: %d unusual words, %d name-like words, %d day and month names. New since the last approve: %d.\n",
		g.unusual, g.nameLike, g.calendar, len(fresh))
	if len(fresh) == 0 {
		_, _ = fmt.Fprintln(out, "No new words to ask you about.")
		return ch, nil
	}
	_, _ = fmt.Fprintln(out, "Words you keep stay in the text the AI reads. Words you do not keep become stand-ins like [NAME_4].")
	_, _ = fmt.Fprintln(out, "Next to each word: how often I flagged it, and a short piece of the text. Other flagged words show as "+words.Mask+".")
	_, _ = fmt.Fprintln(out, "This screen shows real words from your files. Do not copy it to the AI.")
	pick := func(title, help string, list []*words.Cand) error {
		for start := 0; start < len(list); start += pageSize {
			end := start + pageSize
			if end > len(list) {
				end = len(list)
			}
			page := list[start:end]
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintf(out, "%s (%d to %d of %d). %s\n", title, start+1, end, len(list), help)
			for i, cd := range page {
				_, _ = fmt.Fprintf(out, "  %2d  %-18s x%-4d \"%s\"\n", i+1, cd.Word, cd.Count, r.finder.Context(cd))
			}
			var nums []int
			for tries := 0; ; tries++ {
				_, _ = fmt.Fprint(out, "Keep: ")
				line, err := readLine()
				if err != nil {
					return err
				}
				var ok bool
				if nums, ok = parsePicks(line, len(page)); ok {
					break
				}
				if tries >= 2 {
					return fail.New("bad-answer", "I could not read your answer, so I approved nothing. Run approve again.")
				}
				_, _ = fmt.Fprintf(out, "Type numbers from 1 to %d, like 1 3 5-7, or press Enter to keep none.\n", len(page))
			}
			kept := map[int]bool{}
			for _, n := range nums {
				kept[n] = true
			}
			for i, cd := range page {
				if kept[i+1] {
					ch.Keep[cd.Key] = true
					delete(ch.Swap, cd.Key)
				} else {
					ch.Swap[cd.Key] = true
				}
			}
		}
		return nil
	}
	if err := pick("Unusual words, most used first. Read these with care: words worth keeping (brands, streets, products) are mostly here",
		"Type the numbers to keep, like 1 3 5-7, or press Enter to keep none.", groupA); err != nil {
		return nil, err
	}
	if err := pick("These look like names. They will all become stand-ins",
		"Type numbers to keep any, or press Enter.", groupB); err != nil {
		return nil, err
	}
	return ch, nil
}

// parsePicks reads "1 3 5-7", "none" or an empty line. Every number must be
// from 1 to n.
func parsePicks(line string, n int) ([]int, bool) {
	s := strings.ToLower(strings.TrimSpace(line))
	if s == "" || s == "none" || s == "no" {
		return nil, true
	}
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' }) {
		lo, hi := f, f
		if i := strings.Index(f, "-"); i > 0 {
			lo, hi = f[:i], f[i+1:]
		}
		a, err1 := strconv.Atoi(lo)
		b, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || a < 1 || b > n || a > b {
			return nil, false
		}
		for k := a; k <= b; k++ {
			out = append(out, k)
		}
	}
	return out, true
}

// StdinIsTerminal reports whether stdin is a terminal (a person typing).
func StdinIsTerminal() bool { return isTerminal(os.Stdin) }

// StdoutIsTerminal reports whether stdout is a terminal (a screen, not a pipe or file).
func StdoutIsTerminal() bool { return isTerminal(os.Stdout) }

// StderrIsTerminal reports whether stderr is a terminal.
func StderrIsTerminal() bool { return isTerminal(os.Stderr) }

// isTerminal requires an OS terminal or console, not merely a character
// device. A pseudo-terminal still does not prove that a person is present.
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(st, null) {
		return false
	}
	return terminalDevice(f)
}
