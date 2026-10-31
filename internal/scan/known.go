package scan

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/mapping"
)

// MinKnownLen is the shortest value (in characters) the known-values pass looks for.
// Shorter values give too many false hits.
const MinKnownLen = 3

// Known finds values from the mapping in text: case-insensitive, whole
// words, longest first.
//
// Values are indexed by their first three letters (case-folded, spaces as
// one space), so each spot in the text tries only the few values that can
// start there. (One regexp with every value joined by | was slow with a
// thousand values.)
type Known struct {
	entries []mapping.Entry // sorted longest key first
	byStart map[[3]rune][]int
	one     []*regexp.Regexp // lazily built, one per entry
}

func isSpaceLike(r rune) bool { return unicode.IsSpace(r) || unicode.Is(unicode.Z, r) }

// fold gives one form for every case form of r, as (?i) matches them.
func fold(r rune) rune {
	min := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < min {
			min = f
		}
	}
	return min
}

// start3 reads the first three letters of s from i, folded, with a run of
// spaces read as one space. ok is false if s has fewer.
func start3(s string, i int) (k [3]rune, ok bool) {
	n := 0
	space := false
	for i < len(s) && n < 3 {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if isSpaceLike(r) {
			if space {
				continue
			}
			space = true
			r = ' '
		} else {
			space = false
		}
		k[n] = fold(r)
		n++
	}
	return k, n == 3
}

// SkipKnown, when set, leaves a mapping row out of the known list. The words
// package sets it: a one-word NAME that is also an English word ("may",
// "supply", "rose") is found by the word check only, so a scan column,
// check, or a hook does not fire on the everyday word.
var SkipKnown func(e mapping.Entry) bool

// NewKnown builds a finder from the mapping. It returns nil if there is nothing to find.
// Words the person chose to keep (Entry.Kept) are left out.
func NewKnown(m *mapping.Mapping) *Known {
	seen := map[string]bool{}
	var es []mapping.Entry
	for _, e := range m.Entries {
		if e.Kept || utf8.RuneCountInString(e.Key) < MinKnownLen || seen[e.Key] || (SkipKnown != nil && SkipKnown(e)) {
			continue
		}
		seen[e.Key] = true
		es = append(es, e)
	}
	if len(es) == 0 {
		return nil
	}
	sort.SliceStable(es, func(i, j int) bool { return len(es[i].Key) > len(es[j].Key) })
	k := &Known{entries: es, byStart: map[[3]rune][]int{}, one: make([]*regexp.Regexp, len(es))}
	for i, e := range es {
		if st, ok := start3(e.Key, 0); ok {
			k.byStart[st] = append(k.byStart[st], i)
		}
	}
	return k
}

// phrase turns a normalised value into a pattern that allows any run of spaces between words.
func phrase(key string) string {
	words := strings.FieldsFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.Is(unicode.Zs, r) })
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	// Any run of spaces, including no-break and other Unicode spaces.
	return strings.Join(words, `[\s\p{Z}]+`)
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

// Find returns the known values in s: at each spot, left to right, the
// longest value that matches there as whole words.
func (k *Known) Find(s string) []Hit {
	if k == nil {
		return nil
	}
	var hits []Hit
	for pos := 0; pos < len(s); {
		_, size := utf8.DecodeRuneInString(s[pos:])
		if st, ok := start3(s, pos); ok {
			if h, ok := k.tryAt(s, pos, k.byStart[st]); ok {
				hits = append(hits, h)
				pos = h.End
				continue
			}
		}
		pos += size
	}
	return hits
}

// withTail takes in the trailing . , ; or : that the key leaves out, when the
// text has the same ones as the first spelling ("Harbourline Courier Co."),
// so restore does not write the dot twice.
func withTail(s string, b int, e mapping.Entry) int {
	v := strings.TrimSpace(e.Value())
	tail := v[len(strings.TrimRight(v, ".,;:")):]
	if tail != "" && strings.HasPrefix(s[b:], tail) {
		return b + len(tail)
	}
	return b
}

func (k *Known) re(i int) *regexp.Regexp {
	if k.one[i] == nil {
		k.one[i] = regexp.MustCompile(`(?i)^(?:` + phrase(k.entries[i].Key) + `)`)
	}
	return k.one[i]
}

func (k *Known) tryAt(s string, a int, idx []int) (Hit, bool) {
	for _, i := range idx {
		e := k.entries[i]
		loc := k.re(i).FindStringIndex(s[a:])
		if loc == nil {
			continue
		}
		b := a + loc[1]
		if boundaryOK(s, a, b) {
			return Hit{Start: a, End: withTail(s, b, e), Kind: "known value", Category: e.Category, Standin: e.Standin}, true
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
