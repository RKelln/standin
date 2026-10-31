package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/RKelln/standin/internal/dataset"
	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/mapping"
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

// load parses a raw file. Until every file has been read, a later value could
// reveal that this filename is private, so a batch run does not print names.
// A file named on the command line is already on the person's screen, so its
// name is printed back unless the name itself looks like data (decision 70).
func (c *ctx) load(f rawFile) (*dataset.Dataset, error) {
	d, err := dataset.Load(f.abs)
	if err != nil {
		code := fail.Code(err)
		if f.named && !dataLikeName(f.inRaw) {
			return nil, fail.New(code, "%s: %s", show(f.rel), err.Error())
		}
		ext := safeExt(f.inRaw)
		if ext != "" {
			ext = ", " + ext
		}
		return nil, fail.New(code, "A raw file (name hidden%s): %s", ext, err.Error())
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
	// A filename may itself repeat a private cell, even when that word is
	// ordinary English. This transient list is never saved or shown.
	names := mapping.New()
	for _, f := range files {
		d, err := c.load(f)
		if err != nil {
			return err
		}
		for _, values := range d.Values {
			for _, value := range values {
				if value != "" {
					names.Get("VALUE", value)
				}
			}
		}
	}
	shapeDir := filepath.Join(c.project, "shape")
	var outs []store.Out
	type note struct {
		out    string
		cols   int
		hidden int
		text   bool
		fields int
		tcols  int
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
		if dataLikeName(f.inRaw) || knownDataName(f.inRaw, names) {
			shown = ""
			out = filepath.Join(shapeDir, anonymousName(f.inRaw))
			c.warn("One file name looks like private data. Its shape file is %s. Rename the raw file to something plain (like invoices.csv), then run shape again.", filepath.Base(out))
		} else {
			out = filepath.Join(shapeDir, filepath.FromSlash(f.inRaw)+".shape.md")
		}
		hidden := 0
		for _, ci := range shape.Columns(d) {
			if ci.HeaderFlag != "" && ci.HeaderFlag != "empty" {
				hidden++
			}
		}
		if hidden > 0 {
			label := show(f.rel)
			if shown == "" {
				label = "raw/(name hidden)"
			}
			c.warn("%s: %d column header(s) look like private data (names, emails, numbers). The shape file hides them as [COLUMN_n].", label, hidden)
		}
		md := ""
		if d.Kind == dataset.Text {
			md = shape.TextMarkdown(d, shown, c.version(), textCounts(d))
		} else {
			md = shape.Markdown(d, shown, c.version())
		}
		outs = append(outs, store.Out{Path: out, Data: []byte(md), Perm: 0o644, Root: shapeDir})
		nf := 0
		for _, src := range d.Source {
			if src == dataset.FromField {
				nf++
			}
		}
		notes = append(notes, note{out: out, cols: len(d.Columns), hidden: hidden, text: d.Kind == dataset.Text,
			fields: nf, tcols: len(d.Columns) - nf})
	}
	if err := store.WriteAll(outs); err != nil {
		return err
	}
	for _, n := range notes {
		r, _ := c.rel(n.out)
		if n.text {
			c.msg("Wrote %s (free text; %s).", r, countLabel(n.fields, n.tcols))
		} else {
			c.msg("Wrote %s (%d columns).", r, n.cols)
		}
	}
	c.msg("The shape files hold no real values. Give them to the AI and ask it to write %s.", "standin.config.json")
	return nil
}

// safeExt returns the extension only when it is a plain one (".csv", ".xlsx"),
// so a file named ".Zelda Quorn" cannot leak its name through this message.
func safeExt(name string) string {
	ext := filepath.Ext(name)
	if len(ext) < 3 || len(ext) > 9 {
		return ""
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return ""
		}
	}
	return ext
}

// anonymousName names the shape file of a raw file whose own name must not
// appear. It is a hash of the project-relative path, so the same file always
// gets the same name: a second shape run overwrites its own file instead of
// leaving a stale one behind (decision 71).
func anonymousName(inRaw string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(inRaw)))
	return "file_" + hex.EncodeToString(sum[:3]) + ".shape.md"
}

// countLabel counts fields and table columns in words: "1 field and 2 table
// columns", "3 table columns", "no fields or table columns".
func countLabel(fields, cols int) string {
	switch {
	case fields == 0 && cols == 0:
		return "no fields or table columns"
	case fields == 0:
		return plural(cols, "table column")
	case cols == 0:
		return plural(fields, "field")
	}
	return plural(fields, "field") + " and " + plural(cols, "table column")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
