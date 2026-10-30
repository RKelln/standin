package scan

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/mapping"
)

// MinKnownLen is the shortest value (in characters) the known-values pass looks for.
// Shorter values give too many false hits.
const MinKnownLen = 3

// Known finds values from the mapping in text: case-insensitive, whole
// words, longest first.
type Known struct {
	entries []mapping.Entry // sorted longest key first
	all     *regexp.Regexp
	one     []*regexp.Regexp // lazily built, one per entry
}

// NewKnown builds a finder from the mapping. It returns nil if there is nothing to find.
func NewKnown(m *mapping.Mapping) *Known {
	seen := map[string]bool{}
	var es []mapping.Entry
	for _, e := range m.Entries {
		if utf8.RuneCountInString(e.Key) < MinKnownLen || seen[e.Key] {
			continue
		}
		seen[e.Key] = true
		es = append(es, e)
	}
	if len(es) == 0 {
		return nil
	}
	sort.SliceStable(es, func(i, j int) bool { return len(es[i].Key) > len(es[j].Key) })
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = phrase(e.Key)
	}
	return &Known{
		entries: es,
		all:     regexp.MustCompile(`(?i)(?:` + strings.Join(parts, "|") + `)`),
		one:     make([]*regexp.Regexp, len(es)),
	}
}

// phrase turns a normalised value into a pattern that allows any run of spaces between words.
func phrase(key string) string {
	words := strings.Fields(key)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return strings.Join(words, `\s+`)
}

// boundaryOK checks the whole-word rule, in any script. A value that starts
// or ends with a sign (like "+1 ...") has no rule on that side.
func boundaryOK(s string, a, b int) bool {
	first, _ := utf8.DecodeRuneInString(s[a:b])
	last, _ := utf8.DecodeLastRuneInString(s[a:b])
	if IsWordRune(first) {
		if r, ok := runeBefore(s, a); ok && IsWordRune(r) {
			return false
		}
	}
	if IsWordRune(last) {
		if r, ok := runeAfter(s, b); ok && IsWordRune(r) {
			return false
		}
	}
	return true
}

// Find returns the known values in s.
func (k *Known) Find(s string) []Hit {
	if k == nil {
		return nil
	}
	var hits []Hit
	pos := 0
	for pos < len(s) {
		loc := k.all.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		a, b := loc[0]+pos, loc[1]+pos
		if boundaryOK(s, a, b) {
			e := k.which(s[a:b])
			hits = append(hits, Hit{Start: a, End: b, Kind: "known value", Category: e.Category, Standin: e.Standin})
			pos = b
			continue
		}
		// The longest choice broke the whole-word rule; try each value at this spot.
		if h, ok := k.tryAt(s, a); ok {
			hits = append(hits, h)
			pos = h.End
			continue
		}
		_, size := utf8.DecodeRuneInString(s[a:])
		pos = a + size
	}
	return hits
}

func (k *Known) re(i int) *regexp.Regexp {
	if k.one[i] == nil {
		k.one[i] = regexp.MustCompile(`(?i)^(?:` + phrase(k.entries[i].Key) + `)`)
	}
	return k.one[i]
}

// which finds the entry whose value matched text.
func (k *Known) which(text string) mapping.Entry {
	for i := range k.entries {
		if loc := k.re(i).FindStringIndex(text); loc != nil && loc[1] == len(text) {
			return k.entries[i]
		}
	}
	return k.entries[0] // not reached: text came from the joined pattern
}

func (k *Known) tryAt(s string, a int) (Hit, bool) {
	for i, e := range k.entries {
		loc := k.re(i).FindStringIndex(s[a:])
		if loc == nil {
			continue
		}
		b := a + loc[1]
		if boundaryOK(s, a, b) {
			return Hit{Start: a, End: b, Kind: "known value", Category: e.Category, Standin: e.Standin}, true
		}
	}
	return Hit{}, false
}

// FindAll returns known values first, then patterns, with no overlaps, sorted by place.
func FindAll(s string, k *Known) []Hit {
	hits := k.Find(s)
	hits = append(hits, FindPatterns(s, hits)...)
	sort.Slice(hits, func(i, j int) bool { return hits[i].Start < hits[j].Start })
	return hits
}

// Replace swaps every known value and pattern match in s for a stand-in.
// Pattern matches get new stand-ins in m. It returns the new text and how
// many pattern matches (not known values) it replaced.
func Replace(s string, k *Known, m *mapping.Mapping) (string, int) {
	hits := FindAll(s, k)
	if len(hits) == 0 {
		return s, 0
	}
	var b strings.Builder
	last, patterns := 0, 0
	for _, h := range hits {
		b.WriteString(s[last:h.Start])
		st := h.Standin
		if st == "" {
			st = m.Get(h.Category, s[h.Start:h.End])
			patterns++
		}
		b.WriteString(st)
		last = h.End
	}
	b.WriteString(s[last:])
	return b.String(), patterns
}
