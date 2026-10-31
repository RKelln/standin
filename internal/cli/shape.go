package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/shape"
	"github.com/RKelln/standin/internal/store"
	"github.com/RKelln/standin/internal/textenc"
	"github.com/RKelln/standin/internal/words"
)

func (c *ctx) version() string {
	if c.env.Version == "" {
		return "dev"
	}
	return c.env.Version
}

// load parses a raw file, adding the file name to a refusal.
func (c *ctx) load(f rawFile) (*dataset.Dataset, error) {
	d, err := dataset.Load(f.abs)
	if err != nil {
		code := fail.Code(err)
		return nil, fail.New(code, "%s: %s", show(f.rel), err.Error())
	}
	return d, nil
}

func (c *ctx) noteEncoding(f rawFile, d *dataset.Dataset) {
	if d.Encoding == textenc.Windows1252 {
		c.warn("%s is not UTF-8. I read it as Windows-1252 (common on Windows). Check that letters like é look right in the output.", show(f.rel))
	}
}

// textCounts counts the candidate words and patterns in the free text of
// one file, with no mapping (shape runs before there is one).
func textCounts(d *dataset.Dataset) shape.TextCounts {
	tc := shape.TextCounts{Lines: d.FreeTextLines(), Patterns: map[string]int{}}
	f := words.NewFinder(nil)
	lines := d.TextLines()
	f.AddLines(lines)
	for _, cd := range f.Candidates() {
		switch cd.Group() {
		case words.GroupUnusual:
			tc.Unusual++
		case words.GroupNameLike:
			tc.NameLike++
		default:
			tc.Calendar++
		}
	}
	for _, l := range lines {
		for _, h := range scan.FindPatterns(l, nil) {
			tc.Patterns[h.Kind]++
		}
	}
	return tc
}

func runShape(c *ctx) error {
	files, err := c.rawFiles()
	if err != nil {
		return err
	}
	shapeDir := filepath.Join(c.project, "shape")
	used := map[string]bool{}
	var outs []store.Out
	type note struct {
		out    string
		cols   int
		hidden int
		text   bool
	}
	var notes []note
	for _, f := range files {
		d, err := c.load(f)
		if err != nil {
			return err
		}
		c.noteEncoding(f, d)
		shown := f.rel
		var out string
		if dataLikeName(f.inRaw) {
			shown = ""
			for n := 1; ; n++ {
				name := fmt.Sprintf("file_%d.shape.md", n)
				p := filepath.Join(shapeDir, name)
				if _, err := os.Stat(p); isNotExist(err) && !used[p] {
					out = p
					break
				}
			}
			c.warn("One file name looks like private data. Its shape file is %s. Rename the raw file to something plain (like invoices.csv), then run shape again.", filepath.Base(out))
		} else {
			out = filepath.Join(shapeDir, filepath.FromSlash(f.inRaw)+".shape.md")
		}
		used[out] = true
		hidden := 0
		for _, ci := range shape.Columns(d) {
			if ci.HeaderFlag != "" && ci.HeaderFlag != "empty" {
				hidden++
			}
		}
		if hidden > 0 {
			c.warn("%s: %d column header(s) look like private data (names, emails, numbers). The shape file hides them as [COLUMN_n].", show(f.rel), hidden)
		}
		md := ""
		if d.Kind == dataset.Text {
			md = shape.TextMarkdown(d, shown, c.version(), textCounts(d))
		} else {
			md = shape.Markdown(d, shown, c.version())
		}
		outs = append(outs, store.Out{Path: out, Data: []byte(md), Perm: 0o644})
		notes = append(notes, note{out, len(d.Columns), hidden, d.Kind == dataset.Text})
	}
	if err := store.WriteAll(outs); err != nil {
		return err
	}
	for _, n := range notes {
		r, _ := c.rel(n.out)
		if n.text {
			c.msg("Wrote %s (free text; %d fields and table columns).", r, n.cols)
		} else {
			c.msg("Wrote %s (%d columns).", r, n.cols)
		}
	}
	c.msg("The shape files hold no real values. Give them to the AI and ask it to write %s.", "standin.config.json")
	return nil
}
