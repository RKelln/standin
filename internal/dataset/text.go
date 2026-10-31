package dataset

import (
	"encoding/csv"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/scan"
	"github.com/RKelln/standin/internal/words"
)

// Text is the kind for free-text files (.md and .txt).
const Text = "text"

// Where a column of a text file comes from.
const (
	FromField = "field" // a "Label: value" line
	FromTable = "table" // a Markdown table
)

type lineKind int

const (
	lineText lineKind = iota
	lineField
	lineTableHead
	lineTableSep
	lineTableRow
)

type textLine struct {
	kind  lineKind
	raw   string // without the line ending
	eol   string // "\n", "\r\n" or "" (last line)
	col   int    // field: its column
	cols  []int  // table: the column of each cell
	spans [][2]int
}

// MaxUncasedShare: a text file with more than this share of its letters in a
// script with no capital letters (Chinese, Arabic, Hebrew ...) is refused,
// since the word check cannot find names in it. Fewer than minUncased such
// letters never refuse (a name or two is fine: those words are swapped).
const (
	MaxUncasedShare = 0.2
	minUncased      = 10
)

var (
	fieldRe = regexp.MustCompile(`^(\s*(?:[-*+]\s+)?(?:\*\*|__)?)(\p{Lu}[\pL'’-]*(?: [\pL'’-]+){0,2})((?:\*\*|__)?:(?:\*\*|__)?[ \t]+)(\S.*?)\s*$`)
	sepRe   = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$`)
)

// Labels often used on "Label: value" lines that may not be English words.
var fieldWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`requester attendees attendee regrets cc bcc re fwd worker caller assignee reporter
		organizer organiser host chair signed sent to from subject date client contact owner`) {
		fieldWords[w] = true
	}
}

// fieldLabel reports whether a label can start a field line. Each word must be
// a common label word or an English word that is not also a first name, so
// "Leila: see you then" (a chat line) stays text and gets the word check.
func fieldLabel(label string) bool {
	if utf8.RuneCountInString(label) > 30 {
		return false
	}
	if _, ok := scan.LooksLikeDataName(label); ok {
		return false
	}
	for _, w := range strings.Fields(label) {
		lw := strings.ToLower(w)
		if fieldWords[lw] || scan.IsCommonHeaderWord(lw) {
			continue
		}
		if !words.InDict(w) || words.IsFirstName(w) {
			return false
		}
	}
	return true
}

// cellSpans splits a table line on | (not \|) and returns the trimmed span
// of each cell. A leading and a trailing | are edges, not empty cells.
func cellSpans(s string) [][2]int {
	var cuts []int
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '|' {
			cuts = append(cuts, i)
		}
	}
	start := 0
	var segs [][2]int
	for _, c := range cuts {
		segs = append(segs, [2]int{start, c})
		start = c + 1
	}
	segs = append(segs, [2]int{start, len(s)})
	if len(segs) > 1 && strings.TrimSpace(s[segs[0][0]:segs[0][1]]) == "" && len(cuts) > 0 && strings.TrimSpace(s[:cuts[0]]) == "" {
		segs = segs[1:]
	}
	if len(segs) > 1 && len(cuts) > 0 && strings.TrimSpace(s[cuts[len(cuts)-1]+1:]) == "" {
		segs = segs[:len(segs)-1]
	}
	for i, g := range segs {
		a, b := g[0], g[1]
		for a < b && (s[a] == ' ' || s[a] == '\t') {
			a++
		}
		for b > a && (s[b-1] == ' ' || s[b-1] == '\t') {
			b--
		}
		segs[i] = [2]int{a, b}
	}
	return segs
}

func hasPipe(s string) bool { return strings.Contains(strings.ReplaceAll(s, `\|`, ""), "|") }

// looksLikeTable: the text reads as a CSV or TSV table with at least 3 rows
// and 2 columns, every row the same width. Such a .txt must be named .csv or
// .tsv, so it gets the column rules.
func looksLikeTable(text string) bool {
	for _, sep := range []rune{',', '\t'} {
		r := csv.NewReader(strings.NewReader(text))
		r.Comma = sep
		r.FieldsPerRecord = 0
		recs, err := r.ReadAll()
		if err == nil && len(recs) >= 3 && len(recs[0]) >= 2 {
			return true
		}
	}
	return false
}

func parseText(text string) (*Dataset, error) {
	if err := words.Load(); err != nil {
		return nil, err
	}
	cased, uncased := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			if words.HasCase(r) {
				cased++
			} else {
				uncased++
			}
		}
	}
	if uncased >= minUncased && float64(uncased) > MaxUncasedShare*float64(cased+uncased) {
		return nil, fail.New("uncased-script", "This file is mostly in a language Standin cannot check yet (a script with no capital letters, like Chinese or Arabic), so I cannot find the names in it. Remove private details by hand.")
	}
	d := &Dataset{colOf: map[string]int{}}
	raw := strings.Split(text, "\n")
	for i, l := range raw {
		tl := textLine{raw: l, eol: "\n"}
		if i == len(raw)-1 {
			tl.eol = ""
		}
		if strings.HasSuffix(l, "\r") {
			tl.raw, tl.eol = l[:len(l)-1], "\r"+tl.eol
		}
		d.lines = append(d.lines, tl)
	}
	d.Rows = len(d.lines)
	if d.lines[len(d.lines)-1].raw == "" && len(d.lines) > 1 {
		d.Rows-- // a file that ends in a line break has no extra line
	}
	addCol := func(key, name, src string) int {
		if i, ok := d.colOf[key]; ok {
			return i
		}
		d.colOf[key] = len(d.Columns)
		d.Columns = append(d.Columns, name)
		d.Source = append(d.Source, src)
		d.Values = append(d.Values, nil)
		d.Recs = append(d.Recs, nil)
		return len(d.Columns) - 1
	}
	for i := 0; i < len(d.lines); i++ {
		l := &d.lines[i]
		// A Markdown table: a header line, then a |---| line.
		if hasPipe(l.raw) && i+1 < len(d.lines) && sepRe.MatchString(d.lines[i+1].raw) && hasPipe(d.lines[i+1].raw) {
			head := cellSpans(l.raw)
			if len(cellSpans(d.lines[i+1].raw)) == len(head) {
				d.Tables++
				t := strconv.Itoa(d.Tables)
				l.kind, l.spans = lineTableHead, head
				for j, sp := range head {
					name := l.raw[sp[0]:sp[1]]
					l.cols = append(l.cols, addCol("t"+t+"\x00"+strconv.Itoa(j), name, FromTable+" "+t))
				}
				d.lines[i+1].kind = lineTableSep
				j := i + 2
				for ; j < len(d.lines) && strings.TrimSpace(d.lines[j].raw) != "" && hasPipe(d.lines[j].raw); j++ {
					row := &d.lines[j]
					sp := cellSpans(row.raw)
					if len(sp) != len(head) {
						return nil, fail.New("ragged", "Line %d is a table row with a different number of cells than the table's header line. Every row needs the same number of cells. Fix that line, then try again.", j+1)
					}
					row.kind, row.spans, row.cols = lineTableRow, sp, l.cols
					for k, s := range sp {
						c := l.cols[k]
						d.Values[c] = append(d.Values[c], row.raw[s[0]:s[1]])
						d.Recs[c] = append(d.Recs[c], j+1)
					}
				}
				i = j - 1
				continue
			}
		}
		if m := fieldRe.FindStringSubmatchIndex(l.raw); m != nil {
			label := l.raw[m[4]:m[5]]
			value := l.raw[m[8]:m[9]]
			if !strings.HasPrefix(value, "//") && fieldLabel(label) {
				l.kind = lineField
				l.col = addCol("f\x00"+label, label, FromField)
				l.spans = [][2]int{{m[8], m[9]}}
				d.Values[l.col] = append(d.Values[l.col], value)
				d.Recs[l.col] = append(d.Recs[l.col], i+1)
			}
		}
	}
	return d, nil
}

// TextLines returns the free-text lines of a text file, with "" in place of
// field and table lines, so line numbers still match.
func (d *Dataset) TextLines() []string {
	out := make([]string, len(d.lines))
	for i, l := range d.lines {
		if l.kind == lineText {
			out[i] = l.raw
		}
	}
	return out
}

// FreeTextLines counts the non-blank free-text lines.
func (d *Dataset) FreeTextLines() int {
	n := 0
	for _, l := range d.lines {
		if l.kind == lineText && strings.TrimSpace(l.raw) != "" {
			n++
		}
	}
	return n
}

// TableHeaders returns the header names of table t (from 1).
func (d *Dataset) TableHeaders(t int) []string {
	var out []string
	for i, src := range d.Source {
		if src == FromTable+" "+strconv.Itoa(t) {
			out = append(out, d.Columns[i])
		}
	}
	return out
}

func (d *Dataset) writeText(rw Rewriter) []byte {
	var b strings.Builder
	for i, l := range d.lines {
		out := l.raw
		switch l.kind {
		case lineText:
			if rw.Text != nil {
				out = rw.Text(i, l.raw)
			}
		case lineField:
			sp := l.spans[0]
			if nv, changed := rw.Cell(l.col, l.raw[sp[0]:sp[1]]); changed {
				out = l.raw[:sp[0]] + nv + l.raw[sp[1]:]
			}
		case lineTableHead, lineTableRow:
			out = ""
			last := 0
			for k, sp := range l.spans {
				v := l.raw[sp[0]:sp[1]]
				nv := v
				if l.kind == lineTableHead {
					nv = rw.Name(v)
				} else if x, changed := rw.Cell(l.cols[k], v); changed {
					nv = x
				}
				out += l.raw[last:sp[0]] + nv
				last = sp[1]
			}
			out += l.raw[last:]
		}
		b.WriteString(out)
		b.WriteString(l.eol)
	}
	return []byte(b.String())
}
