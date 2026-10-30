// Package restore puts real values back in place of stand-ins.
//
// It is strict: it only changes exact stand-ins it knows, like [PERSON_1].
// Things that look almost like a stand-in ([person_1], PERSON_1, [PERSON 1],
// Person 1) are reported, never changed.
package restore

import (
	"bytes"
	"encoding/csv"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/RKelln/standin/internal/fail"
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
}

// Issues reports whether there is anything for the person to look at.
func (r Result) Issues() bool {
	return len(r.Unknown) > 0 || len(r.NearMiss) > 0 || len(r.Spellings) > 0
}

var (
	exact = mapping.StandinPattern
	// [person_1], [Person-1], [PERSON 1], [PERSON1]
	nearBracket = regexp.MustCompile(`\[\s*([A-Za-z]+)\s*[ _-]?\s*([0-9]+)\s*\]`)
	// PERSON_1 without brackets
	nearBare = regexp.MustCompile(`\b([A-Z]+)_([1-9][0-9]*)\b`)
	// Person 1, person_1
	nearWord = regexp.MustCompile(`(?i)\b(person|org|email|phone|account|id|address|postal|key|value)[ _]([1-9][0-9]*)\b`)
)

// Restore replaces stand-ins in text.
func Restore(text string, m *mapping.Mapping, mode Mode) (Result, error) {
	var r Result
	analyze(text, m, &r)
	used := map[string]bool{}
	replace := func(s string, esc func(string) string) string {
		return exact.ReplaceAllStringFunc(s, func(tok string) string {
			e, ok := m.Lookup(tok)
			if !ok {
				return tok
			}
			r.Replaced++
			used[tok] = true
			return esc(e.Value())
		})
	}
	plain := func(s string) string { return s }
	switch mode {
	case Text:
		r.Out = replace(text, plain)
	case JSON:
		r.Out = replace(text, func(v string) string {
			q := jsonQuote(v)
			return q[1 : len(q)-1]
		})
	case CSV, TSV:
		comma := ','
		if mode == TSV {
			comma = '\t'
		}
		rd := csv.NewReader(strings.NewReader(text))
		rd.Comma = comma
		rd.FieldsPerRecord = -1
		rd.LazyQuotes = true
		recs, err := rd.ReadAll()
		if err != nil {
			return Result{}, fail.New("bad-csv", "I could not read this file as a table, so I did not restore it. Save it as plain text (.txt or .md) and try again.")
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		w.Comma = comma
		for _, rec := range recs {
			for i := range rec {
				rec[i] = replace(rec[i], plain)
			}
			if err := w.Write(rec); err != nil {
				return Result{}, fail.New("write", "I could not build the restored table.")
			}
		}
		w.Flush()
		r.Out = buf.String()
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

func analyze(text string, m *mapping.Mapping, r *Result) {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
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
