package words

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RKelln/standin/internal/scan"
)

// Reason is why a word was picked out (a set of bits).
type Reason uint16

// Reasons. The research notes (docs/research/free-text.md, section 2) give the rules.
const (
	NameSpot    Reason = 1 << iota // after Hi, Dear, Thanks, "name is", a title ...
	SignOff                        // a short line after "Thanks," or "--"
	NotInDict                      // capitalised and not an English word
	FirstName                      // capitalised, in the first-name list, not at a sentence start
	NextToName                     // capitalised, right after a word picked out as a name
	Calendar                       // a day or month name
	FieldPart                      // a capitalised word from a tokenized "Label: value" line
	Uncased                        // a word in a script with no capital letters (Chinese, Arabic ...)
	KnownName                      // a one-word NAME value from the mapping (swapped before)
	MachineText                    // a long encoded run: a tracking id, a base64 blob, a hex digest
)

// nameLike are the reasons that put a word in the name-like group.
const nameLike = NameSpot | SignOff | FirstName | NextToName | FieldPart | KnownName

// Groups of candidates. Only GroupCalendar may be kept by the config.
const (
	GroupUnusual  = "unusual"   // group A: read these with care; most words to keep are here
	GroupNameLike = "name-like" // group B: nearly all names
	GroupCalendar = "calendar"  // day and month names
)

// Cand is one candidate word: a word that may be private.
type Cand struct {
	Key     string // normal form (see Key)
	Word    string // first spelling seen
	Count   int    // times it was picked out
	Reasons Reason
	line    string // the first line it was seen in, for context
	start   int
	end     int
}

// Group says which group the word is in.
func (c *Cand) Group() string {
	if c.Reasons == Calendar {
		return GroupCalendar
	}
	if c.Reasons&nameLike != 0 || first[c.Key] {
		return GroupNameLike
	}
	return GroupUnusual
}

var (
	wordRe   = regexp.MustCompile(`\pL[\pL\pM]*(?:['’-]\pL[\pL\pM]*)*`)
	mdLead   = regexp.MustCompile(`^\s*(?:(?:[#>*+|-]+|\d+[.)]|\[[ xX]\])\s*)*`)
	nameIntr = regexp.MustCompile(`(?i)(?:\bname is|\bname:|\bnom\s*:|je m'appelle|\bthis is|c'est|it's|\bsigned|\bcontact:|\bclient:|\bcaller:)\s*$`)
	// "it's Friday" is a day far more often than a name: for a day or month
	// name these three do not make a name spot.
	weakIntr = regexp.MustCompile(`(?i)(?:\bthis is|c'est|it's)\s*$`)
	closing  = regexp.MustCompile(`(?i)^\s*(?:thanks|thank you|thx|cheers|regards|best|best regards|kind regards|sincerely|take care|` +
		`merci|merci beaucoup|cordialement|bien à vous|à bientôt|salutations|bonne journée)[^\pL\pN]*$`)
	dashSign = regexp.MustCompile(`^\s*(?:--|—)\s*\pL[\pL\pM]*\s*$`)
	signJunk = regexp.MustCompile(`[\s,.!—-]`)
)

// Tokens returns the word spans in s. A long encoded run (a tracking id, a
// base64 blob, a hex digest) is one span: split into letters it becomes
// several meaningless candidates ("AQH", "JzLm", "Pq" out of one id) and
// several decisions for the person (decision 72).
func Tokens(s string) [][2]int {
	runs := encodedRuns(s)
	if len(runs) == 0 {
		return wordSpans(s)
	}
	// Find words only in the gaps. A hyphenated word can cross the start
	// or end of a run; adding whole word spans would overlap that run.
	var out [][2]int
	pos := 0
	gap := func(end int) {
		for _, w := range wordSpans(s[pos:end]) {
			out = append(out, [2]int{pos + w[0], pos + w[1]})
		}
	}
	for _, run := range runs {
		gap(run[0])
		out = append(out, run)
		pos = run[1]
	}
	gap(len(s))
	return out
}

// runsRe finds a run of at least 12 characters from the alphabet of encoded
// text: letters, digits and the base64 "+". A slash, hyphen, underscore or "="
// ends a run, so a path ("/people/oksana"), a slug ("spring-2026-newsletter")
// and a query part ("campaign=spring") keep their ordinary words.
var runsRe = regexp.MustCompile(`[A-Za-z0-9+]{12,}`)

// encodedRuns returns the runs in s that are machine writing: 12+ characters
// with a digit, or base64 marks. A long run of letters alone is a word (a
// German compound), not an id.
func encodedRuns(s string) [][2]int {
	var out [][2]int
	for _, loc := range runsRe.FindAllStringIndex(s, -1) {
		t := s[loc[0]:loc[1]]
		digit := false
		for i := 0; i < len(t); i++ {
			if t[i] >= '0' && t[i] <= '9' {
				digit = true
				break
			}
		}
		if digit || strings.Contains(t, "+") {
			out = append(out, [2]int{loc[0], loc[1]})
		}
	}
	return out
}

// wordSpans returns the spans of words in s.
func wordSpans(s string) [][2]int {
	locs := wordRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(locs))
	for _, loc := range locs {
		out = append(out, [2]int{loc[0], loc[1]})
	}
	return out
}

// IsCapitalised: the first letter is a capital.
func IsCapitalised(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r) || unicode.IsTitle(r)
}

// IsUncased: the first letter is from a script with no capitals.
func IsUncased(w string) bool {
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsLetter(r) && !HasCase(r)
}

// HasCase reports whether a letter has an upper and lower form.
func HasCase(r rune) bool { return unicode.SimpleFold(r) != r }

// Finder collects candidate words across many files, so that one word gets
// one decision for the whole project.
type Finder struct {
	known *scan.Known
	parts map[string]bool
	cands map[string]*Cand
}

// NewFinder makes a finder. known holds values from the mapping: text they
// cover is not looked at again (it is swapped whole).
func NewFinder(known *scan.Known) *Finder {
	mustLoad()
	return &Finder{known: known, parts: map[string]bool{}, cands: map[string]*Cand{}}
}

// AddFieldParts takes the capitalised words of a field value that will be
// tokenized ("From: Daisy Okafor"), so the same words in the text are caught
// even where no other rule would catch them.
func (f *Finder) AddFieldParts(value string) {
	for _, t := range Tokens(value) {
		w := value[t[0]:t[1]]
		k := Key(w)
		if utf8.RuneCountInString(w) < 2 || !IsCapitalised(w) || noise[k] || calendar[k] {
			continue
		}
		f.parts[k] = true
	}
}

// oneWordName: a hit on a one-word NAME row (a word swapped by an earlier
// word check).
func oneWordName(line string, h scan.Hit) bool {
	return h.Category == Category && h.Standin != "" && !strings.ContainsAny(line[h.Start:h.End], " \t\u00a0")
}

// Hits returns the known values and patterns in a line, which are swapped
// whole. A one-word NAME value is left out only when it is a day or month
// name and keepCal is set (the rule keeps the calendar group): such a word
// was swapped in another file, but this file keeps it. A word the person
// kept is not a known value at all (scan.NewKnown leaves it out), nor is one
// that is also an English word (scan.SkipKnown). Every other known value is
// swapped in any case and any spot, also inside a link (decision 47).
func Hits(line string, known *scan.Known, keepCal bool) []scan.Hit {
	var out []scan.Hit
	for _, h := range scan.FindAll(line, known) {
		if keepCal && oneWordName(line, h) && calendar[Key(line[h.Start:h.End])] {
			continue
		}
		out = append(out, h)
	}
	return out
}

func covered(hits []scan.Hit, a, b int) bool {
	for _, h := range hits {
		if a < h.End && b > h.Start {
			return true
		}
	}
	return false
}

func (f *Finder) add(w string, r Reason, line string, a, b int, counted map[int]bool) {
	k := Key(w)
	c := f.cands[k]
	if c == nil {
		c = &Cand{Key: k, Word: w, line: line, start: a, end: b}
		f.cands[k] = c
	}
	c.Reasons |= r
	if !counted[a] {
		counted[a] = true
		c.Count++
	}
}

// AddLines looks for candidates in the lines of one file. Lines that are not
// free text (fields, table rows) should be passed as "".
func (f *Finder) AddLines(lines []string) {
	prev := ""
	for _, line := range lines {
		f.addLine(line, prev)
		if strings.TrimSpace(line) != "" {
			prev = line
		}
	}
}

// sentenceEnd reports whether the text before a word ends a sentence (or
// is the start of the line), so a capitalised word there may just start a
// sentence. Marks that wrap a word (* _ ( [ and quotes) are looked past. A
// colon is not a sentence end ("Notes: Grace has ..."), nor a comma.
func sentenceEnd(before string, lead, a int) bool {
	if a <= lead {
		return true
	}
	tb := strings.TrimRight(before, " \t*_([\"'“‘«")
	if strings.TrimSpace(tb) == "" || len(tb) <= lead {
		return true
	}
	return strings.HasSuffix(tb, ".") || strings.HasSuffix(tb, "!") || strings.HasSuffix(tb, "?") ||
		strings.HasSuffix(tb, "|") || strings.HasSuffix(tb, "—") || strings.HasSuffix(tb, "--")
}

func (f *Finder) addLine(line, prevLine string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	// The rules look at a one-word NAME value again (so a day name a rule
	// keeps is not caught just for being swapped elsewhere); it is still a
	// candidate, so it is swapped and approve can ask about it.
	var hits, named []scan.Hit
	for _, h := range scan.FindAll(line, f.known) {
		if oneWordName(line, h) {
			if !calendar[Key(line[h.Start:h.End])] {
				named = append(named, h)
			}
			continue
		}
		hits = append(hits, h)
	}
	lead := mdLead.FindStringIndex(line)[1]
	toks := Tokens(line)
	runAt := make(map[int]bool, 4)
	for _, r := range encodedRuns(line) {
		runAt[r[0]] = true
	}
	// A sign-off line: 1 to 3 words and nothing else, after a closing line or after "--".
	body := line[lead:]
	var only [][2]int
	n := 0
	for _, t := range toks {
		if t[0] >= lead {
			only = append(only, t)
			n += t[1] - t[0]
		}
	}
	signoff := len(only) >= 1 && len(only) <= 3 && n == len(signJunk.ReplaceAllString(body, "")) &&
		!strings.HasPrefix(strings.TrimSpace(line), "#") &&
		(closing.MatchString(prevLine) || dashSign.MatchString(line))

	counted := map[int]bool{}
	hitAt := make([]bool, len(toks)) // the word was caught as a name (not just a day name)
	calAt := make([]bool, len(toks)) // the word is a day or month name candidate
	hitPrev := false
	for i, t := range toks {
		a, b := t[0], t[1]
		w := line[a:b]
		if covered(hits, a, b) {
			hitPrev = false
			continue
		}
		if runAt[a] {
			// One encoded run, one candidate: it is noise or an id, never a
			// name, and it is swapped whole if the person keeps it out.
			f.add(w, MachineText, line, a, b, counted)
			hitPrev = false
			continue
		}
		if utf8.RuneCountInString(w) < 2 {
			hitPrev = false
			continue
		}
		lw := strings.ToLower(w)
		k := Key(w)
		before := line[:a]
		initial := sentenceEnd(before, lead, a)
		cpt := IsCapitalised(w)
		prevw, between := "", ""
		if i > 0 {
			prevw = strings.ToLower(line[toks[i-1][0]:toks[i-1][1]])
			between = strings.TrimSpace(line[toks[i-1][1]:a])
		}
		hit := false
		if IsUncased(w) {
			f.add(w, Uncased, line, a, b, counted)
			hitPrev = true
			hitAt[i] = true
			continue
		}
		if covered(named, a, b) {
			f.add(w, KnownName, line, a, b, counted)
			hit = true
		}
		if cpt && !noise[lw] && (!calendar[k] || (prevw != "from" && prevw != "by" && prevw != "with" && prevw != "per")) {
			spot := (greet[prevw] && (between == "" || between == ",")) ||
				(titles[prevw] && (between == "" || between == ".")) ||
				(nameIntr.MatchString(before) && (!calendar[k] || !weakIntr.MatchString(before)))
			if spot {
				f.add(w, NameSpot, line, a, b, counted)
				hit = true
			}
		}
		if signoff && a >= lead && !noise[lw] && !closing.MatchString(w) {
			f.add(w, SignOff, line, a, b, counted)
			hit = true
		}
		nextTo := hitPrev && between == ""
		switch {
		case cpt && f.parts[k]:
			f.add(w, FieldPart, line, a, b, counted)
			hit = true
		case cpt && (!InDict(w) || calendar[k]):
			if calendar[k] {
				f.add(w, Calendar, line, a, b, counted)
				calAt[i] = true
				if nextTo {
					// A day or month name next to a name is part of it: Ana June.
					f.add(w, NextToName, line, a, b, counted)
					hit = true
				}
			} else {
				f.add(w, NotInDict, line, a, b, counted)
				hit = true
			}
		case cpt && first[k] && !initial:
			f.add(w, FirstName, line, a, b, counted)
			hit = true
		case !cpt && first[k] && !InDict(w) && !calendar[k] && utf8.RuneCountInString(w) >= 3 && !strings.HasSuffix(before, ".") &&
			!strings.HasSuffix(before, "/") && !strings.HasSuffix(before, "@") && !strings.HasSuffix(before, "_"):
			// A first name typed in lower case ("thx kavya"). Only when it is
			// not also an English word, so "grace period" is left alone.
			f.add(w, FirstName, line, a, b, counted)
			hit = true
		case cpt && nextTo && !initial:
			// A surname that is also a word: Grace Lee, Ana Brown.
			f.add(w, NextToName, line, a, b, counted)
			hit = true
		case !cpt && nextTo && !InDict(w) && !calendar[k]:
			// A surname typed in lower case: zelda quorn.
			f.add(w, NextToName, line, a, b, counted)
			hit = true
		}
		hitPrev = hit
		hitAt[i] = hit
	}
	// Looking back: a capitalised first name, or a day or month name, right
	// before a caught name is part of it (Rose Fairweather, April Okonkwo),
	// even at the start of a sentence.
	for i := len(toks) - 1; i > 0; i-- {
		if !hitAt[i] {
			continue
		}
		p := toks[i-1]
		w := line[p[0]:p[1]]
		k := Key(w)
		if hitAt[i-1] || strings.TrimSpace(line[p[1]:toks[i][0]]) != "" || covered(hits, p[0], p[1]) ||
			!IsCapitalised(w) || utf8.RuneCountInString(w) < 2 || noise[k] || greet[k] || titles[k] {
			continue
		}
		if first[k] || calAt[i-1] {
			f.add(w, NextToName, line, p[0], p[1], counted)
			hitAt[i-1] = true
		}
	}
}

// Candidates returns every candidate, most used first.
func (f *Finder) Candidates() []*Cand {
	out := make([]*Cand, 0, len(f.cands))
	for _, c := range f.cands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Get returns the candidate for a key, or nil.
func (f *Finder) Get(key string) *Cand { return f.cands[key] }

// Mask is what other candidates look like in a context line.
const Mask = "▒▒▒"

// Context gives one short piece of the line the word was first seen in,
// with the word in [brackets] and every other candidate, known value and
// pattern masked, so one word's context does not show another name. It holds
// real text: show it to the person on a terminal only.
func (f *Finder) Context(c *Cand) string {
	line := c.line
	hits := scan.FindAll(line, f.known)
	type span struct {
		a, b   int
		target bool
	}
	spans := []span{{c.start, c.end, true}}
	over := func(a, b int) bool { return a < c.end && b > c.start }
	for _, h := range hits {
		if !over(h.Start, h.End) {
			spans = append(spans, span{h.Start, h.End, false})
		}
	}
	for _, t := range Tokens(line) {
		w := line[t[0]:t[1]]
		if over(t[0], t[1]) || covered(hits, t[0], t[1]) || f.cands[Key(w)] == nil {
			continue
		}
		if IsCapitalised(w) || IsUncased(w) || !InDict(w) {
			spans = append(spans, span{t[0], t[1], false})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].a < spans[j].a })
	// The window: about 30 letters on each side.
	from, to := runeBack(line, c.start, 30), runeFwd(line, c.end, 30)
	var b strings.Builder
	if from > 0 {
		b.WriteString("...")
	}
	last := from
	for _, s := range spans {
		if s.b <= last || s.a >= to {
			continue
		}
		if s.a > last {
			b.WriteString(line[last:s.a])
		}
		if s.target {
			b.WriteString("[" + line[s.a:s.b] + "]")
		} else {
			b.WriteString(Mask) // also covers a masked word cut by the window edge
		}
		last = s.b
	}
	if last < to {
		b.WriteString(line[last:to])
	}
	if to < len(line) {
		b.WriteString("...")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func runeBack(s string, i, n int) int {
	for ; n > 0 && i > 0; n-- {
		_, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
	}
	return i
}

func runeFwd(s string, i, n int) int {
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return i
}
