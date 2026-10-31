// Package restore puts real values back in place of stand-ins.
//
// It is strict: it only changes exact stand-ins it knows, like [PERSON_1].
// Things that look almost like a stand-in ([person_1], PERSON_1, [PERSON 1],
// Person 1) are reported, never changed.
package restore

import (
	"bytes"
	"encoding/csv"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/RKelln/standin/internal/fail"
	"github.com/RKelln/standin/internal/jsonsafe"
	"github.com/RKelln/standin/internal/mapping"
)

// Mode says how values are written back.
type Mode int

// Modes.
const (
	Text Mode = iota // plain text
	CSV              // CSV, comma
	TSV              // CSV, tab
	JSON             // values are escaped as JSON string text
)

// Finding is a stand-in-like thing restore did not change.
type Finding struct {
	Line int
	Text string // the stand-in-like text (never a real value)
	Like string // the stand-in it looks like, if known
}

// Result is what restore did.
type Result struct {
	Out       string
	Replaced  int
	Unknown   []Finding // exact form, but not in the mapping
	NearMiss  []Finding // almost a stand-in
	Spellings []string  // stand-ins used that have more than one spelling
	Blocked   []Finding // known stand-ins left alone: unsafe link, formula or tag context
}

// Issues reports whether there is anything for the person to look at.
func (r Result) Issues() bool {
	return len(r.Unknown) > 0 || len(r.NearMiss) > 0 || len(r.Spellings) > 0 || len(r.Blocked) > 0
}

var (
	exact = mapping.StandinPattern
	// [person_1], [Person-1], [PERSON 1], [PERSON1]
	nearBracket = regexp.MustCompile(`\[\s*([A-Za-z]+)\s*[ _-]?\s*([0-9]+)\s*\]`)
	// PERSON_1 without brackets
	nearBare = regexp.MustCompile(`\b([A-Z]+)_([1-9][0-9]*)\b`)
	// Person 1, person_1
	nearWord = regexp.MustCompile(`(?i)\b(person|org|email|phone|account|id|address|postal|key|value)[ _]([1-9][0-9]*)\b`)
	// A web link, or the target of a Markdown or HTML link. A real value put
	// there could be sent out when the file is opened.
	linkRe = regexp.MustCompile(`(?i)(?:https?://|www\.|\]\(|\bsrc\s*=\s*["']?|\bhref\s*=\s*["']?)[^\s"'<>)]*`)
	// Values must not become part of an HTML tag. This also catches tags
	// introduced by a value, rather than present in the agent's answer.
	tagRe = regexp.MustCompile(`(?is)</?[a-z][a-z0-9:-]*(?:\s[^<>]*)?/?>`)
	// Plain text restore is not an HTML sanitizer. Refuse active containers,
	// including unclosed ones, rather than insert secrets into their bodies.
	activeRe = regexp.MustCompile(`(?i)<\s*(?:script|style|iframe|object|embed|svg|math|template|noscript)\b`)
)

// isFormula reports a spreadsheet cell that a program may run as a formula.
func isFormula(cell string) bool {
	t := strings.TrimSpace(cell)
	return t != "" && strings.ContainsRune("=+-@", rune(t[0]))
}

// Restore replaces stand-ins in text.
func Restore(text string, m *mapping.Mapping, mode Mode) (Result, error) {
	if mode == Text && activeRe.MatchString(text) {
		return Result{}, fail.New("active-content", "This file holds web-page code that may run when opened. I did not restore it. Copy the words into a plain text file without that code, then try again.")
	}
	if mode == CSV || mode == TSV {
		for i := 0; i < len(text); i++ {
			if text[i] == '\r' && (i+1 == len(text) || text[i+1] != '\n') {
				return Result{}, fail.New("bad-csv", "This table uses lone carriage returns, so I cannot tell where its rows end. Save it with normal line endings, then try again.")
			}
		}
	}
	if mode == JSON && jsonsafe.Validate([]byte(text)) != nil {
		return Result{}, fail.New("bad-json", "This JSON file is not valid, or repeats a name in one object. I did not restore it. Fix the file, or save the fragment as .txt and try again.")
	}
	var r Result
	analyze(text, m, &r)
	used := map[string]bool{}
	// replace swaps known stand-ins in s, except where they would put a real
	// value into a link, formula or HTML tag. Check both the source and the
	// expanded result. lineOf gives the source line of a place in s.
	replace := func(s string, esc func(string) string, formulaCell bool, lineOf func(int) int) string {
		links := linkRe.FindAllStringIndex(s, -1)
		links = append(links, tagRe.FindAllStringIndex(s, -1)...)
		type replacement struct {
			a, b       int
			tok, value string
			blocked    bool
			outA, outB int
		}
		var reps []replacement
		for _, loc := range exact.FindAllStringIndex(s, -1) {
			tok := s[loc[0]:loc[1]]
			e, ok := m.Lookup(tok)
			if !ok {
				continue
			}
			blocked := formulaCell && isFormula(s)
			for _, l := range links {
				if loc[0] >= l[0] && loc[0] < l[1] {
					blocked = true
				}
			}
			reps = append(reps, replacement{a: loc[0], b: loc[1], tok: tok, value: esc(e.Value()), blocked: blocked})
		}
		// Rebuild after blocking a substitution: two adjacent stand-ins can
		// jointly form a link, so checking each value alone is not enough.
		var out string
		for {
			var b strings.Builder
			last := 0
			for i := range reps {
				p := &reps[i]
				b.WriteString(s[last:p.a])
				p.outA = b.Len()
				if p.blocked {
					b.WriteString(p.tok)
				} else {
					b.WriteString(p.value)
				}
				p.outB = b.Len()
				last = p.b
			}
			b.WriteString(s[last:])
			out = b.String()
			hazards := linkRe.FindAllStringIndex(out, -1)
			hazards = append(hazards, tagRe.FindAllStringIndex(out, -1)...)
			changed := false
			for i := range reps {
				p := &reps[i]
				if p.blocked {
					continue
				}
				unsafe := formulaCell && isFormula(out)
				for _, h := range hazards {
					if p.outA < h[1] && p.outB > h[0] {
						unsafe = true
					}
				}
				if unsafe {
					p.blocked = true
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		for _, p := range reps {
			if p.blocked {
				r.Blocked = append(r.Blocked, Finding{Line: lineOf(p.a), Text: p.tok})
			} else {
				r.Replaced++
				used[p.tok] = true
			}
		}
		return out
	}
	starts := lineStarts(text)
	textLine := func(pos int) int { return lineAt(starts, pos) }
	plain := func(s string) string { return s }
	switch mode {
	case Text:
		r.Out = replace(text, plain, false, textLine)
	case JSON:
		r.Out = replace(text, func(v string) string {
			q := jsonQuote(v)
			return q[1 : len(q)-1]
		}, false, textLine)
	case CSV, TSV:
		comma := ','
		if mode == TSV {
			comma = '\t'
		}
		rd := csv.NewReader(strings.NewReader(text))
		rd.Comma = comma
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.Comma = comma
		for {
			rec, err := rd.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return Result{}, fail.New("bad-csv", "I could not read this file as a table, so I did not restore it. Save it as plain text (.txt or .md) and try again.")
			}
			for i := range rec {
				line, _ := rd.FieldPos(i)
				cellLine := func(pos int) int { return line + strings.Count(rec[i][:pos], "\n") }
				rec[i] = replace(rec[i], plain, true, cellLine)
			}
			if err := w.Write(rec); err != nil {
				return Result{}, fail.New("write", "I could not build the restored table.")
			}
		}
		w.Flush()
		if w.Error() != nil {
			return Result{}, fail.New("write", "I could not build the restored table.")
		}
		r.Out = buf.String()
	}
	if mode == JSON && jsonsafe.Validate([]byte(r.Out)) != nil {
		return Result{}, fail.New("bad-json", "Putting the real values back would make invalid JSON or repeat a name in one object. I wrote nothing. Rename the object entries, then try again.")
	}
	for tok := range used {
		if e, _ := m.Lookup(tok); len(e.Spellings) > 1 {
			r.Spellings = append(r.Spellings, tok)
		}
	}
	sort.Strings(r.Spellings)
	return r, nil
}

func lineAt(starts []int, pos int) int {
	return sort.Search(len(starts), func(i int) bool { return starts[i] > pos })
}

func lineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func analyze(text string, m *mapping.Mapping, r *Result) {
	starts := lineStarts(text)
	var taken [][2]int
	for _, loc := range exact.FindAllStringSubmatchIndex(text, -1) {
		taken = append(taken, [2]int{loc[0], loc[1]})
		tok := text[loc[0]:loc[1]]
		if !mapping.ValidCategory(text[loc[2]:loc[3]]) {
			continue // like [COLUMN_2]: not a stand-in
		}
		if _, ok := m.Lookup(tok); !ok {
			r.Unknown = append(r.Unknown, Finding{Line: lineAt(starts, loc[0]), Text: tok})
		}
	}
	free := func(a, b int) bool {
		for _, t := range taken {
			if a < t[1] && b > t[0] {
				return false
			}
		}
		return true
	}
	add := func(a, b int, cat, num string, needKnown bool) {
		if !free(a, b) {
			return
		}
		like := "[" + strings.ToUpper(cat) + "_" + strings.TrimLeft(num, "0") + "]"
		_, known := m.Lookup(like)
		if needKnown && !known {
			return
		}
		if !known {
			like = ""
		}
		taken = append(taken, [2]int{a, b})
		r.NearMiss = append(r.NearMiss, Finding{Line: lineAt(starts, a), Text: text[a:b], Like: like})
	}
	for _, loc := range nearBracket.FindAllStringSubmatchIndex(text, -1) {
		cat := text[loc[2]:loc[3]]
		if !mapping.ValidCategory(strings.ToUpper(cat)) {
			continue
		}
		add(loc[0], loc[1], cat, text[loc[4]:loc[5]], false)
	}
	for _, loc := range nearBare.FindAllStringSubmatchIndex(text, -1) {
		if !mapping.ValidCategory(text[loc[2]:loc[3]]) {
			continue
		}
		add(loc[0], loc[1], text[loc[2]:loc[3]], text[loc[4]:loc[5]], false)
	}
	for _, loc := range nearWord.FindAllStringSubmatchIndex(text, -1) {
		add(loc[0], loc[1], text[loc[2]:loc[3]], text[loc[4]:loc[5]], true)
	}
	sort.Slice(r.NearMiss, func(i, j int) bool { return r.NearMiss[i].Line < r.NearMiss[j].Line })
}

func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strconv.FormatInt(int64(c)>>4, 16))
				b.WriteString(strconv.FormatInt(int64(c)&0xF, 16))
			} else {
				b.WriteRune(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
